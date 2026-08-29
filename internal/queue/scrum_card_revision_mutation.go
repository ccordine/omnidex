package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

var ErrScrumCardActiveDelete = errors.New("active Scrum card deletion is forbidden")

// ScrumCardRevisionPatch is the complete browser-authored card edit surface.
// Nil fields are absent; every non-nil field is applied. Operational card
// transitions use separate typed methods and cannot enter through this DTO.
type ScrumCardRevisionPatch struct {
	Title       *string
	Description *string
	RefFiles    *[]string
	CardTicket  *string
	CardPrompt  *string
	Tags        *[]string
}

// UpdateScrumCardAtRevision applies an exact browser-authored card patch only
// to the server revision the browser observed. Operational internal updates use
// their own typed transitions and cannot silently substitute for this rail.
func (r *Repository) UpdateScrumCardAtRevision(
	ctx context.Context,
	projectID int64,
	cardID string,
	expectedUpdatedAt time.Time,
	patch ScrumCardRevisionPatch,
) (DBScrumCard, error) {
	if cardID == "" || cardID != strings.TrimSpace(cardID) {
		return DBScrumCard{}, fmt.Errorf("revision-bound Scrum edit requires one canonical card ID")
	}
	if r == nil || r.pool == nil || ctx == nil || projectID <= 0 {
		return DBScrumCard{}, fmt.Errorf("PostgreSQL, context, project, and card are required for revision-bound Scrum edit")
	}
	if expectedUpdatedAt.IsZero() {
		return DBScrumCard{}, fmt.Errorf("revision-bound Scrum edit requires an expected card revision")
	}
	if err := patch.validate(); err != nil {
		return DBScrumCard{}, err
	}
	tx, err := r.beginLockedProjectTx(ctx, projectID, "revision-bound Scrum edit")
	if err != nil {
		return DBScrumCard{}, err
	}
	defer rollbackTx(ctx, tx, "revision-bound Scrum edit")
	current, err := lockScrumCardTx(ctx, tx, projectID, cardID)
	if err != nil {
		return DBScrumCard{}, err
	}
	if !current.UpdatedAt.Equal(expectedUpdatedAt) {
		return DBScrumCard{}, scrumCardRevisionConflict(cardID)
	}
	if err := patch.applyTo(&current); err != nil {
		return DBScrumCard{}, err
	}
	if err := validateDBScrumCursorAuthority(current); err != nil {
		return DBScrumCard{}, err
	}
	if err := updateScrumCardFieldsTx(ctx, tx, current); err != nil {
		return DBScrumCard{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE scrum_cards SET updated_at=GREATEST(clock_timestamp(),$3::timestamptz+interval '1 microsecond')
		WHERE project_id=$1 AND id=$2
	`, projectID, cardID, expectedUpdatedAt); err != nil {
		return DBScrumCard{}, fmt.Errorf("advance revision-bound Scrum edit timestamp: %w", err)
	}
	if err := refreshScrumFlowMetricsTx(ctx, tx, projectID, cardID); err != nil {
		return DBScrumCard{}, fmt.Errorf("refresh revision-bound Scrum edit flow metrics: %w", err)
	}
	updated, err := scanDBScrumCard(tx.QueryRow(ctx, scrumCardSelectSQL, projectID, cardID))
	if err != nil {
		return DBScrumCard{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE projects SET last_seen_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1
	`, projectID); err != nil {
		return DBScrumCard{}, fmt.Errorf("touch revision-bound Scrum edit project: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return DBScrumCard{}, fmt.Errorf("commit revision-bound Scrum edit: %w", err)
	}
	return updated, nil
}

func (patch ScrumCardRevisionPatch) validate() error {
	if patch.Title == nil && patch.Description == nil && patch.RefFiles == nil &&
		patch.CardTicket == nil && patch.CardPrompt == nil && patch.Tags == nil {
		return fmt.Errorf("revision-bound Scrum edit requires at least one editable field")
	}
	for name, value := range map[string]*string{
		"title": patch.Title, "description": patch.Description,
		"card_ticket": patch.CardTicket, "card_prompt": patch.CardPrompt,
	} {
		if value != nil && (!utf8.ValidString(*value) || strings.ContainsRune(*value, '\x00')) {
			return fmt.Errorf("revision-bound Scrum %s must be valid UTF-8 without NUL", name)
		}
	}
	if patch.Title != nil && strings.TrimSpace(*patch.Title) == "" {
		return fmt.Errorf("revision-bound Scrum title must not be blank")
	}
	for name, values := range map[string]*[]string{"reference file": patch.RefFiles, "tag": patch.Tags} {
		if values == nil {
			continue
		}
		for index, value := range *values {
			if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
				return fmt.Errorf("revision-bound Scrum %s %d must be valid UTF-8 without NUL", name, index+1)
			}
		}
	}
	return nil
}

func (patch ScrumCardRevisionPatch) applyTo(card *DBScrumCard) error {
	if card == nil {
		return fmt.Errorf("revision-bound Scrum edit requires a locked card")
	}
	if patch.Title != nil {
		card.Title = *patch.Title
	}
	if patch.Description != nil {
		card.Description = *patch.Description
	}
	if patch.CardTicket != nil {
		card.CardTicket = *patch.CardTicket
	}
	if patch.CardPrompt != nil {
		card.CardPrompt = *patch.CardPrompt
	}
	for name, values := range map[string]*[]string{"reference files": patch.RefFiles, "tags": patch.Tags} {
		if values == nil {
			continue
		}
		encoded, err := json.Marshal(*values)
		if err != nil {
			return fmt.Errorf("encode revision-bound Scrum %s: %w", name, err)
		}
		if name == "reference files" {
			card.RefFiles = encoded
		} else {
			card.Tags = encoded
		}
	}
	return nil
}

// DeleteScrumCardAtRevision prevents a stale card view from deleting a newer
// server state. The revision check and deletion share one locked transaction.
func (r *Repository) DeleteScrumCardAtRevision(
	ctx context.Context,
	projectID int64,
	cardID string,
	expectedUpdatedAt time.Time,
) error {
	if cardID == "" || cardID != strings.TrimSpace(cardID) {
		return fmt.Errorf("revision-bound Scrum deletion requires one canonical card ID")
	}
	if r == nil || r.pool == nil || ctx == nil || projectID <= 0 {
		return fmt.Errorf("PostgreSQL, context, project, and card are required for revision-bound Scrum deletion")
	}
	if expectedUpdatedAt.IsZero() {
		return fmt.Errorf("revision-bound Scrum deletion requires an expected card revision")
	}
	tx, err := r.beginLockedProjectTx(ctx, projectID, "revision-bound Scrum deletion")
	if err != nil {
		return err
	}
	defer rollbackTx(ctx, tx, "revision-bound Scrum deletion")
	current, err := lockScrumCardTx(ctx, tx, projectID, cardID)
	if err != nil {
		return err
	}
	if !current.UpdatedAt.Equal(expectedUpdatedAt) {
		return scrumCardRevisionConflict(cardID)
	}
	if err := requireInactiveScrumCardForDeleteTx(ctx, tx, current); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM scrum_cards WHERE project_id=$1 AND id=$2`, projectID, cardID)
	if err != nil {
		return fmt.Errorf("delete revision-bound Scrum card: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("%w: Scrum card %q disappeared during deletion", ErrScrumCardNotFound, cardID)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE projects SET last_seen_at=clock_timestamp(),updated_at=clock_timestamp() WHERE id=$1
	`, projectID); err != nil {
		return fmt.Errorf("touch revision-bound Scrum deletion project: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit revision-bound Scrum deletion: %w", err)
	}
	return nil
}

