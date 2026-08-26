package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/operation"
	"github.com/gryph/omnidex/internal/queue"
)

func (runtime *directCodingSessionDeploymentRuntime) Rollback(
	terminal queue.GeneratedWorkloadDeploymentTransition,
) error {
	if terminal.State != queue.GeneratedWorkloadDeploymentRolledBack {
		return fmt.Errorf("deployment cleanup requires an exact rolled-back transition")
	}
	if detail, started := runtime.startedForwardExecutionDetail(); started {
		terminal.DetailSHA256 = detail
		if _, err := runtime.ObservePersistedRollback(terminal); err != nil {
			return err
		}
		return fmt.Errorf("deployment forward-command quiescence remains unproven after exact observation")
	}
	proceed, err := authorizeDirectCodingDestructiveRollback(
		runtime.hasDurableInitialStartExecution(),
		func() (bool, error) { return runtime.ObservePersistedRollback(terminal) },
	)
	if err != nil || !proceed {
		return err
	}
	repo := runtime.session.runtime.svc.repo
	ctx := runtime.session.runtime.ctx
	authority := runtime.session.runtime.claim.Authority
	attempt, err := repo.CurrentGeneratedWorkloadDeploymentRollbackAttempt(
		ctx, runtime.prepared.command,
	)
	if err != nil {
		return runtime.fenceRollbackObservationFailure(err)
	}
	if attempt != nil {
		observation, err := repo.GeneratedWorkloadDeploymentRollbackObservation(
			ctx, runtime.prepared.command, attempt.StepAttempt,
		)
		if err != nil {
			return runtime.fenceRollbackObservationFailure(err)
		}
		if observation != nil {
			if observation.Outcome == queue.GeneratedWorkloadDeploymentRollbackClean {
				_, _, err = repo.RecordGeneratedWorkloadDeploymentRollbackObservation(
					ctx, authority, runtime.prepared.command, runtime.prepared.rollback,
					attempt.StepAttempt, observation.Observation, terminal,
				)
				return err
			}
			if attempt.StepAttempt >= authority.Attempt {
				return fmt.Errorf(
					"deployment rollback attempt %d retained project resources; a newer bounded recovery attempt is required",
					attempt.StepAttempt,
				)
			}
		} else {
			return runtime.observeRollbackAttempt(*attempt, terminal, nil)
		}
	}
	executionRoot, err := runtime.prepared.rollbackSnapshot.ExecutionRoot()
	if err != nil {
		return runtime.fenceRollbackObservationFailure(
			fmt.Errorf("bind deployment rollback to persisted Compose snapshot: %w", err),
		)
	}
	if runtime.prepared.executionRoot != executionRoot {
		return runtime.fenceRollbackObservationFailure(fmt.Errorf(
			"%w: deployment rollback root differs from persisted Compose snapshot",
			errDirectCodingDeploymentSnapshotDrift,
		))
	}
	if err := runtime.session.qualifyDirectCodingDeploymentRuntime(
		runtime.prepared.executionRoot, runtime.prepared.command.ProfileID,
	); err != nil {
		return runtime.fenceRollbackObservationFailure(
			fmt.Errorf("qualify deployment rollback runtime before durable side-effect journal: %w", err),
		)
	}
	attemptRecord, created, err := repo.BeginGeneratedWorkloadDeploymentRollbackAttempt(
		ctx, authority, runtime.prepared.command, runtime.prepared.rollback,
	)
	if err != nil {
		return runtime.fenceRollbackObservationFailure(err)
	}
	if !created {
		return runtime.observeRollbackAttempt(attemptRecord, terminal, nil)
	}
	result, commandErr := executeDirectCodingComposeSnapshotBoundCommand(
		runtime.prepared.rollbackSnapshot, runtime.prepared.executionRoot,
		func(root string) (operation.Result, error) {
			return runtime.session.executeDirectCodingDeploymentCommand(
				root, directCodingDeploymentRollback,
				runtime.prepared.command.ProfileID, runtime.prepared.project,
				runtime.prepared.descriptor, runtime.prepared.environment,
			)
		},
	)
	if commandErr == nil {
		if _, err := repo.CompleteGeneratedWorkloadDeploymentRollbackAttempt(
			ctx, authority, runtime.prepared.command, runtime.prepared.rollback,
			result.Evidence[0],
		); err != nil {
			commandErr = fmt.Errorf("persist deployment rollback command result: %w", err)
		}
	}
	return runtime.observeRollbackAttempt(attemptRecord, terminal, commandErr)
}

