package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

const maxScrumCardTicketBytes = 64 * 1024

var (
	ErrScrumCardNotFound        = errors.New("Scrum card was not found")
	ErrScrumCardVersionConflict = errors.New("Scrum card version conflict")
)

// ScrumCardTicketMutation is the complete code-owned card-ticket post-state.
// The repository never accepts an action, prompt, or model-selected operation.
type ScrumCardTicketMutation struct {
	ProjectID         int64
	CardID            string
	ExpectedUpdatedAt time.Time
	Ticket            string
	Elaboration       string
}

func (mutation *ScrumCardTicketMutation) validate() error {
	if mutation == nil || mutation.ProjectID <= 0 || strings.TrimSpace(mutation.CardID) == "" {
		return fmt.Errorf("Scrum card ticket mutation requires a project and card")
	}
	mutation.CardID = strings.TrimSpace(mutation.CardID)
	if mutation.ExpectedUpdatedAt.IsZero() {
		return fmt.Errorf("Scrum card ticket mutation requires an expected card revision")
	}
	if strings.TrimSpace(mutation.Ticket) == "" {
		return fmt.Errorf("Scrum card ticket mutation requires a non-blank ticket")
	}
	for name, value := range map[string]string{"ticket": mutation.Ticket, "elaboration": mutation.Elaboration} {
		if !utf8.ValidString(value) || strings.ContainsRune(value, '\x00') {
			return fmt.Errorf("Scrum card ticket %s must be valid UTF-8 without NUL", name)
		}
	}
	if len(mutation.Ticket) > maxScrumCardTicketBytes {
		return fmt.Errorf("Scrum card ticket exceeds the %d-byte bound", maxScrumCardTicketBytes)
	}
	return nil
}

// UpdateScrumCardTicket atomically binds the complete ticket post-state to the
// exact card revision observed by the caller.
func (r *Repository) UpdateScrumCardTicket(
	ctx context.Context,
	mutation ScrumCardTicketMutation,
) (DBScrumCard, error) {
	if r == nil || r.pool == nil || ctx == nil {
		return DBScrumCard{}, fmt.Errorf("PostgreSQL and context are required for Scrum card ticket mutation")
	}
	if err := mutation.validate(); err != nil {
		return DBScrumCard{}, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return DBScrumCard{}, fmt.Errorf("begin Scrum card ticket mutation: %w", err)
	}
	defer rollbackTx(ctx, tx, "Scrum card ticket mutation")

	current, err := lockScrumCardTx(ctx, tx, mutation.ProjectID, mutation.CardID)
	if err != nil {
		return DBScrumCard{}, err
	}
	if !current.UpdatedAt.Equal(mutation.ExpectedUpdatedAt) {
		return DBScrumCard{}, fmt.Errorf(
			"%w: Scrum card %q changed; reload server state and retry",
			ErrScrumCardVersionConflict,
			mutation.CardID,
		)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE scrum_cards
		SET card_ticket=$3,
		    card_prompt=$4,
		    updated_at=GREATEST(clock_timestamp(), updated_at + interval '1 microsecond')
		WHERE project_id=$1 AND id=$2
	`, mutation.ProjectID, mutation.CardID, mutation.Ticket, mutation.Elaboration)
	if err != nil {
		return DBScrumCard{}, fmt.Errorf("persist Scrum card ticket: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return DBScrumCard{}, fmt.Errorf("%w: Scrum card %q disappeared during mutation", ErrScrumCardNotFound, mutation.CardID)
	}
	if err := touchScrumTicketProject(ctx, tx, mutation.ProjectID); err != nil {
		return DBScrumCard{}, err
	}
	updated, err := scanDBScrumCard(tx.QueryRow(ctx, scrumCardSelectSQL, mutation.ProjectID, mutation.CardID))
	if err != nil {
		return DBScrumCard{}, fmt.Errorf("load mutated Scrum card ticket: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return DBScrumCard{}, fmt.Errorf("commit Scrum card ticket mutation: %w", err)
	}
	return updated, nil
}

func touchScrumTicketProject(ctx context.Context, tx pgx.Tx, projectID int64) error {
	tag, err := tx.Exec(ctx, `
		UPDATE projects SET last_seen_at=clock_timestamp(), updated_at=clock_timestamp()
		WHERE id=$1
	`, projectID)
	if err != nil {
		return fmt.Errorf("touch Scrum card ticket project: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("Scrum card ticket project %d was not found", projectID)
	}
	return nil
}
