package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalidProjectPlanningDraftAction = errors.New("invalid project planning draft action")
	ErrProjectPlanningDraftConflict      = errors.New("project planning draft conflict")
)

type ProjectPlanningDraftMutation struct {
	Action   string
	DraftID  string
	DraftIDs []string
	Status   string
}

type ProjectPlanningDraftMutationResult struct {
	Drafts []model.ProjectPlanningDraft
	Cards  []DBScrumCard
}

func (r *Repository) MutateProjectPlanningDrafts(ctx context.Context, projectID int64, mutation ProjectPlanningDraftMutation) (ProjectPlanningDraftMutationResult, error) {
	mutation.Action = strings.ToLower(strings.TrimSpace(mutation.Action))
	mutation.DraftID = strings.TrimSpace(mutation.DraftID)
	mutation.Status = strings.ToLower(strings.TrimSpace(mutation.Status))
	requestedIDs, err := normalizePlanningDraftIDs(mutation.DraftIDs)
	if err != nil {
		return ProjectPlanningDraftMutationResult{}, err
	}
	tx, err := r.beginLockedProjectTx(ctx, projectID, "project planning draft mutation")
	if err != nil {
		return ProjectPlanningDraftMutationResult{}, err
	}
	defer rollbackTx(ctx, tx, "project planning draft mutation")
	drafts, err := listProjectPlanningDrafts(ctx, tx, projectID)
	if err != nil {
		return ProjectPlanningDraftMutationResult{}, err
	}

	result := ProjectPlanningDraftMutationResult{}
	switch mutation.Action {
	case "add":
		if mutation.DraftID == "" {
			return result, fmt.Errorf("%w: draft_id is required", ErrInvalidProjectPlanningDraftAction)
		}
		result.Cards, err = promotePlanningDrafts(ctx, tx, projectID, drafts, map[string]struct{}{mutation.DraftID: {}}, true)
	case "add_all":
		result.Cards, err = promotePlanningDrafts(ctx, tx, projectID, drafts, requestedIDs, len(requestedIDs) > 0)
	case "dismiss":
		if mutation.DraftID == "" {
			return result, fmt.Errorf("%w: draft_id is required", ErrInvalidProjectPlanningDraftAction)
		}
		err = dismissPlanningDrafts(ctx, tx, projectID, []string{mutation.DraftID}, true)
	case "dismiss_all":
		err = dismissPlanningDrafts(ctx, tx, projectID, nil, false)
	case "clear":
		if mutation.Status != "added" && mutation.Status != "dismissed" {
			return result, fmt.Errorf("%w: clear status must be added or dismissed", ErrInvalidProjectPlanningDraftAction)
		}
		err = clearPlanningDrafts(ctx, tx, projectID, mutation.Status)
	default:
		return result, fmt.Errorf("%w: unsupported action %q", ErrInvalidProjectPlanningDraftAction, mutation.Action)
	}
	if err != nil {
		return ProjectPlanningDraftMutationResult{}, err
	}
	if err := touchProjectTx(ctx, tx, projectID); err != nil {
		return ProjectPlanningDraftMutationResult{}, err
	}
	result.Drafts, err = listProjectPlanningDrafts(ctx, tx, projectID)
	if err != nil {
		return ProjectPlanningDraftMutationResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ProjectPlanningDraftMutationResult{}, err
	}
	return result, nil
}

func normalizePlanningDraftIDs(ids []string) (map[string]struct{}, error) {
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			return nil, fmt.Errorf("%w: draft_ids cannot contain an empty ID", ErrInvalidProjectPlanningDraftAction)
		}
		if _, exists := out[id]; exists {
			return nil, fmt.Errorf("%w: duplicate draft ID %q", ErrInvalidProjectPlanningDraftAction, id)
		}
		out[id] = struct{}{}
	}
	return out, nil
}

