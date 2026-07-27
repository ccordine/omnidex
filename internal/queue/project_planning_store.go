package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func upsertProjectPlanningConfig(ctx context.Context, tx pgx.Tx, projectID int64, config *model.ProjectPlanningConfig) error {
	return tx.QueryRow(ctx, `
		INSERT INTO project_planning_configs (project_id, model, reasoning_mode)
		VALUES ($1, $2, $3)
		ON CONFLICT (project_id) DO UPDATE
		SET model = EXCLUDED.model, reasoning_mode = EXCLUDED.reasoning_mode, updated_at = NOW()
		RETURNING updated_at
	`, projectID, config.Model, config.ReasoningMode).Scan(&config.UpdatedAt)
}

func insertProjectPlanningMessage(ctx context.Context, tx pgx.Tx, projectID int64, role, content string) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO project_planning_messages (project_id, role, content)
		VALUES ($1, $2, $3)
	`, projectID, role, content)
	return err
}

func touchProjectTx(ctx context.Context, tx pgx.Tx, projectID int64) error {
	tag, err := tx.Exec(ctx, `UPDATE projects SET last_seen_at = NOW(), updated_at = NOW() WHERE id = $1`, projectID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrProjectNotFound
	}
	return nil
}

func listProjectPlanningDrafts(ctx context.Context, db planningQuerier, projectID int64) ([]model.ProjectPlanningDraft, error) {
	rows, err := db.Query(ctx, `
		SELECT project_id, id, title, description, column_name, checklist, status,
		       source, batch_id, card_id, created_at, added_at, updated_at
		FROM project_planning_drafts
		WHERE project_id = $1
		ORDER BY created_at ASC, id ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	drafts := []model.ProjectPlanningDraft{}
	for rows.Next() {
		var draft model.ProjectPlanningDraft
		var checklist json.RawMessage
		if err := rows.Scan(&draft.ProjectID, &draft.ID, &draft.Title, &draft.Description, &draft.Column, &checklist,
			&draft.Status, &draft.Source, &draft.BatchID, &draft.CardID, &draft.CreatedAt, &draft.AddedAt, &draft.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(checklist, &draft.Checklist); err != nil {
			return nil, fmt.Errorf("decode project planning draft %q checklist: %w", draft.ID, err)
		}
		drafts = append(drafts, draft)
	}
	return drafts, rows.Err()
}

func insertProjectPlanningDrafts(ctx context.Context, tx pgx.Tx, projectID int64, drafts []model.ProjectPlanningDraft) error {
	for _, draft := range drafts {
		checklist, err := json.Marshal(draft.Checklist)
		if err != nil {
			return fmt.Errorf("encode project planning draft %q checklist: %w", draft.ID, err)
		}
		var duplicate bool
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (
				SELECT 1 FROM project_planning_drafts
				WHERE project_id = $1 AND lower(btrim(title)) = lower(btrim($2)) AND status <> 'dismissed'
			)
		`, projectID, draft.Title).Scan(&duplicate); err != nil {
			return err
		}
		if duplicate {
			return fmt.Errorf("%w: active draft %q already exists", ErrProjectPlanningConflict, draft.Title)
		}
		_, err = tx.Exec(ctx, `
			INSERT INTO project_planning_drafts (
				project_id, id, title, description, column_name, checklist, status, source, batch_id
			) VALUES ($1, $2, $3, $4, $5, $6::jsonb, 'pending', $7, $8)
		`, projectID, draft.ID, draft.Title, draft.Description, draft.Column, string(checklist), draft.Source, draft.BatchID)
		if err != nil {
			return err
		}
	}
	return nil
}

func validateProjectPlanningDraft(draft *model.ProjectPlanningDraft, projectID int64) error {
	draft.ProjectID = projectID
	draft.ID = strings.TrimSpace(draft.ID)
	draft.Title = strings.TrimSpace(draft.Title)
	draft.Description = strings.TrimSpace(draft.Description)
	draft.Column = strings.TrimSpace(draft.Column)
	draft.Source = strings.TrimSpace(draft.Source)
	draft.BatchID = strings.TrimSpace(draft.BatchID)
	if draft.ID == "" || draft.Title == "" || draft.Column == "" || draft.Source == "" {
		return fmt.Errorf("id, title, column, and source are required")
	}
	validColumn := false
	for _, column := range []string{"backlog", "ready", "assigned", "in_progress", "review", "blocked", "error", "done"} {
		if draft.Column == column {
			validColumn = true
			break
		}
	}
	if !validColumn {
		return fmt.Errorf("unsupported Scrum column %q", draft.Column)
	}
	if draft.Status != "" && draft.Status != "pending" {
		return fmt.Errorf("new draft status must be pending")
	}
	draft.Status = "pending"
	for i := range draft.Checklist {
		draft.Checklist[i] = strings.TrimSpace(draft.Checklist[i])
		if draft.Checklist[i] == "" {
			return fmt.Errorf("checklist item %d is empty", i)
		}
	}
	return nil
}
