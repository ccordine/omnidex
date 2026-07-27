package queue

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/scrumcardllm"
	"github.com/jackc/pgx/v5"
)

type ScrumCardJobField uint8

const (
	ScrumCardTagsJob ScrumCardJobField = iota + 1
	ScrumCardTicketJob
)

var ErrScrumCardJobActive = errors.New("scrum card job is already active")

func (f ScrumCardJobField) column() (string, error) {
	switch f {
	case ScrumCardTagsJob:
		return "tags_job_id", nil
	case ScrumCardTicketJob:
		return "ticket_job_id", nil
	default:
		return "", fmt.Errorf("unsupported Scrum card job field %d", f)
	}
}

func (f ScrumCardJobField) action() (string, error) {
	switch f {
	case ScrumCardTagsJob:
		return scrumcardllm.ActionTagsSuggest, nil
	case ScrumCardTicketJob:
		return scrumcardllm.ActionCardTicket, nil
	default:
		return "", fmt.Errorf("unsupported Scrum card job field %d", f)
	}
}

// EnqueueScrumCardJob atomically locks the card, rejects a duplicate active
// job, inserts the new job, and links it to the card in one transaction.
func (r *Repository) EnqueueScrumCardJob(
	ctx context.Context,
	projectID int64,
	cardID string,
	field ScrumCardJobField,
	instruction string,
	metadataJSON []byte,
) (model.Job, error) {
	if r == nil || r.pool == nil {
		return model.Job{}, fmt.Errorf("postgres repository is not configured")
	}
	if ctx == nil {
		return model.Job{}, fmt.Errorf("Scrum card job context is required")
	}
	if projectID <= 0 {
		return model.Job{}, fmt.Errorf("project_id is required")
	}
	cardID = strings.TrimSpace(cardID)
	if cardID == "" {
		return model.Job{}, fmt.Errorf("scrum_card_id is required")
	}
	column, err := field.column()
	if err != nil {
		return model.Job{}, err
	}
	action, err := field.action()
	if err != nil {
		return model.Job{}, err
	}
	meta, err := scrumcardllm.ParseMetadata(metadataJSON)
	if err != nil {
		return model.Job{}, err
	}
	if meta.ProjectID != projectID || meta.CardID != cardID || meta.Action != action {
		return model.Job{}, fmt.Errorf(
			"Scrum card job metadata mismatch: want project=%d card=%q action=%q, got project=%d card=%q action=%q",
			projectID,
			cardID,
			action,
			meta.ProjectID,
			meta.CardID,
			meta.Action,
		)
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return model.Job{}, err
	}
	defer rollbackTx(ctx, tx, "Scrum card job enqueue")

	var existing string
	err = tx.QueryRow(ctx, `
		SELECT `+column+`
		FROM scrum_cards
		WHERE project_id = $1 AND id = $2
		FOR UPDATE
	`, projectID, cardID).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Job{}, fmt.Errorf("Scrum card %q was not found in project %d", cardID, projectID)
	}
	if err != nil {
		return model.Job{}, err
	}
	if err := validateExistingScrumCardJob(ctx, tx, projectID, cardID, action, existing); err != nil {
		return model.Job{}, err
	}

	job, err := r.enqueueJobTx(ctx, tx, instruction, model.PipelineScrumCardLLM, metadataJSON)
	if err != nil {
		return model.Job{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE scrum_cards
		SET `+column+` = $3, updated_at = NOW()
		WHERE project_id = $1 AND id = $2
	`, projectID, cardID, strconv.FormatInt(job.ID, 10))
	if err != nil {
		return model.Job{}, err
	}
	if tag.RowsAffected() != 1 {
		return model.Job{}, fmt.Errorf("Scrum card job link updated %d rows; expected 1", tag.RowsAffected())
	}
	if err := tx.Commit(ctx); err != nil {
		return model.Job{}, err
	}
	return job, nil
}

func validateExistingScrumCardJob(
	ctx context.Context,
	tx pgx.Tx,
	projectID int64,
	cardID, action, existing string,
) error {
	existing = strings.TrimSpace(existing)
	if existing == "" {
		return nil
	}
	jobID, err := strconv.ParseInt(existing, 10, 64)
	if err != nil || jobID <= 0 {
		return fmt.Errorf("card %q has invalid %s job id %q", cardID, action, existing)
	}
	var status string
	var linkedProjectID *int64
	var metadata []byte
	err = tx.QueryRow(ctx, `
		SELECT status, project_id, metadata
		FROM jobs
		WHERE id = $1
	`, jobID).Scan(&status, &linkedProjectID, &metadata)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("card %q links missing %s job %d", cardID, action, jobID)
	}
	if err != nil {
		return err
	}
	if linkedProjectID == nil || *linkedProjectID != projectID {
		return fmt.Errorf("card %q job %d has invalid project authority", cardID, jobID)
	}
	meta, err := scrumcardllm.ParseMetadata(metadata)
	if err != nil {
		return fmt.Errorf("card %q job %d metadata: %w", cardID, jobID, err)
	}
	if meta.ProjectID != projectID || meta.CardID != cardID || meta.Action != action {
		return fmt.Errorf("card %q job %d metadata does not match its card link", cardID, jobID)
	}
	switch status {
	case model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting:
		return fmt.Errorf("%w: card=%q action=%q job=%d status=%s", ErrScrumCardJobActive, cardID, action, jobID, status)
	case model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
		return nil
	default:
		return fmt.Errorf("card %q job %d has unsupported status %q", cardID, jobID, status)
	}
}