func requireInactiveScrumCardForDeleteTx(ctx context.Context, tx pgx.Tx, card DBScrumCard) error {
	if card.PlayState == "running" || card.PlayState == "queued" || card.SyncJobID != "" {
		return fmt.Errorf(
			"%w: card %q has play_state=%q and sync_job_id=%q; pause it before deletion",
			ErrScrumCardActiveDelete, card.ID, card.PlayState, card.SyncJobID,
		)
	}
	if card.JobID == "" {
		return nil
	}
	jobID, err := strconv.ParseInt(card.JobID, 10, 64)
	if err != nil || jobID <= 0 || strconv.FormatInt(jobID, 10) != card.JobID {
		return fmt.Errorf("%w: card %q has noncanonical job identity %q", ErrScrumCardActiveDelete, card.ID, card.JobID)
	}
	var projectID int64
	var pipeline, status string
	err = tx.QueryRow(ctx, `SELECT project_id,pipeline,status FROM jobs WHERE id=$1 FOR SHARE`, jobID).
		Scan(&projectID, &pipeline, &status)
	if err != nil {
		return fmt.Errorf("%w: load card %q job %d: %v", ErrScrumCardActiveDelete, card.ID, jobID, err)
	}
	if projectID != card.ProjectID || pipeline != model.PipelineScrum {
		return fmt.Errorf("%w: card %q job %d is not its project-bound Scrum job", ErrScrumCardActiveDelete, card.ID, jobID)
	}
	if !terminalJobStatus(status) {
		return fmt.Errorf("%w: card %q job %d remains %q", ErrScrumCardActiveDelete, card.ID, jobID, status)
	}
	return nil
}

func scrumCardRevisionConflict(cardID string) error {
	return fmt.Errorf(
		"%w: Scrum card %q changed; reload server state and retry",
		ErrScrumCardVersionConflict,
		cardID,
	)
}
