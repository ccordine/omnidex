package queue

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type ScrumCardReconcileKind string

const (
	ScrumReconcileJobProgress    ScrumCardReconcileKind = "job_progress"
	ScrumReconcileJobTerminal    ScrumCardReconcileKind = "job_terminal"
	ScrumReconcileAutoWorkReady  ScrumCardReconcileKind = "auto_work_ready"
	ScrumReconcileAutoWorkFailed ScrumCardReconcileKind = "auto_work_failed"
)

type ScrumCardReconcileCommand struct {
	ProjectID         int64
	CardID            string
	ExpectedUpdatedAt time.Time
	ExpectedJobID     string
	Kind              ScrumCardReconcileKind
	Column            ScrumCardColumn
	PlayState         string
	QueueOrder        int
	JobID             string
	SyncJobID         string
	StepContextCursor int64
	Messages          []ScrumCardMessageAppend
	Outcome           string
}

// ReconcileScrumCard applies one code-owned runtime projection at the exact
// observed card revision. The operational row, canonical message appends,
// lifecycle evidence, and relation-backed flow metrics commit together.
func (r *Repository) ReconcileScrumCard(
	ctx context.Context,
	command ScrumCardReconcileCommand,
) (DBScrumCard, error) {
	if r == nil || r.pool == nil || ctx == nil || command.ProjectID <= 0 {
		return DBScrumCard{}, fmt.Errorf("PostgreSQL, context, and project are required for Scrum reconciliation")
	}
	if command.CardID == "" || command.CardID != strings.TrimSpace(command.CardID) || command.ExpectedUpdatedAt.IsZero() {
		return DBScrumCard{}, fmt.Errorf("Scrum reconciliation requires a canonical card and observed revision")
	}
	if err := validateScrumCardReconcileCommand(command); err != nil {
		return DBScrumCard{}, err
	}
	tx, err := r.beginLockedProjectTx(ctx, command.ProjectID, "reconcile Scrum card")
	if err != nil {
		return DBScrumCard{}, err
	}
	defer rollbackTx(ctx, tx, "reconcile Scrum card")
	current, err := lockScrumCardTx(ctx, tx, command.ProjectID, command.CardID)
	if err != nil {
		return DBScrumCard{}, err
	}
	if !current.UpdatedAt.Equal(command.ExpectedUpdatedAt) {
		return DBScrumCard{}, scrumCardRevisionConflict(command.CardID)
	}
	if current.JobID != command.ExpectedJobID {
		return DBScrumCard{}, fmt.Errorf("Scrum card %q job authority changed before reconciliation", command.CardID)
	}
	if err := validateScrumReconcileTransition(current, command); err != nil {
		return DBScrumCard{}, err
	}
	next := current
	next.Column, next.PlayState, next.QueueOrder = string(command.Column), command.PlayState, command.QueueOrder
	next.JobID, next.SyncJobID, next.StepContextCursor = command.JobID, command.SyncJobID, command.StepContextCursor
	tag, err := tx.Exec(ctx, `
		UPDATE scrum_cards SET column_name=$3,play_state=$4,queue_order=$5,
		 job_id=$6,sync_job_id=$7,step_context_cursor=$8,
		 updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond')
		WHERE project_id=$1 AND id=$2 AND updated_at=$9
	`, command.ProjectID, command.CardID, command.Column, command.PlayState, command.QueueOrder,
		command.JobID, command.SyncJobID, command.StepContextCursor, command.ExpectedUpdatedAt)
	if err != nil {
		return DBScrumCard{}, fmt.Errorf("apply typed Scrum reconciliation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return DBScrumCard{}, scrumCardRevisionConflict(command.CardID)
	}
	for index, message := range command.Messages {
		if _, err := insertScrumCardMessageTx(ctx, tx, command.ProjectID, command.CardID, message); err != nil {
			return DBScrumCard{}, fmt.Errorf("append Scrum reconciliation message %d: %w", index+1, err)
		}
	}
	if err := applyScrumCardStateMetricsTx(ctx, tx, current, next, command.Outcome); err != nil {
		return DBScrumCard{}, err
	}
	if err := touchScrumPlayProjectTx(ctx, tx, command.ProjectID); err != nil {
		return DBScrumCard{}, err
	}
	updated, err := scanDBScrumCard(tx.QueryRow(ctx, scrumCardSelectSQL, command.ProjectID, command.CardID))
	if err != nil {
		return DBScrumCard{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DBScrumCard{}, fmt.Errorf("commit Scrum reconciliation: %w", err)
	}
	return updated, nil
}

func validateScrumCardReconcileCommand(command ScrumCardReconcileCommand) error {
	switch command.Kind {
	case ScrumReconcileJobProgress, ScrumReconcileJobTerminal,
		ScrumReconcileAutoWorkReady, ScrumReconcileAutoWorkFailed:
	default:
		return fmt.Errorf("Scrum reconciliation kind %q is not registered", command.Kind)
	}
	if _, err := ParseScrumCardColumn(string(command.Column)); err != nil {
		return err
	}
	for name, value := range map[string]string{
		"play state": command.PlayState, "job ID": command.JobID,
		"sync job ID": command.SyncJobID, "expected job ID": command.ExpectedJobID,
		"outcome": command.Outcome,
	} {
		if value != strings.TrimSpace(value) {
			return fmt.Errorf("Scrum reconciliation %s is not canonical", name)
		}
	}
	if command.QueueOrder < 0 || command.StepContextCursor < 0 {
		return fmt.Errorf("Scrum reconciliation queue order and cursor must be non-negative")
	}
	for _, jobID := range []string{command.ExpectedJobID, command.JobID, command.SyncJobID} {
		if jobID == "" {
			continue
		}
		parsed, err := strconv.ParseInt(jobID, 10, 64)
		if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != jobID {
			return fmt.Errorf("Scrum reconciliation job ID %q is not canonical", jobID)
		}
	}
	for index, message := range command.Messages {
		if _, err := normalizeScrumCardMessageAppend(message); err != nil {
			return fmt.Errorf("validate Scrum reconciliation message %d: %w", index+1, err)
		}
	}
	return nil
}

func validateScrumReconcileTransition(current DBScrumCard, command ScrumCardReconcileCommand) error {
	if command.JobID != current.JobID {
		return fmt.Errorf("Scrum reconciliation may not replace card job identity")
	}
	switch command.PlayState {
	case "":
		if command.QueueOrder != 0 {
			return fmt.Errorf("inactive Scrum reconciliation requires zero queue order")
		}
	case "running":
		if command.QueueOrder != 0 || command.JobID == "" || command.SyncJobID != command.JobID {
			return fmt.Errorf("running Scrum reconciliation requires one exact job and zero queue order")
		}
	case "paused":
		if command.QueueOrder != 0 || command.JobID != "" || command.SyncJobID != "" || command.StepContextCursor != 0 {
			return fmt.Errorf("paused Scrum reconciliation may not retain job authority")
		}
	case "queued":
		return fmt.Errorf("queued Scrum state is owned only by the play transaction")
	default:
		return fmt.Errorf("Scrum reconciliation play state %q is not registered", command.PlayState)
	}
	if command.SyncJobID == "" && command.StepContextCursor != 0 {
		return fmt.Errorf("Scrum reconciliation cursor requires exact sync job authority")
	}
	if command.SyncJobID != "" && command.SyncJobID != command.JobID {
		return fmt.Errorf("Scrum reconciliation sync job differs from card job")
	}
	switch command.Kind {
	case ScrumReconcileJobProgress:
		if command.Outcome != "" || current.JobID == "" || command.Column != ScrumCardColumn(current.Column) || command.PlayState != current.PlayState {
			return fmt.Errorf("job-progress reconciliation may only advance typed output rows and cursor")
		}
	case ScrumReconcileJobTerminal:
		if current.PlayState != "running" || command.PlayState != "" || current.JobID == "" {
			return fmt.Errorf("terminal reconciliation requires one running job transition")
		}
		switch command.Outcome {
		case "success", "failed":
		default:
			return fmt.Errorf("terminal Scrum reconciliation outcome %q is not registered", command.Outcome)
		}
	case ScrumReconcileAutoWorkReady:
		if command.Outcome != "" || current.JobID != "" || command.JobID != "" || command.Column != ScrumCardColumn(current.Column) ||
			(current.PlayState != "" && current.PlayState != "paused") || command.PlayState != "" {
			return fmt.Errorf("auto-work preparation has invalid Scrum transition authority")
		}
	case ScrumReconcileAutoWorkFailed:
		if command.Outcome != "" || current.JobID != "" || command.JobID != "" || command.Column != ScrumCardError || command.PlayState != "" {
			return fmt.Errorf("auto-work failure has invalid Scrum transition authority")
		}
	}
	return nil
}
