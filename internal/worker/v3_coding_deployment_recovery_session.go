package worker

import (
	"fmt"
	"os"

	"github.com/gryph/omnidex/internal/queue"
)

type directCodingSessionDeploymentRecovery struct {
	session *directCodingSession
}

func (recovery *directCodingSessionDeploymentRecovery) CurrentDeployment() (
	*queue.GeneratedWorkloadDeploymentSnapshot,
	error,
) {
	if recovery == nil || recovery.session == nil || recovery.session.runtime == nil ||
		recovery.session.runtime.svc == nil || recovery.session.runtime.svc.repo == nil ||
		recovery.session.runtime.claim == nil {
		return nil, fmt.Errorf("persisted cleanup observation requires one claimed runtime")
	}
	session := recovery.session
	authority := session.runtime.claim.Authority
	return session.runtime.svc.repo.CurrentGeneratedWorkloadDeployment(
		session.runtime.ctx, authority.JobID, authority.Generation,
	)
}

func (recovery *directCodingSessionDeploymentRecovery) DeployBeforeJournal(
	verification directCodingVerification,
) (directCodingDeploymentOutcome, error) {
	session, err := recovery.requireSession()
	if err != nil {
		return directCodingDeploymentOutcome{}, err
	}
	return session.persistVerifiedApplication(verification)
}

func (recovery *directCodingSessionDeploymentRecovery) ResumeJournaledDeployment(
	snapshot *queue.GeneratedWorkloadDeploymentSnapshot,
	verification directCodingVerification,
) (directCodingDeploymentOutcome, error) {
	session, err := recovery.requireSession()
	if err != nil {
		return directCodingDeploymentOutcome{}, err
	}
	authority := session.runtime.claim.Authority
	deploymentEvidence, err := session.runtime.svc.repo.GeneratedWorkloadDeploymentEvidence(
		session.runtime.ctx, authority.JobID, authority.Generation,
	)
	if err != nil || deploymentEvidence == nil {
		return directCodingDeploymentOutcome{}, fmt.Errorf(
			"load persisted cleanup authority before workspace reconstruction: %w", err,
		)
	}
	cleanup, transition, err := recovery.recoveredCleanupTransition(
		snapshot, deploymentEvidence,
	)
	if err != nil {
		return directCodingDeploymentOutcome{}, err
	}
	if cleanup {
		observationRuntime, err := recovery.persistedCleanupObservationRuntime(
			snapshot, deploymentEvidence,
		)
		if err != nil {
			return directCodingDeploymentOutcome{}, err
		}
		startedDetail, started := recoveredStartedForwardExecutionDetail(deploymentEvidence)
		if started {
			transition.DetailSHA256 = startedDetail
		}
		needsCommand, err := observationRuntime.ObservePersistedRollback(transition)
		if err != nil {
			return directCodingDeploymentOutcome{}, fmt.Errorf(
				"observe persisted deployment cleanup before workspace reconstruction: %w", err,
			)
		}
		if started {
			return directCodingDeploymentOutcome{}, fmt.Errorf(
				"deployment forward-command quiescence is unproven after exact persisted observation",
			)
		}
		if !needsCommand {
			return directCodingDeploymentOutcome{}, fmt.Errorf("persisted deployment cleanup stopped without a terminal result")
		}
		if !recoveredDeploymentHasInitialStartExecution(deploymentEvidence) {
			return directCodingDeploymentOutcome{}, fmt.Errorf(
				"destructive recovered cleanup is forbidden before a durable initial_start execution",
			)
		}
	}
	prepared, recoveredEvidence, err := recovery.reconstruct(snapshot, verification)
	if err != nil {
		return directCodingDeploymentOutcome{}, err
	}
	runtime := &directCodingSessionDeploymentRuntime{session: recovery.session, prepared: prepared}
	if err := restoreDirectCodingRecoveredExecutions(runtime, recoveredEvidence); err != nil {
		return directCodingDeploymentOutcome{}, err
	}
	if cleanup {
		if err := runtime.Rollback(transition); err != nil {
			return directCodingDeploymentOutcome{}, fmt.Errorf("reconcile recovered deployment cleanup: %w", err)
		}
		return directCodingDeploymentOutcome{}, fmt.Errorf("recovered side-effect-possible deployment was cleanly rolled back")
	}
	return runDirectCodingDeploymentLifecycle(prepared, verification, runtime)
}

