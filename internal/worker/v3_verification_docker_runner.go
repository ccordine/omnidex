package worker

import (
	"errors"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/experiment"
	"github.com/gryph/omnidex/internal/queue"
)

func (session *directCodingSession) runRecordedDockerVerificationCommand(workspace *experiment.Workspace, phase queue.VerificationCommandPhase, command directCodingVerificationCommand) (result directCodingVerificationCommandResult, resultErr error) {
	return session.runRecordedDockerCommand(workspace, phase, command, false)
}

func (session *directCodingSession) runRecordedDockerAcquisitionCommand(workspace *experiment.Workspace, phase queue.VerificationCommandPhase, command directCodingVerificationCommand) (directCodingVerificationCommandResult, error) {
	if phase != queue.VerificationIsolatedInstall && phase != queue.VerificationHostInstall {
		return directCodingVerificationCommandResult{}, fmt.Errorf("Docker acquisition is only available at dependency installation")
	}
	return session.runRecordedDockerCommand(workspace, phase, command, true)
}

func (session *directCodingSession) runRecordedDockerCommand(workspace *experiment.Workspace, phase queue.VerificationCommandPhase, command directCodingVerificationCommand, acquisition bool) (result directCodingVerificationCommandResult, resultErr error) {
	if session == nil || session.runtime == nil || session.runtime.svc == nil || session.runtime.svc.repo == nil || session.runtime.claim == nil || session.runtime.ctx == nil || workspace == nil {
		return result, fmt.Errorf("Docker verification requires one active persisted step and experiment workspace")
	}
	if directCodingVerificationPhaseUsesHostRoot(phase) {
		if err := session.runtime.requireWorkspaceMutationFence(); err != nil {
			return result, err
		}
	}
	ordinal, err := session.nextVerificationCommandOrdinal()
	if err != nil {
		return result, err
	}
	started := directCodingVerificationTimestamp(time.Now())
	execute := workspace.Run
	if acquisition {
		execute = workspace.Acquire
	}
	observed, runErr := execute(session.runtime.ctx, experiment.Command{
		Argv: command.Argv, Environment: command.Environment, Stdin: command.Stdin, Timeout: command.Timeout,
	})
	finished := directCodingVerificationTimestamp(time.Now())
	result.Stdout, result.Stderr = observed.Stdout, observed.Stderr
	record := queue.VerificationCommandEvidence{
		Authority: session.runtime.claim.Authority, Phase: phase, Ordinal: ordinal,
		Argv: append([]string(nil), command.Argv...), Environment: observed.Environment, Stdin: command.Stdin,
		WorkingDirectory: experiment.WorkingDirectory,
		ContainerID:      observed.ContainerID, ContainerImageID: observed.ImageID, ContainerExecID: observed.ExecutionID,
		ContainerNetworkEnabled: observed.NetworkEnabled,
		StartedAt:               started, FinishedAt: finished, ExitCode: observed.ExitCode,
		Stdout: observed.Stdout, Stderr: observed.Stderr, StdoutComplete: observed.StdoutComplete, StderrComplete: observed.StderrComplete,
	}
	if observed.ExitCode == nil {
		if runErr == nil {
			runErr = fmt.Errorf("Docker verification returned no exact process exit")
		}
		record.LaunchError = trimForBudget(runErr.Error(), 4000)
	} else if runErr != nil {
		var exited *experiment.ExitError
		if !errors.As(runErr, &exited) || exited.Code != *observed.ExitCode {
			record.ObservationError = trimForBudget(runErr.Error(), 4000)
		}
	}
	if directCodingVerificationPhaseUsesHostRoot(phase) {
		if err := session.runtime.workspaceFence.Reattest(session.root); err != nil {
			runErr = errors.Join(runErr, err)
			record.ObservationError = trimForBudget(runErr.Error(), 4000)
		}
	}
	ctx, cancel := directCodingVerificationEvidenceContext(session.runtime.ctx)
	defer cancel()
	if err := session.runtime.svc.repo.AppendVerificationCommandEvidence(ctx, record); err != nil {
		return result, errors.Join(runErr, fmt.Errorf("record Docker verification command %d: %w", ordinal, err))
	}
	return result, runErr
}
