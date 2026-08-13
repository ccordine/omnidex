package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

const maxReplanFeedbackBytes = 64 * 1024

func (r *Repository) ReplanJob(ctx context.Context, command ReplanJobCommand) (model.Job, error) {
	command, feedbackSHA, err := normalizeReplanJobCommand(command)
	if err != nil {
		return model.Job{}, err
	}
	descriptor, err := describeLifecycleOperation(command.OperationID, LifecycleReplanJob, command)
	if err != nil {
		return model.Job{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Job{}, fmt.Errorf("begin replan for job %d: %w", command.JobID, err)
	}
	defer tx.Rollback(ctx)
	job, err := replanJobTx(ctx, tx, command, feedbackSHA, descriptor)
	if err != nil {
		return model.Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Job{}, fmt.Errorf("commit generation %d for job %d: %w", job.CurrentGeneration, command.JobID, err)
	}
	return job, nil
}

func replanJobTx(
	ctx context.Context,
	tx pgx.Tx,
	command ReplanJobCommand,
	feedbackSHA string,
	descriptor lifecycleOperationDescriptor,
) (model.Job, error) {
	job, err := lockedJobTx(ctx, tx, command.JobID)
	if err != nil {
		return model.Job{}, err
	}
	if err := lockLifecycleOperationIdentityTx(ctx, tx, command.OperationID); err != nil {
		return model.Job{}, err
	}
	if existing, found, err := loadLifecycleOperationTx(ctx, tx, descriptor, command.JobID); err != nil {
		return model.Job{}, err
	} else if found {
		if err := requireReplanReplayTx(ctx, tx, existing, command, feedbackSHA); err != nil {
			return model.Job{}, err
		}
		return existing.ResultJob, nil
	}
	if terminalJobStatus(job.Status) {
		return model.Job{}, fmt.Errorf("job is already %s", job.Status)
	}
	if err := rejectUnresolvedRepositoryMutationsTx(
		ctx, tx, command.JobID, job.CurrentGeneration,
	); err != nil {
		return model.Job{}, err
	}
	currentGeneration, err := lockCurrentJobGenerationTx(ctx, tx, command.JobID)
	if err != nil {
		return model.Job{}, err
	}
	if currentGeneration >= math.MaxInt64 {
		return model.Job{}, fmt.Errorf("%w: job %d exhausted generation capacity", ErrInvalidJobGeneration, command.JobID)
	}
	if err := lockGenerationRecordTx(ctx, tx, command.JobID, currentGeneration); err != nil {
		return model.Job{}, err
	}
	seeds, err := canonicalReplanStepsTx(ctx, tx, job)
	if err != nil {
		return model.Job{}, fmt.Errorf("recompute canonical steps for job %d: %w", command.JobID, err)
	}
	boundary, err := canonicalReplanTail(seeds)
	if err != nil {
		return model.Job{}, fmt.Errorf("job %d cannot be replanned: %w", command.JobID, err)
	}
	currentTail, err := lockCurrentReplanTailTx(ctx, tx, command.JobID, boundary.sortIndex)
	if err != nil {
		return model.Job{}, err
	}
	retiringIDs, err := validateCurrentReplanTail(currentGeneration, boundary, currentTail)
	if err != nil {
		return model.Job{}, fmt.Errorf("validate replan tail for job %d: %w", command.JobID, err)
	}
	header, err := loadTaskLedgerHeaderTx(ctx, tx, command.JobID, true)
	if err != nil {
		return model.Job{}, err
	}
	if header.Status != taskstate.LedgerActive {
		return model.Job{}, fmt.Errorf(
			"%w: nonterminal job %d has %s task ledger", ErrInvalidJobGeneration, command.JobID, header.Status,
		)
	}
	if err := rejectAssignedRetiringStepsTx(ctx, tx, command.JobID, retiringIDs); err != nil {
		return model.Job{}, err
	}
	if err := terminalizeStepAttemptsForAuthorityChangeTx(
		ctx, tx, command.JobID, currentGeneration, retiringIDs, model.StepAttemptSuperseded,
	); err != nil {
		return model.Job{}, err
	}
	newGeneration := currentGeneration + 1
	if err := createReplanGenerationTx(
		ctx, tx, command, feedbackSHA, currentGeneration, newGeneration, boundary,
	); err != nil {
		return model.Job{}, err
	}
	if err := retireReplanTailTx(ctx, tx, command.JobID, retiringIDs, newGeneration); err != nil {
		return model.Job{}, err
	}
	if err := insertReplanTailTx(ctx, tx, command.JobID, newGeneration, boundary.seeds); err != nil {
		return model.Job{}, err
	}
	job, err = advanceReplannedJobTx(
		ctx, tx, command, feedbackSHA, currentGeneration, newGeneration, boundary.action,
	)
	if err != nil {
		return model.Job{}, err
	}
	if err := insertLifecycleOperationTx(ctx, tx, descriptor, lifecycleOperationRecord{
		ID: descriptor.ID, JobID: command.JobID, ObservedGeneration: currentGeneration,
		ResultGeneration: newGeneration, Kind: descriptor.Kind, CommandSHA256: descriptor.SHA256,
		ResultJobStatus: job.Status, ResultJob: job,
	}); err != nil {
		return model.Job{}, err
	}
	return job, nil
}

func validateReplanFeedback(feedback string) (string, string, error) {
	return validateLifecycleFeedback(feedback, "replan feedback")
}

func validateLifecycleFeedback(feedback, subject string) (string, string, error) {
	if !utf8.ValidString(feedback) {
		return "", "", fmt.Errorf("%s must be valid UTF-8", subject)
	}
	feedback = strings.TrimSpace(feedback)
	if feedback == "" {
		return "", "", fmt.Errorf("feedback is required")
	}
	if strings.ContainsRune(feedback, '\x00') {
		return "", "", fmt.Errorf("%s must not contain NUL", subject)
	}
	if len(feedback) > maxReplanFeedbackBytes {
		return "", "", fmt.Errorf("%s exceeds the %d-byte limit", subject, maxReplanFeedbackBytes)
	}
	digest := sha256.Sum256([]byte(feedback))
	return feedback, hex.EncodeToString(digest[:]), nil
}

func terminalJobStatus(status string) bool {
	return status == model.JobStatusCanceled || status == model.JobStatusCompleted || status == model.JobStatusFailed
}

func lockCurrentJobGenerationTx(ctx context.Context, tx pgx.Tx, jobID int64) (int64, error) {
	var generation int64
	if err := tx.QueryRow(ctx, `SELECT current_generation FROM jobs WHERE id=$1`, jobID).Scan(&generation); err != nil {
		return 0, fmt.Errorf("read current generation for job %d: %w", jobID, err)
	}
	if generation <= 0 {
		return 0, fmt.Errorf("%w: job %d has invalid generation %d", ErrInvalidJobGeneration, jobID, generation)
	}
	return generation, nil
}

func lockGenerationRecordTx(ctx context.Context, tx pgx.Tx, jobID, generation int64) error {
	var persisted int64
	err := tx.QueryRow(ctx, `
		SELECT generation FROM job_generations
		WHERE job_id=$1 AND generation=$2
		FOR UPDATE
	`, jobID, generation).Scan(&persisted)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: job %d has no current generation record", ErrInvalidJobGeneration, jobID)
	}
	if err != nil {
		return fmt.Errorf("lock generation %d for job %d: %w", generation, jobID, err)
	}
	return nil
}
