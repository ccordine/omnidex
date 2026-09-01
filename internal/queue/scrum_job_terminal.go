package queue

import (
	"context"
	"fmt"
	"strconv"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// terminalizeScrumJobCardTx projects one terminal Scrum job onto its single
// owning card in the same transaction. The card is not a second eventual
// authority and no HTTP callback is required to repair it afterward.
func terminalizeScrumJobCardTx(
	ctx context.Context,
	tx pgx.Tx,
	job model.Job,
	status string,
) error {
	if job.Pipeline != model.PipelineScrum {
		return nil
	}
	var projectID int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(project_id, 0) FROM jobs WHERE id=$1
	`, job.ID).Scan(&projectID); err != nil {
		return fmt.Errorf("load Scrum job project authority: %w", err)
	}
	if projectID <= 0 {
		return fmt.Errorf("Scrum job %d has no project authority", job.ID)
	}
	jobID := strconv.FormatInt(job.ID, 10)
	rows, err := tx.Query(ctx, `
		SELECT `+scrumCardSelectColumns+`
		FROM scrum_cards
		WHERE project_id=$1 AND job_id=$2
		FOR UPDATE
	`, projectID, jobID)
	if err != nil {
		return fmt.Errorf("lock Scrum card for terminal job %d: %w", job.ID, err)
	}
	defer rows.Close()
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return err
		}
		return fmt.Errorf("Scrum job %d has no owning card", job.ID)
	}
	card, err := scanDBScrumCard(rows)
	if err != nil {
		return err
	}
	if rows.Next() {
		return fmt.Errorf("Scrum job %d is owned by multiple cards", job.ID)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	var outcome string
	expectedColumn := ""
	switch status {
	case model.JobStatusCompleted:
		expectedColumn, outcome = "review", "success"
	case model.JobStatusFailed, model.JobStatusCanceled:
		expectedColumn, outcome = "error", "failed"
	default:
		return fmt.Errorf("Scrum job %d cannot project nonterminal status %q", job.ID, status)
	}
	if card.PlayState == "" && card.Column == expectedColumn && card.QueueOrder == 0 && card.JobID == jobID {
		return nil
	}
	if card.PlayState != "running" || card.Column != "in_progress" || card.JobID != jobID {
		return fmt.Errorf(
			"Scrum job %d terminal state contradicts card %q state column=%q play=%q job=%q",
			job.ID, card.ID, card.Column, card.PlayState, card.JobID,
		)
	}

	next := card
	next.Column = expectedColumn
	next.PlayState, next.QueueOrder = "", 0
	tag, err := tx.Exec(ctx, `
		UPDATE scrum_cards
		SET column_name=$3, play_state='', queue_order=0,
		    updated_at=GREATEST(clock_timestamp(), updated_at+interval '1 microsecond')
		WHERE project_id=$1 AND id=$2 AND job_id=$4 AND play_state='running'
	`, projectID, card.ID, next.Column, jobID)
	if err != nil {
		return fmt.Errorf("project terminal Scrum job %d: %w", job.ID, err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("Scrum job %d lost card terminalization authority", job.ID)
	}
	if err := applyScrumCardStateMetricsTx(ctx, tx, card, next, outcome); err != nil {
		return err
	}
	return touchScrumPlayProjectTx(ctx, tx, projectID)
}
