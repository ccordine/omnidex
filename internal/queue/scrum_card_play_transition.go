package queue

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

type ScrumCardPlayCommand struct {
	ProjectID         int64
	CardID            string
	ExpectedUpdatedAt time.Time
	Pivot             bool
}

type ScrumCardPlayResult struct {
	Card          DBScrumCard
	Job           model.Job
	Action        string
	QueuePosition int
}

func (r *Repository) ApplyScrumCardPlay(
	ctx context.Context,
	command ScrumCardPlayCommand,
) (ScrumCardPlayResult, error) {
	if r == nil || r.pool == nil || ctx == nil || command.ProjectID <= 0 {
		return ScrumCardPlayResult{}, fmt.Errorf("PostgreSQL, context, and project are required for Scrum play")
	}
	if command.CardID == "" || command.CardID != strings.TrimSpace(command.CardID) {
		return ScrumCardPlayResult{}, fmt.Errorf("Scrum play requires one canonical card ID")
	}
	if command.ExpectedUpdatedAt.IsZero() {
		return ScrumCardPlayResult{}, fmt.Errorf("Scrum play requires the observed card revision")
	}
	tx, err := r.beginLockedProjectTx(ctx, command.ProjectID, "Scrum play")
	if err != nil {
		return ScrumCardPlayResult{}, err
	}
	defer rollbackTx(ctx, tx, "Scrum play")
	card, err := lockScrumCardTx(ctx, tx, command.ProjectID, command.CardID)
	if err != nil {
		return ScrumCardPlayResult{}, err
	}
	if !card.UpdatedAt.Equal(command.ExpectedUpdatedAt) {
		return ScrumCardPlayResult{}, scrumCardRevisionConflict(command.CardID)
	}
	running, found, err := findRunningScrumCardTx(ctx, tx, command.ProjectID)
	if err != nil {
		return ScrumCardPlayResult{}, err
	}
	if found && running.ID == card.ID {
		if command.Pivot || card.PlayState == "running" {
			if err := tx.Commit(ctx); err != nil {
				return ScrumCardPlayResult{}, err
			}
			return ScrumCardPlayResult{Card: card, Action: "already_running"}, nil
		}
		return ScrumCardPlayResult{}, fmt.Errorf("Scrum card %q has contradictory running selection", card.ID)
	}
	if found && !command.Pivot {
		return r.queueScrumCardPlayTx(ctx, tx, card)
	}
	if found {
		if err := pauseRunningScrumCardTx(ctx, tx, command, &running); err != nil {
			return ScrumCardPlayResult{}, err
		}
	}
	return r.startScrumCardPlayTx(ctx, tx, card)
}

