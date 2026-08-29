package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
)

// directCodingDeploymentRecoveryBackend separates recovery state routing from
// the concrete PostgreSQL, workspace, and Docker readers. Recovery owns no
// semantic decision: the durable journal state selects exactly one code path.
type directCodingDeploymentRecoveryBackend interface {
	CurrentDeployment() (*queue.GeneratedWorkloadDeploymentSnapshot, error)
	DeployBeforeJournal(directCodingVerification) (directCodingDeploymentOutcome, error)
	ResumeJournaledDeployment(
		*queue.GeneratedWorkloadDeploymentSnapshot,
		directCodingVerification,
	) (directCodingDeploymentOutcome, error)
}

type directCodingDeploymentRecovery struct {
	backend directCodingDeploymentRecoveryBackend
}

func newDirectCodingDeploymentRecovery(
	session *directCodingSession,
) *directCodingDeploymentRecovery {
	return &directCodingDeploymentRecovery{
		backend: &directCodingSessionDeploymentRecovery{session: session},
	}
}

func (recovery *directCodingDeploymentRecovery) RecoverVerifiedDeployment(
	verification directCodingVerification,
	disposition directCodingCompletionTaskDisposition,
) (directCodingDeploymentOutcome, error) {
	if recovery == nil || recovery.backend == nil {
		return directCodingDeploymentOutcome{}, fmt.Errorf(
			"persistent deployment recovery requires one concrete backend",
		)
	}
	if err := verification.validate(); err != nil {
		return directCodingDeploymentOutcome{}, fmt.Errorf(
			"persistent deployment recovery requires exact successful verification: %w", err,
		)
	}
	if !verification.Passed {
		return directCodingDeploymentOutcome{}, fmt.Errorf(
			"persistent deployment recovery requires exact successful verification",
		)
	}
	snapshot, err := recovery.backend.CurrentDeployment()
	if err != nil {
		return directCodingDeploymentOutcome{}, fmt.Errorf(
			"load persistent deployment recovery state: %w", err,
		)
	}
	if snapshot == nil {
		if disposition != directCodingCompletionTaskResumed {
			return directCodingDeploymentOutcome{}, fmt.Errorf(
				"completed deployment cognition has no durable deployment journal",
			)
		}
		return recovery.backend.DeployBeforeJournal(verification)
	}
	switch snapshot.Record.State {
	case queue.GeneratedWorkloadDeploymentPrepared,
		queue.GeneratedWorkloadDeploymentApplying,
		queue.GeneratedWorkloadDeploymentIndeterminate:
		if disposition != directCodingCompletionTaskResumed || snapshot.Receipt != nil {
			return directCodingDeploymentOutcome{}, fmt.Errorf(
				"journaled deployment %s conflicts with persisted cognition or receipt state",
				snapshot.Record.State,
			)
		}
		return recovery.backend.ResumeJournaledDeployment(snapshot, verification)
	case queue.GeneratedWorkloadDeploymentApplied:
		return directCodingDeploymentOutcome{}, fmt.Errorf(
			"applied deployment reached late recovery instead of the required pre-workspace gate",
		)
	case queue.GeneratedWorkloadDeploymentFailed,
		queue.GeneratedWorkloadDeploymentRolledBack:
		return directCodingDeploymentOutcome{}, fmt.Errorf(
			"persistent deployment journal is terminal in state %s", snapshot.Record.State,
		)
	default:
		return directCodingDeploymentOutcome{}, fmt.Errorf(
			"persistent deployment journal has unsupported state %q", snapshot.Record.State,
		)
	}
}
