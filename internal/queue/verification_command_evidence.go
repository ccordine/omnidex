package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

type VerificationCommandPhase string

// One command can retain 1 MiB each of stdin, stdout, and stderr.
const MaxVerificationCommandEvidencePageSize = 10

const (
	VerificationIsolatedInstall        VerificationCommandPhase = "isolated_install"
	VerificationIsolatedImplementation VerificationCommandPhase = "isolated_implementation"
	VerificationIsolatedTask           VerificationCommandPhase = "isolated_task"
	VerificationIsolatedFinal          VerificationCommandPhase = "isolated_final"
	VerificationHostInstall            VerificationCommandPhase = "host_install"
	VerificationHostFinal              VerificationCommandPhase = "host_final"
)

type VerificationCommandStatus string

const (
	VerificationCommandSucceeded         VerificationCommandStatus = "succeeded"
	VerificationCommandExitFailed        VerificationCommandStatus = "exit_failed"
	VerificationCommandLaunchFailed      VerificationCommandStatus = "launch_failed"
	VerificationCommandObservationFailed VerificationCommandStatus = "observation_failed"
)

// VerificationCommandEvidence is both the append input and history
// projection for one code-selected argv execution. Append callers provide only
// the command and result fields; database-owned identity,
// duration, status, and CreatedAt must remain zero.
type VerificationCommandEvidence struct {
	ID                      int64                      `json:"id,omitempty"`
	Authority               model.StepAttemptAuthority `json:"authority"`
	Phase                   VerificationCommandPhase   `json:"phase"`
	Ordinal                 int64                      `json:"ordinal"`
	Argv                    []string                   `json:"argv"`
	Environment             []string                   `json:"environment"`
	Stdin                   []byte                     `json:"stdin,omitempty"`
	WorkingDirectory        string                     `json:"working_directory"`
	ContainerID             string                     `json:"container_id,omitempty"`
	ContainerImageID        string                     `json:"container_image_id,omitempty"`
	ContainerExecID         string                     `json:"container_exec_id,omitempty"`
	ContainerNetworkEnabled *bool                      `json:"container_network_enabled,omitempty"`
	StartedAt               time.Time                  `json:"started_at"`
	FinishedAt              time.Time                  `json:"finished_at"`
	ExitCode                *int                       `json:"exit_code,omitempty"`
	LaunchError             string                     `json:"launch_error,omitempty"`
	ObservationError        string                     `json:"observation_error,omitempty"`
	Stdout                  []byte                     `json:"stdout"`
	StdoutComplete          bool                       `json:"stdout_complete"`
	Stderr                  []byte                     `json:"stderr"`
	StderrComplete          bool                       `json:"stderr_complete"`
	Status                  VerificationCommandStatus  `json:"status,omitempty"`
	DurationNanos           int64                      `json:"duration_nanos,omitempty"`
	StdinPresent            bool                       `json:"stdin_present"`
	CreatedAt               time.Time                  `json:"created_at,omitempty"`
}

func (r *Repository) AppendVerificationCommandEvidence(
	ctx context.Context,
	record VerificationCommandEvidence,
) error {
	if ctx == nil || r == nil || r.pool == nil {
		return fmt.Errorf("verification command evidence requires context and PostgreSQL")
	}
	normalized, err := normalizeVerificationCommandEvidence(record)
	if err != nil {
		return err
	}
	argvJSON, err := json.Marshal(normalized.Argv)
	if err != nil {
		return fmt.Errorf("encode verification argv evidence: %w", err)
	}
	environmentJSON, err := json.Marshal(normalized.Environment)
	if err != nil {
		return fmt.Errorf("encode verification environment evidence: %w", err)
	}
	stdin := any(nil)
	if normalized.StdinPresent {
		stdin = normalized.Stdin
	}
	exitCode := any(nil)
	if normalized.ExitCode != nil {
		exitCode = *normalized.ExitCode
	}
	launchError := any(nil)
	if normalized.LaunchError != "" {
		launchError = normalized.LaunchError
	}
	observationError := any(nil)
	if normalized.ObservationError != "" {
		observationError = normalized.ObservationError
	}
	result, err := r.pool.Exec(ctx, `
		INSERT INTO verification_command_evidence (
			job_id,generation,step_id,step_attempt,worker_id,phase,ordinal,
			argv,environment,stdin_present,stdin,working_directory,
			started_at,finished_at,duration_nanos,exit_code,launch_error,observation_error,
			stdout,stdout_complete,stderr,stderr_complete,status,
			container_id,container_image_id,container_exec_id,container_network_enabled
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,
			$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27
		)
	`, normalized.Authority.JobID, normalized.Authority.Generation,
		normalized.Authority.StepID, normalized.Authority.Attempt,
		normalized.Authority.WorkerID, string(normalized.Phase), normalized.Ordinal,
		argvJSON, environmentJSON, normalized.StdinPresent, stdin,
		normalized.WorkingDirectory, normalized.StartedAt, normalized.FinishedAt,
		normalized.DurationNanos, exitCode, launchError, observationError,
		normalized.Stdout, normalized.StdoutComplete,
		normalized.Stderr, normalized.StderrComplete, string(normalized.Status),
		optionalExactText(normalized.ContainerID), optionalExactText(normalized.ContainerImageID), optionalExactText(normalized.ContainerExecID), normalized.ContainerNetworkEnabled)
	if err != nil {
		return fmt.Errorf("append exact verification command evidence: %w", err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("verification command evidence was not appended")
	}
	return nil
}

func optionalExactText(value string) any {
	if value == "" {
		return nil
	}
	return value
}
