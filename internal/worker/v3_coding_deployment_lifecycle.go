package worker

import (
	"errors"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/queue"
)

type directCodingDeploymentOutcome struct {
	OperationID   string
	ReceiptSHA256 string
	Endpoint      directCodingObservedEndpoint
}

type directCodingDeploymentLifecycleRuntime interface {
	Transition(queue.GeneratedWorkloadDeploymentTransition) (queue.GeneratedWorkloadDeploymentRecord, error)
	Rollback(queue.GeneratedWorkloadDeploymentTransition) error
	Execute(queue.GeneratedWorkloadDeploymentLifecycleSlot) error
	Observe(queue.GeneratedWorkloadDeploymentLifecycleSlot) (directCodingDeploymentObservation, error)
	EvidenceIDs() ([]int64, []int64, error)
	Seal(queue.GeneratedWorkloadDeploymentReceipt) (queue.GeneratedWorkloadDeploymentRecord, error)
	CompleteDeployment(string, string) error
	Now() time.Time
}

func (s *directCodingSession) persistVerifiedApplication(
	verification directCodingVerification,
) (directCodingDeploymentOutcome, error) {
	prepared, err := s.prepareVerifiedDeployment(verification)
	if err != nil {
		return directCodingDeploymentOutcome{}, err
	}
	runtime := &directCodingSessionDeploymentRuntime{session: s, prepared: prepared}
	return runDirectCodingDeploymentLifecycle(prepared, verification, runtime)
}

func runDirectCodingDeploymentLifecycle(
	prepared directCodingPreparedDeployment,
	verification directCodingVerification,
	runtime directCodingDeploymentLifecycleRuntime,
) (directCodingDeploymentOutcome, error) {
	if runtime == nil {
		return directCodingDeploymentOutcome{}, fmt.Errorf("persistent deployment lifecycle runtime is required")
	}
	applying, err := runtime.Transition(queue.GeneratedWorkloadDeploymentTransition{
		State: queue.GeneratedWorkloadDeploymentApplying,
	})
	if err != nil {
		return directCodingDeploymentOutcome{}, err
	}
	prepared.record = applying
	if err := runtime.Execute(queue.GeneratedDeploymentSlotBuild); err != nil {
		sideEffects := !errors.Is(err, errDirectCodingDeploymentSnapshotDrift)
		return directCodingDeploymentOutcome{}, failDirectCodingDeployment(runtime, "build_failed", err, sideEffects)
	}
	if err := runtime.Execute(queue.GeneratedDeploymentSlotInitialStart); err != nil {
		return directCodingDeploymentOutcome{}, failDirectCodingDeployment(runtime, "start_failed", err, true)
	}
	if prepared.hasState && prepared.descriptor.MigrationScript != "" {
		if err := runtime.Execute(queue.GeneratedDeploymentSlotMigrate); err != nil {
			return directCodingDeploymentOutcome{}, failDirectCodingDeployment(runtime, "migration_failed", err, true)
		}
	}
	initial, err := runtime.Observe(queue.GeneratedDeploymentSlotInitialObserve)
	if err != nil {
		return directCodingDeploymentOutcome{}, failDirectCodingDeployment(runtime, "initial_observation_failed", err, true)
	}
	appliedAt := runtime.Now()
	if prepared.hasState {
		if err := runtime.Execute(queue.GeneratedDeploymentSlotStateWrite); err != nil {
			return directCodingDeploymentOutcome{}, failDirectCodingDeployment(runtime, "state_write_failed", err, true)
		}
	}
	if err := runtime.Execute(queue.GeneratedDeploymentSlotRestart); err != nil {
		return directCodingDeploymentOutcome{}, failDirectCodingDeployment(runtime, "restart_failed", err, true)
	}
	if err := runtime.Execute(queue.GeneratedDeploymentSlotRestartStart); err != nil {
		return directCodingDeploymentOutcome{}, failDirectCodingDeployment(runtime, "restart_wait_failed", err, true)
	}
	final, err := runtime.Observe(queue.GeneratedDeploymentSlotFinalObserve)
	if err != nil {
		return directCodingDeploymentOutcome{}, failDirectCodingDeployment(runtime, "restart_observation_failed", err, true)
	}
	observedAt := runtime.Now()
	if final.SHA256 != initial.SHA256 {
		return directCodingDeploymentOutcome{}, failDirectCodingDeployment(
			runtime, "restart_identity_changed",
			fmt.Errorf("persistent deployment identity changed across registered restart verification"), true,
		)
	}
	if prepared.hasState {
		if err := runtime.Execute(queue.GeneratedDeploymentSlotStateRead); err != nil {
			return directCodingDeploymentOutcome{}, failDirectCodingDeployment(runtime, "state_read_failed", err, true)
		}
	}
	executionEvidenceIDs, observationEvidenceIDs, err := runtime.EvidenceIDs()
	if err != nil {
		return directCodingDeploymentOutcome{}, failDirectCodingDeployment(runtime, "evidence_binding_failed", err, true)
	}
	receipt, err := directCodingGeneratedDeploymentReceipt(
		prepared.record, prepared.command, final, appliedAt, observedAt,
		prepared.verification.ID, executionEvidenceIDs, observationEvidenceIDs,
	)
	if err != nil {
		return directCodingDeploymentOutcome{}, failDirectCodingDeployment(runtime, "receipt_invalid", err, true)
	}
	applied, err := runtime.Seal(receipt)
	if err != nil {
		return directCodingDeploymentOutcome{}, failDirectCodingDeployment(runtime, "receipt_persistence_failed", err, true)
	}
	if applied.State != queue.GeneratedWorkloadDeploymentApplied ||
		len(applied.ReceiptSHA256) != 64 || applied.EvidenceID <= 0 {
		return directCodingDeploymentOutcome{}, fmt.Errorf("sealed deployment receipt is incomplete")
	}
	if err := runtime.CompleteDeployment(applied.OperationID, applied.ReceiptSHA256); err != nil {
		return directCodingDeploymentOutcome{}, fmt.Errorf("complete persisted deployment cognition: %w", err)
	}
	return directCodingDeploymentOutcome{
		OperationID: applied.OperationID, ReceiptSHA256: applied.ReceiptSHA256,
		Endpoint: final.Endpoint,
	}, nil
}