func authorizeDirectCodingDestructiveRollback(
	hasDurableInitialStart bool,
	observe func() (bool, error),
) (bool, error) {
	if hasDurableInitialStart {
		return true, nil
	}
	if observe == nil {
		return false, fmt.Errorf("pre-start deployment cleanup requires exact resource observation")
	}
	needsCommand, err := observe()
	if err != nil {
		return false, err
	}
	if needsCommand {
		return false, fmt.Errorf(
			"destructive deployment rollback is forbidden before a durable initial_start execution",
		)
	}
	return false, fmt.Errorf("pre-start deployment cleanup observation stopped without a terminal result")
}

func (runtime *directCodingSessionDeploymentRuntime) hasDurableInitialStartExecution() bool {
	if runtime == nil {
		return false
	}
	_, exists := runtime.executions[queue.GeneratedDeploymentSlotInitialStart.Ordinal]
	return exists
}

// ObservePersistedRollback performs only durable-journal reads and canonical
// Docker resource observation. It is used before any current-workspace read.
// A true result means an older residual attempt is closed and a newer bounded
// command attempt may be considered after ordinary reconstruction succeeds.
func (runtime *directCodingSessionDeploymentRuntime) ObservePersistedRollback(
	terminal queue.GeneratedWorkloadDeploymentTransition,
) (bool, error) {
	repo := runtime.session.runtime.svc.repo
	ctx := runtime.session.runtime.ctx
	authority := runtime.session.runtime.claim.Authority
	attempt, err := repo.CurrentGeneratedWorkloadDeploymentRollbackAttempt(
		ctx, runtime.prepared.command,
	)
	if err != nil {
		return false, runtime.fenceRollbackObservationFailure(err)
	}
	if attempt == nil {
		recorded, err := repo.GeneratedWorkloadDeploymentPreRollbackObservation(
			ctx, runtime.prepared.command, authority.Attempt,
		)
		if err != nil {
			return false, runtime.fenceRollbackObservationFailure(err)
		}
		var observation queue.GeneratedWorkloadDeploymentRollbackObservation
		if recorded == nil {
			observed, err := observeDirectCodingDeploymentRollback(
				ctx, runtime.prepared.socketPath, runtime.prepared.rollback,
			)
			if err != nil {
				return false, runtime.fenceRollbackObservationFailure(err)
			}
			observation = observed
		} else {
			observation = recorded.Observation
		}
		deployment, value, err := repo.RecordGeneratedWorkloadDeploymentPreRollbackObservation(
			ctx, authority, runtime.prepared.command, runtime.prepared.rollback,
			observation, terminal,
		)
		if err != nil {
			return false, runtime.fenceRollbackObservationFailure(err)
		}
		if deployment.TerminalCode == "external_quiescence_unproven" {
			return false, fmt.Errorf("deployment forward-command quiescence is unproven after exact observation")
		}
		if value.Outcome == queue.GeneratedWorkloadDeploymentRollbackClean {
			if deployment.State != queue.GeneratedWorkloadDeploymentRolledBack {
				return false, fmt.Errorf("clean deployment observation retained nonterminal state %s", deployment.State)
			}
			return false, fmt.Errorf("recovered side-effect-possible deployment was cleanly rolled back")
		}
		return true, nil
	}
	recorded, err := repo.GeneratedWorkloadDeploymentRollbackObservation(
		ctx, runtime.prepared.command, attempt.StepAttempt,
	)
	if err != nil {
		return false, runtime.fenceRollbackObservationFailure(err)
	}
	var observation queue.GeneratedWorkloadDeploymentRollbackObservation
	if recorded == nil {
		observed, err := observeDirectCodingDeploymentRollback(
			ctx, runtime.prepared.socketPath, runtime.prepared.rollback,
		)
		if err != nil {
			return false, runtime.fenceRollbackObservationFailure(err)
		}
		observation = observed
	} else {
		observation = recorded.Observation
	}
	deployment, value, err := repo.RecordGeneratedWorkloadDeploymentRollbackObservation(
		ctx, authority, runtime.prepared.command, runtime.prepared.rollback,
		attempt.StepAttempt, observation, terminal,
	)
	if err != nil {
		return false, runtime.fenceRollbackObservationFailure(err)
	}
	if deployment.TerminalCode == "external_quiescence_unproven" {
		return false, fmt.Errorf("deployment forward-command quiescence is unproven after exact observation")
	}
	if value.Outcome == queue.GeneratedWorkloadDeploymentRollbackClean {
		if deployment.State != queue.GeneratedWorkloadDeploymentRolledBack {
			return false, fmt.Errorf("clean deployment observation retained nonterminal state %s", deployment.State)
		}
		return false, fmt.Errorf("recovered side-effect-possible deployment was cleanly rolled back")
	}
	if attempt.StepAttempt >= authority.Attempt {
		return false, fmt.Errorf(
			"deployment rollback attempt %d retained project resources; a newer bounded recovery attempt is required",
			attempt.StepAttempt,
		)
	}
	return true, nil
}

