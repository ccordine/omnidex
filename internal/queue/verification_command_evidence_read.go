package queue

import (
	"context"
	"encoding/json"
	"fmt"
)

const verificationCommandEvidenceColumns = `
	commands.id,commands.job_id,commands.generation,commands.step_id,
	commands.step_attempt,commands.worker_id,commands.phase,commands.ordinal,
	commands.argv,commands.argv_sha256,commands.environment,commands.environment_sha256,
	commands.stdin_present,commands.stdin,commands.stdin_sha256,
	commands.working_directory,commands.started_at,commands.finished_at,
	commands.duration_nanos,commands.exit_code,commands.launch_error,commands.observation_error,
	commands.stdout,commands.stdout_complete,commands.stdout_sha256,
	commands.stderr,commands.stderr_complete,commands.stderr_sha256,
	commands.workspace_sha256_before,commands.workspace_sha256_after,
	commands.status,commands.created_at`

func (r *Repository) ListVerificationCommandEvidenceForJob(
	ctx context.Context,
	jobID, afterID int64,
	limit int,
) ([]VerificationCommandEvidence, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return nil, fmt.Errorf("verification command history requires context and PostgreSQL")
	}
	if jobID < 1 || afterID < 0 || limit < 1 || limit > MaxVerificationCommandEvidencePageSize {
		return nil, fmt.Errorf(
			"verification command history requires a positive job, nonnegative cursor, and limit between 1 and %d",
			MaxVerificationCommandEvidencePageSize,
		)
	}
	rows, err := r.pool.Query(ctx, `SELECT `+verificationCommandEvidenceColumns+`
		FROM verification_command_evidence AS commands
		WHERE commands.job_id=$1 AND commands.id>$2
		ORDER BY commands.id ASC
		LIMIT $3
	`, jobID, afterID, limit)
	if err != nil {
		return nil, fmt.Errorf("list verification command evidence: %w", err)
	}
	defer rows.Close()
	items := make([]VerificationCommandEvidence, 0, limit)
	for rows.Next() {
		var item VerificationCommandEvidence
		if err := scanVerificationCommandEvidence(rows, &item); err != nil {
			return nil, fmt.Errorf("scan verification command evidence: %w", err)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate verification command evidence: %w", err)
	}
	return items, nil
}

type verificationCommandScanner interface {
	Scan(dest ...any) error
}

func scanVerificationCommandEvidence(
	scanner verificationCommandScanner,
	item *VerificationCommandEvidence,
) error {
	var argvJSON, environmentJSON []byte
	var stdin []byte
	var stdinSHA256, launchError, observationError, workspaceBefore, workspaceAfter *string
	if err := scanner.Scan(
		&item.ID, &item.Authority.JobID, &item.Authority.Generation,
		&item.Authority.StepID, &item.Authority.Attempt, &item.Authority.WorkerID,
		&item.Phase, &item.Ordinal, &argvJSON, &item.ArgvSHA256,
		&environmentJSON, &item.EnvironmentSHA256,
		&item.StdinPresent, &stdin, &stdinSHA256, &item.WorkingDirectory,
		&item.StartedAt, &item.FinishedAt, &item.DurationNanos,
		&item.ExitCode, &launchError, &observationError, &item.Stdout, &item.StdoutComplete,
		&item.StdoutSHA256, &item.Stderr, &item.StderrComplete,
		&item.StderrSHA256, &workspaceBefore, &workspaceAfter,
		&item.Status, &item.CreatedAt,
	); err != nil {
		return err
	}
	if err := json.Unmarshal(argvJSON, &item.Argv); err != nil {
		return fmt.Errorf("decode exact verification argv: %w", err)
	}
	if err := json.Unmarshal(environmentJSON, &item.Environment); err != nil {
		return fmt.Errorf("decode exact verification environment: %w", err)
	}
	if item.StdinPresent {
		item.Stdin = make([]byte, len(stdin))
		copy(item.Stdin, stdin)
	}
	if stdinSHA256 != nil {
		item.StdinSHA256 = *stdinSHA256
	}
	if launchError != nil {
		item.LaunchError = *launchError
	}
	if observationError != nil {
		item.ObservationError = *observationError
	}
	if workspaceBefore != nil {
		item.WorkspaceSHA256Before = *workspaceBefore
	}
	if workspaceAfter != nil {
		item.WorkspaceSHA256After = *workspaceAfter
	}
	exactStdout := make([]byte, len(item.Stdout))
	copy(exactStdout, item.Stdout)
	item.Stdout = exactStdout
	exactStderr := make([]byte, len(item.Stderr))
	copy(exactStderr, item.Stderr)
	item.Stderr = exactStderr
	return nil
}
