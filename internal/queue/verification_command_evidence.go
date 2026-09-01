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
	VerificationHostCleanup            VerificationCommandPhase = "host_cleanup"
)

type VerificationCommandStatus string

const (
	VerificationCommandSucceeded         VerificationCommandStatus = "succeeded"
	VerificationCommandExitFailed        VerificationCommandStatus = "exit_failed"
	VerificationCommandLaunchFailed      VerificationCommandStatus = "launch_failed"
	VerificationCommandWorkspaceChanged  VerificationCommandStatus = "workspace_changed"
	VerificationCommandObservationFailed VerificationCommandStatus = "observation_failed"
)

// VerificationCommandEvidence is both the append input and immutable history
// projection for one code-selected argv execution. Append callers provide only
// the authority-through-workspace fields; PostgreSQL-owned identity, hashes,
// duration, status, and CreatedAt must remain zero.
type VerificationCommandEvidence struct {
	ID                    int64                      `json:"id,omitempty"`
	Authority             model.StepAttemptAuthority `json:"authority"`
	Phase                 VerificationCommandPhase   `json:"phase"`
	Ordinal               int64                      `json:"ordinal"`
	Argv                  []string                   `json:"argv"`
	Environment           []string                   `json:"environment"`
	Stdin                 []byte                     `json:"stdin,omitempty"`
	WorkingDirectory      string                     `json:"working_directory"`
	StartedAt             time.Time                  `json:"started_at"`
	FinishedAt            time.Time                  `json:"finished_at"`
	ExitCode              *int                       `json:"exit_code,omitempty"`
	LaunchError           string                     `json:"launch_error,omitempty"`
	ObservationError      string                     `json:"observation_error,omitempty"`
	Stdout                []byte                     `json:"stdout"`
	StdoutComplete        bool                       `json:"stdout_complete"`
	Stderr                []byte                     `json:"stderr"`
	StderrComplete        bool                       `json:"stderr_complete"`
	WorkspaceSHA256Before string                     `json:"workspace_sha256_before,omitempty"`
	WorkspaceSHA256After  string                     `json:"workspace_sha256_after,omitempty"`
	Status                VerificationCommandStatus  `json:"status,omitempty"`
	DurationNanos         int64                      `json:"duration_nanos,omitempty"`
	ArgvSHA256            string                     `json:"argv_sha256,omitempty"`
	EnvironmentSHA256     string                     `json:"environment_sha256,omitempty"`
	StdinPresent          bool                       `json:"stdin_present"`
	StdinSHA256           string                     `json:"stdin_sha256,omitempty"`
	StdoutSHA256          string                     `json:"stdout_sha256,omitempty"`
	StderrSHA256          string                     `json:"stderr_sha256,omitempty"`
	CreatedAt             time.Time                  `json:"created_at,omitempty"`
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
	stdinSHA256 := any(nil)
	if normalized.StdinPresent {
		stdin = normalized.Stdin
		stdinSHA256 = normalized.StdinSHA256
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
	workspaceBefore := optionalExactText(normalized.WorkspaceSHA256Before)
	workspaceAfter := optionalExactText(normalized.WorkspaceSHA256After)
	result, err := r.pool.Exec(ctx, `
		INSERT INTO verification_command_evidence (
			job_id,generation,step_id,step_attempt,worker_id,phase,ordinal,
			argv,argv_sha256,environment,environment_sha256,
			stdin_present,stdin,stdin_sha256,working_directory,
			started_at,finished_at,duration_nanos,exit_code,launch_error,observation_error,
			stdout,stdout_complete,stdout_sha256,stderr,stderr_complete,stderr_sha256,
			workspace_sha256_before,workspace_sha256_after,status
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,
			$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30
		)
	`, normalized.Authority.JobID, normalized.Authority.Generation,
		normalized.Authority.StepID, normalized.Authority.Attempt,
		normalized.Authority.WorkerID, string(normalized.Phase), normalized.Ordinal,
		argvJSON, normalized.ArgvSHA256, environmentJSON,
		normalized.EnvironmentSHA256, normalized.StdinPresent, stdin, stdinSHA256,
		normalized.WorkingDirectory, normalized.StartedAt, normalized.FinishedAt,
		normalized.DurationNanos, exitCode, launchError, observationError,
		normalized.Stdout, normalized.StdoutComplete, normalized.StdoutSHA256,
		normalized.Stderr, normalized.StderrComplete, normalized.StderrSHA256,
		workspaceBefore, workspaceAfter, string(normalized.Status))
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
