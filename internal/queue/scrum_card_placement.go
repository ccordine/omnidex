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

var ErrScrumCardActiveMove = errors.New("active Scrum card move is forbidden")

type ScrumCardMove struct {
	ProjectID         int64
	CardID            string
	Column            ScrumCardColumn
	BeforeCardID      string
	ExpectedUpdatedAt time.Time
}

// MoveScrumCard derives the complete column reorder inside one locked database
// transaction. Neither the browser nor API materializes or rewrites a column.
func (r *Repository) MoveScrumCard(ctx context.Context, move ScrumCardMove) (DBScrumCard, DBScrumCard, error) {
	if move.CardID == "" || !utf8.ValidString(move.CardID) || strings.ContainsRune(move.CardID, '\x00') ||
		move.CardID != strings.TrimSpace(move.CardID) || len(move.CardID) > MaxScrumCardIDBytes {
		return DBScrumCard{}, DBScrumCard{}, fmt.Errorf("Scrum card move requires one canonical card ID")
	}
	if _, err := ParseScrumCardColumn(string(move.Column)); err != nil {
		return DBScrumCard{}, DBScrumCard{}, err
	}
	if !utf8.ValidString(move.BeforeCardID) || strings.ContainsRune(move.BeforeCardID, '\x00') ||
		move.BeforeCardID != strings.TrimSpace(move.BeforeCardID) || len(move.BeforeCardID) > MaxScrumCardIDBytes {
		return DBScrumCard{}, DBScrumCard{}, fmt.Errorf("Scrum card move requires one canonical before-card ID")
	}
	if move.ExpectedUpdatedAt.IsZero() {
		return DBScrumCard{}, DBScrumCard{}, fmt.Errorf("Scrum card move requires an expected card revision")
	}
	if r == nil || r.pool == nil || ctx == nil || move.ProjectID <= 0 {
		return DBScrumCard{}, DBScrumCard{}, fmt.Errorf("PostgreSQL, context, project, card, and column are required for Scrum card move")
	}
	tx, err := r.beginLockedProjectTx(ctx, move.ProjectID, "move Scrum card")
	if err != nil {
		return DBScrumCard{}, DBScrumCard{}, err
	}
	defer rollbackTx(ctx, tx, "move Scrum card")
	previous, err := lockScrumCardTx(ctx, tx, move.ProjectID, move.CardID)
	if err != nil {
		return DBScrumCard{}, DBScrumCard{}, err
	}
	if !previous.UpdatedAt.Equal(move.ExpectedUpdatedAt) {
		return DBScrumCard{}, DBScrumCard{}, fmt.Errorf(
			"%w: Scrum card %q changed; reload server state and retry",
			ErrScrumCardVersionConflict,
			move.CardID,
		)
	}
	if previous.PlayState == "running" || previous.PlayState == "queued" {
		return DBScrumCard{}, DBScrumCard{}, fmt.Errorf(
			"%w: card %q is %s; pause it before moving",
			ErrScrumCardActiveMove,
			move.CardID,
			previous.PlayState,
		)
	}
	if move.BeforeCardID == move.CardID {
		return DBScrumCard{}, DBScrumCard{}, fmt.Errorf("before card must differ from moved card")
	}
	if move.BeforeCardID != "" {
		var beforeID string
		err := tx.QueryRow(ctx, `
			SELECT id FROM scrum_cards
			WHERE project_id=$1 AND column_name=$2 AND id=$3
			FOR UPDATE
		`, move.ProjectID, string(move.Column), move.BeforeCardID).Scan(&beforeID)
		if errors.Is(err, pgx.ErrNoRows) {
			return DBScrumCard{}, DBScrumCard{}, fmt.Errorf("before card %q was not found in target column %q", move.BeforeCardID, move.Column)
		}
		if err != nil {
			return DBScrumCard{}, DBScrumCard{}, err
		}
	}
	if previous.Column != string(move.Column) {
		if err := normalizeScrumColumnOrderTx(ctx, tx, move.ProjectID, previous.Column, move.CardID); err != nil {
			return DBScrumCard{}, DBScrumCard{}, err
		}
	}
	if err := normalizeScrumColumnOrderTx(ctx, tx, move.ProjectID, string(move.Column), move.CardID); err != nil {
		return DBScrumCard{}, DBScrumCard{}, err
	}
	insertAt, err := scrumMoveInsertPositionTx(ctx, tx, move)
	if err != nil {
		return DBScrumCard{}, DBScrumCard{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE scrum_cards SET board_order=board_order+1,
		 updated_at=GREATEST(clock_timestamp(), updated_at + interval '1 microsecond')
		WHERE project_id=$1 AND column_name=$2 AND id<>$3 AND board_order >= $4
	`, move.ProjectID, string(move.Column), move.CardID, insertAt); err != nil {
		return DBScrumCard{}, DBScrumCard{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE scrum_cards SET column_name=$3, board_order=$4,
		 updated_at=GREATEST(clock_timestamp(), updated_at + interval '1 microsecond')
		WHERE project_id=$1 AND id=$2
	`, move.ProjectID, move.CardID, string(move.Column), insertAt); err != nil {
		return DBScrumCard{}, DBScrumCard{}, err
	}
	if previous.Column != string(move.Column) {
		if err := applyScrumFlowMetricDeltasTx(ctx, tx, move.ProjectID, move.CardID, ScrumFlowMetricDelta{
			Kind:       ScrumFlowMetricColumnMove,
			FromColumn: previous.Column, ToColumn: string(move.Column),
		}); err != nil {
			return DBScrumCard{}, DBScrumCard{}, err
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE projects SET last_seen_at=clock_timestamp(), updated_at=clock_timestamp()
		WHERE id=$1
	`, move.ProjectID); err != nil {
		return DBScrumCard{}, DBScrumCard{}, fmt.Errorf("touch moved Scrum card project: %w", err)
	}
	updated, err := scanDBScrumCard(tx.QueryRow(ctx, scrumCardSelectSQL, move.ProjectID, move.CardID))
	if err != nil {
		return DBScrumCard{}, DBScrumCard{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return DBScrumCard{}, DBScrumCard{}, err
	}
	return previous, updated, nil
}

func normalizeScrumColumnOrderTx(ctx context.Context, tx pgx.Tx, projectID int64, column, excludeID string) error {
	_, err := tx.Exec(ctx, `
		WITH ordered AS (
			SELECT id, ROW_NUMBER() OVER (ORDER BY
				CASE WHEN $2='assigned' AND play_state='queued' THEN 1 ELSE 0 END,
				CASE WHEN $2='assigned' AND play_state='queued' THEN queue_order ELSE 0 END,
				CASE WHEN $2='in_progress' AND play_state='running' THEN 0 ELSE 1 END,
				board_order ASC, updated_at DESC, id ASC
			)-1 AS next_order
			FROM scrum_cards WHERE project_id=$1 AND column_name=$2 AND id<>$3
		)
		UPDATE scrum_cards card SET board_order=ordered.next_order,
		 updated_at=GREATEST(clock_timestamp(), card.updated_at + interval '1 microsecond')
		FROM ordered WHERE card.project_id=$1 AND card.id=ordered.id
		 AND card.board_order IS DISTINCT FROM ordered.next_order
	`, projectID, column, excludeID)
	return err
}

func scrumMoveInsertPositionTx(ctx context.Context, tx pgx.Tx, move ScrumCardMove) (int, error) {
	var position int
	if move.BeforeCardID == "" {
		err := tx.QueryRow(ctx, `
			SELECT COUNT(*) FROM scrum_cards
			WHERE project_id=$1 AND column_name=$2 AND id<>$3
		`, move.ProjectID, string(move.Column), move.CardID).Scan(&position)
		return position, err
	}
	err := tx.QueryRow(ctx, `
		SELECT board_order FROM scrum_cards
		WHERE project_id=$1 AND column_name=$2 AND id=$3
	`, move.ProjectID, string(move.Column), move.BeforeCardID).Scan(&position)
	return position, err
}