func promotePlanningDrafts(ctx context.Context, tx pgx.Tx, projectID int64, drafts []model.ProjectPlanningDraft, requested map[string]struct{}, explicit bool) ([]DBScrumCard, error) {
	selected := make([]model.ProjectPlanningDraft, 0)
	matched := map[string]struct{}{}
	for _, draft := range drafts {
		if explicit {
			if _, ok := requested[draft.ID]; !ok {
				continue
			}
			matched[draft.ID] = struct{}{}
		}
		if draft.Status != "pending" {
			if explicit {
				return nil, fmt.Errorf("%w: draft %q is %s, expected pending", ErrProjectPlanningDraftConflict, draft.ID, draft.Status)
			}
			continue
		}
		selected = append(selected, draft)
	}
	if explicit && len(matched) != len(requested) {
		return nil, fmt.Errorf("%w: requested %d drafts but found %d", ErrProjectPlanningDraftConflict, len(requested), len(matched))
	}
	if len(selected) == 0 {
		return nil, fmt.Errorf("%w: no pending drafts selected", ErrProjectPlanningDraftConflict)
	}

	cards := make([]DBScrumCard, 0, len(selected))
	for index, draft := range selected {
		cardID := fmt.Sprintf("card_%d_%d", time.Now().UnixNano(), index)
		checklist := make([]map[string]any, 0, len(draft.Checklist))
		for _, item := range draft.Checklist {
			checklist = append(checklist, map[string]any{"text": item, "done": false})
		}
		checklistJSON, err := json.Marshal(checklist)
		if err != nil {
			return nil, fmt.Errorf("encode planning draft %q checklist: %w", draft.ID, err)
		}
		card, err := insertPlanningDraftCard(ctx, tx, projectID, cardID, draft, checklistJSON)
		if err != nil {
			return nil, fmt.Errorf("promote planning draft %q: %w", draft.ID, err)
		}
		tag, err := tx.Exec(ctx, `
			UPDATE project_planning_drafts
			SET status = 'added', card_id = $3, added_at = NOW(), updated_at = NOW()
			WHERE project_id = $1 AND id = $2 AND status = 'pending'
		`, projectID, draft.ID, card.ID)
		if err != nil {
			return nil, err
		}
		if tag.RowsAffected() != 1 {
			return nil, fmt.Errorf("%w: draft %q changed while being promoted", ErrProjectPlanningDraftConflict, draft.ID)
		}
		cards = append(cards, card)
	}
	return cards, nil
}

func insertPlanningDraftCard(ctx context.Context, tx pgx.Tx, projectID int64, cardID string, draft model.ProjectPlanningDraft, checklist []byte) (DBScrumCard, error) {
	return scanDBScrumCard(tx.QueryRow(ctx, `
		INSERT INTO scrum_cards (id, project_id, title, description, column_name, checklist, board_order)
		VALUES (
			$1, $2, $3, $4, $5, $6::jsonb,
			COALESCE((SELECT MAX(board_order) FROM scrum_cards WHERE project_id = $2 AND column_name = $5), -1) + 1
		)
		RETURNING id, project_id, title, description, column_name, checklist, ref_files, chat,
		          model_config, agent_config, card_ticket, card_prompt, recipe_id, recipe,
		          tags, planning_chat, coach_config, test_criteria, flow_metrics,
		          job_id, tags_job_id, ticket_job_id, console_log, play_state, queue_order, board_order, created_at, updated_at
	`, cardID, projectID, draft.Title, draft.Description, draft.Column, string(checklist)))
}

func dismissPlanningDrafts(ctx context.Context, tx pgx.Tx, projectID int64, ids []string, explicit bool) error {
	query := `
		UPDATE project_planning_drafts
		SET status = 'dismissed', updated_at = NOW()
		WHERE project_id = $1 AND status = 'pending'
	`
	arguments := []any{projectID}
	if explicit {
		query += ` AND id = ANY($2)`
		arguments = append(arguments, ids)
	}
	tag, err := tx.Exec(ctx, query, arguments...)
	if err != nil {
		return err
	}
	expected := int64(len(ids))
	if !explicit {
		expected = 1
	}
	if tag.RowsAffected() < expected {
		return fmt.Errorf("%w: dismissed %d drafts; expected at least %d", ErrProjectPlanningDraftConflict, tag.RowsAffected(), expected)
	}
	return nil
}

func clearPlanningDrafts(ctx context.Context, tx pgx.Tx, projectID int64, status string) error {
	tag, err := tx.Exec(ctx, `DELETE FROM project_planning_drafts WHERE project_id = $1 AND status = $2`, projectID, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: no %s drafts to clear", ErrProjectPlanningDraftConflict, status)
	}
	return nil
}