func (r *Repository) PauseScrumCardPlayAtRevision(
	ctx context.Context,
	projectID int64,
	cardID string,
	expectedUpdatedAt time.Time,
) (DBScrumCard, error) {
	if r == nil || r.pool == nil || ctx == nil || projectID <= 0 ||
		cardID == "" || cardID != strings.TrimSpace(cardID) || expectedUpdatedAt.IsZero() {
		return DBScrumCard{}, fmt.Errorf("PostgreSQL, context, project, canonical card, and revision are required to pause Scrum play")
	}
	tx, err := r.beginLockedProjectTx(ctx, projectID, "pause Scrum play")
	if err != nil {
		return DBScrumCard{}, err
	}
	defer rollbackTx(ctx, tx, "pause Scrum play")
	card, err := lockScrumCardTx(ctx, tx, projectID, cardID)
	if err != nil {
		return DBScrumCard{}, err
	}
	if !card.UpdatedAt.Equal(expectedUpdatedAt) {
		return DBScrumCard{}, scrumCardRevisionConflict(cardID)
	}
	if card.PlayState != "running" && card.PlayState != "queued" {
		return DBScrumCard{}, fmt.Errorf("only active Scrum cards can be paused")
	}
	if card.PlayState == "running" {
		command := ScrumCardPlayCommand{ProjectID: projectID, CardID: cardID, ExpectedUpdatedAt: expectedUpdatedAt}
		if err := pauseRunningScrumCardTx(ctx, tx, command, &card); err != nil {
			return DBScrumCard{}, err
		}
	} else {
		if card.JobID != "" {
			return DBScrumCard{}, fmt.Errorf("queued Scrum card %q unexpectedly owns a job", card.ID)
		}
		previous := card
		if err := setScrumCardPausedTx(ctx, tx, &card, "Play paused"); err != nil {
			return DBScrumCard{}, err
		}
		if err := applyScrumCardStateMetricsTx(ctx, tx, previous, card, ""); err != nil {
			return DBScrumCard{}, err
		}
	}
	updated, err := scanDBScrumCard(tx.QueryRow(ctx, scrumCardSelectSQL, projectID, cardID))
	if err != nil {
		return DBScrumCard{}, err
	}
	if err := touchScrumPlayProjectTx(ctx, tx, projectID); err != nil {
		return DBScrumCard{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DBScrumCard{}, fmt.Errorf("commit paused Scrum play: %w", err)
	}
	return updated, nil
}

func (r *Repository) queueScrumCardPlayTx(
	ctx context.Context,
	tx pgx.Tx,
	card DBScrumCard,
) (ScrumCardPlayResult, error) {
	if card.PlayState == "queued" {
		if err := tx.Commit(ctx); err != nil {
			return ScrumCardPlayResult{}, err
		}
		return ScrumCardPlayResult{Card: card, Action: "already_queued"}, nil
	}
	previous := card
	var nextOrder, position int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(queue_order) FILTER (WHERE play_state='queued'),0)+1,
		       COUNT(*) FILTER (WHERE play_state='queued')+1
		FROM scrum_cards WHERE project_id=$1
	`, card.ProjectID).Scan(&nextOrder, &position); err != nil {
		return ScrumCardPlayResult{}, err
	}
	card.Column, card.PlayState, card.QueueOrder = "assigned", "queued", nextOrder
	if err := updateScrumCardFieldsTx(ctx, tx, card); err != nil {
		return ScrumCardPlayResult{}, err
	}
	if err := appendScrumPlayMessageTx(ctx, tx, card.ProjectID, card.ID, "system", fmt.Sprintf("Queued for play (#%d in assigned column)", nextOrder)); err != nil {
		return ScrumCardPlayResult{}, err
	}
	if err := applyScrumCardStateMetricsTx(ctx, tx, previous, card, ""); err != nil {
		return ScrumCardPlayResult{}, err
	}
	updated, err := scanDBScrumCard(tx.QueryRow(ctx, scrumCardSelectSQL, card.ProjectID, card.ID))
	if err != nil {
		return ScrumCardPlayResult{}, err
	}
	if err := touchScrumPlayProjectTx(ctx, tx, card.ProjectID); err != nil {
		return ScrumCardPlayResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ScrumCardPlayResult{}, fmt.Errorf("commit queued Scrum play: %w", err)
	}
	return ScrumCardPlayResult{Card: updated, Action: "queued", QueuePosition: position}, nil
}

func (r *Repository) startScrumCardPlayTx(
	ctx context.Context,
	tx pgx.Tx,
	card DBScrumCard,
) (ScrumCardPlayResult, error) {
	result, err := r.prepareScrumCardPlayTx(ctx, tx, card)
	if err != nil {
		return ScrumCardPlayResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ScrumCardPlayResult{}, fmt.Errorf("commit started Scrum play: %w", err)
	}
	return result, nil
}

func (r *Repository) prepareScrumCardPlayTx(
	ctx context.Context,
	tx pgx.Tx,
	card DBScrumCard,
) (ScrumCardPlayResult, error) {
	switch card.Column {
	case "ready", "assigned", "in_progress", "review":
	default:
		return ScrumCardPlayResult{}, fmt.Errorf("Scrum card %q column %q is not playable", card.ID, card.Column)
	}
	previous := card
	metadata, instruction, err := scrumPlayAuthorityTx(ctx, tx, card, r.modelAuthority)
	if err != nil {
		return ScrumCardPlayResult{}, err
	}
	job, err := r.enqueueScrumJobTx(ctx, tx, instruction, card.ProjectID, metadata)
	if err != nil {
		return ScrumCardPlayResult{}, err
	}
	card.Column, card.PlayState, card.QueueOrder = "in_progress", "running", 0
	card.JobID = strconv.FormatInt(job.ID, 10)
	if err := updateScrumCardFieldsTx(ctx, tx, card); err != nil {
		return ScrumCardPlayResult{}, err
	}
	if err := appendScrumPlayMessageTx(ctx, tx, card.ProjectID, card.ID, "system", fmt.Sprintf("Job #%d queued", job.ID)); err != nil {
		return ScrumCardPlayResult{}, err
	}
	if err := appendScrumPlayMessageTx(ctx, tx, card.ProjectID, card.ID, "user", instruction); err != nil {
		return ScrumCardPlayResult{}, err
	}
	if err := applyScrumCardStateMetricsTx(ctx, tx, previous, card, ""); err != nil {
		return ScrumCardPlayResult{}, err
	}
	updated, err := scanDBScrumCard(tx.QueryRow(ctx, scrumCardSelectSQL, card.ProjectID, card.ID))
	if err != nil {
		return ScrumCardPlayResult{}, err
	}
	return ScrumCardPlayResult{Card: updated, Job: job, Action: "started"}, nil
}

func pauseRunningScrumCardTx(
	ctx context.Context,
	tx pgx.Tx,
	command ScrumCardPlayCommand,
	card *DBScrumCard,
) error {
	if card == nil || card.PlayState != "running" || card.JobID == "" {
		return fmt.Errorf("running Scrum card has invalid job authority")
	}
	jobID, err := strconv.ParseInt(card.JobID, 10, 64)
	if err != nil || jobID <= 0 || strconv.FormatInt(jobID, 10) != card.JobID {
		return fmt.Errorf("running Scrum card %q has noncanonical job ID %q", card.ID, card.JobID)
	}
	operationID, err := NewLifecycleOperationID(
		"scrum-card-play-pause-v2", strconv.FormatInt(command.ProjectID, 10),
		command.CardID, command.ExpectedUpdatedAt.UTC().Format(time.RFC3339Nano),
		card.ID, card.JobID,
	)
	if err != nil {
		return err
	}
	if _, err := cancelJobTx(ctx, tx, CancelJobCommand{
		OperationID: operationID, JobID: jobID, Reason: "paused by typed Scrum play transition",
	}); err != nil {
		return err
	}
	previous := *card
	if err := setScrumCardPausedTx(ctx, tx, card, "Play paused"); err != nil {
		return err
	}
	return applyScrumCardStateMetricsTx(ctx, tx, previous, *card, "")
}

func setScrumCardPausedTx(ctx context.Context, tx pgx.Tx, card *DBScrumCard, note string) error {
	card.Column, card.PlayState, card.QueueOrder = "assigned", "paused", 0
	card.JobID = ""
	if err := updateScrumCardFieldsTx(ctx, tx, *card); err != nil {
		return err
	}
	return appendScrumPlayMessageTx(ctx, tx, card.ProjectID, card.ID, "system", note)
}

func findRunningScrumCardTx(ctx context.Context, tx pgx.Tx, projectID int64) (DBScrumCard, bool, error) {
	rows, err := tx.Query(ctx, scrumCardSelectionSQL+` AND play_state='running' ORDER BY id FOR UPDATE`, projectID)
	if err != nil {
		return DBScrumCard{}, false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return DBScrumCard{}, false, rows.Err()
	}
	card, err := scanDBScrumCard(rows)
	if err != nil {
		return DBScrumCard{}, false, err
	}
	if rows.Next() {
		return DBScrumCard{}, false, fmt.Errorf("Scrum project %d has multiple running cards", projectID)
	}
	return card, true, rows.Err()
}