func restoreDirectCodingRecoveredExecutions(
	runtime *directCodingSessionDeploymentRuntime,
	evidence *queue.GeneratedWorkloadDeploymentEvidenceSnapshot,
) error {
	if runtime == nil || evidence == nil {
		return fmt.Errorf("reconstructed deployment omitted durable execution evidence")
	}
	for _, execution := range evidence.Executions {
		runtime.rememberExecution(execution)
	}
	return nil
}

func (recovery *directCodingSessionDeploymentRecovery) persistedCleanupObservationRuntime(
	snapshot *queue.GeneratedWorkloadDeploymentSnapshot,
	evidence *queue.GeneratedWorkloadDeploymentEvidenceSnapshot,
) (*directCodingSessionDeploymentRuntime, error) {
	if snapshot == nil || evidence == nil || evidence.RollbackPlan == nil {
		return nil, fmt.Errorf("persisted deployment cleanup lacks an exact rollback plan")
	}
	session, err := recovery.requireClaimedRuntime()
	if err != nil {
		return nil, err
	}
	head, err := session.runtime.svc.repo.CurrentGeneratedWorkloadProjectDeploymentHead(
		session.runtime.ctx, snapshot.Command.Authority.ProjectID,
	)
	if err != nil || head == nil || head.Candidate == nil ||
		head.Candidate.DeploymentID != snapshot.Record.OperationID {
		return nil, fmt.Errorf("persisted deployment cleanup lacks exact candidate authority: %w", err)
	}
	reservation, err := session.runtime.svc.repo.ReserveGeneratedWorkloadProjectDeploymentCandidate(
		session.runtime.ctx, session.runtime.claim.Authority, snapshot.Command,
		head.SecretGeneration, head.DeploymentKeyFingerprintSHA256,
		queue.GeneratedWorkloadProjectDeploymentHeadExpectation{
			Revision: head.Revision, Fence: head.Fence,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("take over persisted deployment cleanup executor: %w", err)
	}
	socketPath, err := directCodingRecoveryDockerSocket()
	if err != nil {
		return nil, err
	}
	runtime := &directCodingSessionDeploymentRuntime{
		session: session,
		prepared: directCodingPreparedDeployment{
			project: snapshot.Command.ComposeProject, command: snapshot.Command,
			rollback: *evidence.RollbackPlan, record: snapshot.Record,
			reservation: reservation, socketPath: socketPath,
		},
		executions: make(map[int]queue.GeneratedWorkloadDeploymentExecutionRecord),
	}
	for _, execution := range evidence.Executions {
		runtime.rememberExecution(execution)
	}
	return runtime, nil
}

func (recovery *directCodingSessionDeploymentRecovery) requireClaimedRuntime() (
	*directCodingSession,
	error,
) {
	if recovery == nil || recovery.session == nil || recovery.session.runtime == nil ||
		recovery.session.runtime.svc == nil || recovery.session.runtime.svc.repo == nil ||
		recovery.session.runtime.claim == nil {
		return nil, fmt.Errorf(
			"persistent deployment recovery requires one claimed queue runtime",
		)
	}
	return recovery.session, nil
}

func (recovery *directCodingSessionDeploymentRecovery) requireSession() (
	*directCodingSession,
	error,
) {
	session, err := recovery.requireClaimedRuntime()
	if err != nil {
		return nil, err
	}
	if session.cognition == nil || session.program == nil {
		return nil, fmt.Errorf(
			"persistent deployment recovery requires one compiled claimed session",
		)
	}
	return session, nil
}

func directCodingRecoveryDockerSocket() (string, error) {
	_, socketPath, err := resolveV3DockerHost(os.Getenv("DOCKER_HOST"))
	return socketPath, err
}

var _ directCodingDeploymentRecoveryBackend = (*directCodingSessionDeploymentRecovery)(nil)