func failDirectCodingDeployment(
	runtime directCodingDeploymentLifecycleRuntime,
	code string,
	cause error,
	sideEffects bool,
) error {
	if cause == nil {
		cause = fmt.Errorf("persistent deployment failed without an exact cause")
	}
	if namespaceFailure, ok := directCodingDeploymentNamespaceFailureAuthority(cause); ok {
		if namespaceFailure.Journaled {
			transition := queue.GeneratedWorkloadDeploymentTransition{
				State:        queue.GeneratedWorkloadDeploymentRolledBack,
				Code:         directCodingDeploymentExternalNamespaceConflict,
				DetailSHA256: directCodingDigest(cause.Error()),
			}
			if rollbackErr := runtime.Rollback(transition); rollbackErr != nil {
				return fmt.Errorf("%w; journaled no-spawn observation did not converge: %v", cause, rollbackErr)
			}
			return cause
		}
		state := queue.GeneratedWorkloadDeploymentIndeterminate
		if namespaceFailure.Slot == queue.GeneratedDeploymentSlotBuild {
			state = queue.GeneratedWorkloadDeploymentFailed
		}
		transition := queue.GeneratedWorkloadDeploymentTransition{
			State: state, Code: directCodingDeploymentExternalNamespaceConflict,
			DetailSHA256: directCodingDigest(cause.Error()),
		}
		if _, err := runtime.Transition(transition); err != nil {
			return fmt.Errorf("%w; persist deployment %s state: %v", cause, transition.State, err)
		}
		return cause
	}
	transition := queue.GeneratedWorkloadDeploymentTransition{
		State: queue.GeneratedWorkloadDeploymentFailed,
		Code:  code, DetailSHA256: directCodingDigest(cause.Error()),
	}
	if sideEffects {
		transition.State = queue.GeneratedWorkloadDeploymentRolledBack
		if rollbackErr := runtime.Rollback(transition); rollbackErr != nil {
			return fmt.Errorf("%w; scoped deployment rollback did not converge: %v", cause, rollbackErr)
		}
		return cause
	}
	if _, err := runtime.Transition(transition); err != nil {
		return fmt.Errorf("%w; persist deployment %s state: %v", cause, transition.State, err)
	}
	return cause
}