func (runtime *directCodingSessionDeploymentRuntime) observeRollbackAttempt(
	attempt queue.GeneratedWorkloadDeploymentRollbackAttemptRecord,
	terminal queue.GeneratedWorkloadDeploymentTransition,
	commandErr error,
) error {
	observation, err := observeDirectCodingDeploymentRollback(
		runtime.session.runtime.ctx, runtime.prepared.socketPath, runtime.prepared.rollback,
	)
	if err != nil {
		if commandErr != nil {
			err = fmt.Errorf("%v; observe rollback resources: %w", commandErr, err)
		}
		return runtime.fenceRollbackObservationFailure(err)
	}
	deployment, recorded, err := runtime.session.runtime.svc.repo.
		RecordGeneratedWorkloadDeploymentRollbackObservation(
			runtime.session.runtime.ctx, runtime.session.runtime.claim.Authority,
			runtime.prepared.command, runtime.prepared.rollback, attempt.StepAttempt,
			observation, terminal,
		)
	if err != nil {
		return runtime.fenceRollbackObservationFailure(err)
	}
	if recorded.Outcome == queue.GeneratedWorkloadDeploymentRollbackResidual {
		return fmt.Errorf(
			"deployment rollback attempt %d retained %d containers, %d networks, and %d volumes (state %s)",
			attempt.StepAttempt, len(recorded.Observation.ContainerIDs),
			len(recorded.Observation.NetworkIDs), len(recorded.Observation.VolumeNames),
			deployment.State,
		)
	}
	if deployment.State != queue.GeneratedWorkloadDeploymentRolledBack {
		return fmt.Errorf("clean deployment rollback did not commit rolled-back terminal state")
	}
	return nil
}

func (runtime *directCodingSessionDeploymentRuntime) fenceRollbackObservationFailure(
	cause error,
) error {
	if cause == nil {
		cause = fmt.Errorf("rollback observation failed without an exact cause")
	}
	code := "rollback_observation_failed"
	detail := directCodingDigest(cause.Error())
	if startedDetail, started := runtime.startedForwardExecutionDetail(); started {
		code = "external_quiescence_unproven"
		detail = startedDetail
	}
	snapshot, err := runtime.session.runtime.svc.repo.CurrentGeneratedWorkloadDeployment(
		runtime.session.runtime.ctx, runtime.prepared.command.Authority.JobID,
		runtime.prepared.command.Authority.Generation,
	)
	if err == nil && snapshot != nil &&
		snapshot.Record.State == queue.GeneratedWorkloadDeploymentIndeterminate {
		if code == "external_quiescence_unproven" &&
			(snapshot.Record.TerminalCode != code || snapshot.Record.DetailSHA256 != detail) {
			return fmt.Errorf("%w; persisted deployment quiescence fence differs", cause)
		}
		return cause
	}
	_, transitionErr := runtime.Transition(queue.GeneratedWorkloadDeploymentTransition{
		State: queue.GeneratedWorkloadDeploymentIndeterminate,
		Code:  code, DetailSHA256: detail,
	})
	if transitionErr != nil {
		return fmt.Errorf("%w; fence deployment cleanup uncertainty: %v", cause, transitionErr)
	}
	return cause
}
