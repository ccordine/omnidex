package queue

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const (
	projectPlanningMessagePageDefault = 50
	projectPlanningMessagePageMax     = 100
	projectPlanningDraftLimit         = 60
)

var ErrProjectPlanningConflict = errors.New("project planning state conflict")

type ProjectPlanningCommit struct {
	Config           model.ProjectPlanningConfig
	UserMessage      string
	AssistantMessage string
	Drafts           []model.ProjectPlanningDraft
}

type ProjectPlanningCommitResult struct {
	Messages model.ProjectPlanningMessagePage
	Drafts   []model.ProjectPlanningDraft
}

func validateProjectPlanningConfig(config model.ProjectPlanningConfig) (model.ProjectPlanningConfig, error) {
	config.Model = strings.TrimSpace(config.Model)
	config.ReasoningMode = strings.ToLower(strings.TrimSpace(config.ReasoningMode))
	switch config.ReasoningMode {
	case "instant", "thinking":
		return config, nil
	default:
		return model.ProjectPlanningConfig{}, fmt.Errorf("unsupported project planning reasoning mode %q", config.ReasoningMode)
	}
}

func (r *Repository) GetProjectPlanningConfig(ctx context.Context, projectID int64) (model.ProjectPlanningConfig, error) {
	if r == nil || r.pool == nil || ctx == nil || projectID <= 0 {
		return model.ProjectPlanningConfig{}, fmt.Errorf("PostgreSQL, context, and project are required for planning config")
	}
	var config model.ProjectPlanningConfig
	err := r.pool.QueryRow(ctx, `
		SELECT COALESCE(c.model, ''), COALESCE(c.reasoning_mode, 'instant'), COALESCE(c.updated_at, p.updated_at)
		FROM projects p
		LEFT JOIN project_planning_configs c ON c.project_id = p.id
		WHERE p.id = $1
	`, projectID).Scan(&config.Model, &config.ReasoningMode, &config.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.ProjectPlanningConfig{}, ErrProjectNotFound
	}
	if err != nil {
		return model.ProjectPlanningConfig{}, err
	}
	return validateProjectPlanningConfig(config)
}

func (r *Repository) UpdateProjectPlanningConfig(ctx context.Context, projectID int64, config model.ProjectPlanningConfig) (model.ProjectPlanningConfig, error) {
	config, err := validateProjectPlanningConfig(config)
	if err != nil {
		return model.ProjectPlanningConfig{}, err
	}
	tx, err := r.beginLockedProjectTx(ctx, projectID, "project planning config update")
	if err != nil {
		return model.ProjectPlanningConfig{}, err
	}
	defer rollbackTx(ctx, tx, "project planning config update")
	if err := upsertProjectPlanningConfig(ctx, tx, projectID, &config); err != nil {
		return model.ProjectPlanningConfig{}, err
	}
	if err := touchProjectTx(ctx, tx, projectID); err != nil {
		return model.ProjectPlanningConfig{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return model.ProjectPlanningConfig{}, err
	}
	return config, nil
}

func (r *Repository) ListProjectPlanningMessages(ctx context.Context, projectID int64, limit int, beforeID int64) (model.ProjectPlanningMessagePage, error) {
	if r == nil || r.pool == nil || ctx == nil || projectID <= 0 || beforeID < 0 {
		return model.ProjectPlanningMessagePage{}, fmt.Errorf("valid PostgreSQL planning message query is required")
	}
	if limit <= 0 {
		limit = projectPlanningMessagePageDefault
	}
	if limit > projectPlanningMessagePageMax {
		return model.ProjectPlanningMessagePage{}, fmt.Errorf("project planning message limit cannot exceed %d", projectPlanningMessagePageMax)
	}
	return listProjectPlanningMessages(ctx, r.pool, projectID, limit, beforeID)
}

type planningQuerier interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func listProjectPlanningMessages(ctx context.Context, db planningQuerier, projectID int64, limit int, beforeID int64) (model.ProjectPlanningMessagePage, error) {
	rows, err := db.Query(ctx, `
		SELECT id, project_id, role, content, created_at
		FROM project_planning_messages
		WHERE project_id = $1 AND ($2::bigint = 0 OR id < $2)
		ORDER BY id DESC
		LIMIT $3
	`, projectID, beforeID, limit+1)
	if err != nil {
		return model.ProjectPlanningMessagePage{}, err
	}
	defer rows.Close()
	messages := make([]model.ProjectPlanningMessage, 0, limit+1)
	for rows.Next() {
		var message model.ProjectPlanningMessage
		if err := rows.Scan(&message.ID, &message.ProjectID, &message.Role, &message.Content, &message.CreatedAt); err != nil {
			return model.ProjectPlanningMessagePage{}, err
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return model.ProjectPlanningMessagePage{}, err
	}
	hasMore := len(messages) > limit
	if hasMore {
		messages = messages[:limit]
	}
	nextBeforeID := int64(0)
	if hasMore && len(messages) > 0 {
		nextBeforeID = messages[len(messages)-1].ID
	}
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	return model.ProjectPlanningMessagePage{Messages: messages, HasMore: hasMore, NextBeforeID: nextBeforeID}, nil
}

func (r *Repository) ListProjectPlanningDrafts(ctx context.Context, projectID int64) ([]model.ProjectPlanningDraft, error) {
	if r == nil || r.pool == nil || ctx == nil || projectID <= 0 {
		return nil, fmt.Errorf("PostgreSQL, context, and project are required for planning drafts")
	}
	return listProjectPlanningDrafts(ctx, r.pool, projectID)
}

func (r *Repository) CommitProjectPlanningResponse(ctx context.Context, projectID int64, commit ProjectPlanningCommit) (ProjectPlanningCommitResult, error) {
	config, err := validateProjectPlanningConfig(commit.Config)
	if err != nil {
		return ProjectPlanningCommitResult{}, err
	}
	commit.UserMessage = strings.TrimSpace(commit.UserMessage)
	commit.AssistantMessage = strings.TrimSpace(commit.AssistantMessage)
	if commit.AssistantMessage == "" && len(commit.Drafts) == 0 {
		return ProjectPlanningCommitResult{}, fmt.Errorf("project planning response contains no durable result")
	}
	for i := range commit.Drafts {
		if err := validateProjectPlanningDraft(&commit.Drafts[i], projectID); err != nil {
			return ProjectPlanningCommitResult{}, fmt.Errorf("project planning draft %d: %w", i, err)
		}
	}

	tx, err := r.beginLockedProjectTx(ctx, projectID, "project planning response commit")
	if err != nil {
		return ProjectPlanningCommitResult{}, err
	}
	defer rollbackTx(ctx, tx, "project planning response commit")
	if err := upsertProjectPlanningConfig(ctx, tx, projectID, &config); err != nil {
		return ProjectPlanningCommitResult{}, err
	}
	if commit.UserMessage != "" {
		if err := insertProjectPlanningMessage(ctx, tx, projectID, "user", commit.UserMessage); err != nil {
			return ProjectPlanningCommitResult{}, err
		}
	}
	if commit.AssistantMessage != "" {
		if err := insertProjectPlanningMessage(ctx, tx, projectID, "assistant", commit.AssistantMessage); err != nil {
			return ProjectPlanningCommitResult{}, err
		}
	}
	if err := insertProjectPlanningDrafts(ctx, tx, projectID, commit.Drafts); err != nil {
		return ProjectPlanningCommitResult{}, err
	}
	messages, err := listProjectPlanningMessages(ctx, tx, projectID, projectPlanningMessagePageDefault, 0)
	if err != nil {
		return ProjectPlanningCommitResult{}, err
	}
	drafts, err := listProjectPlanningDrafts(ctx, tx, projectID)
	if err != nil {
		return ProjectPlanningCommitResult{}, err
	}
	if len(drafts) > projectPlanningDraftLimit {
		return ProjectPlanningCommitResult{}, fmt.Errorf("%w: draft queue has %d records; clear completed drafts before adding more (limit %d)", ErrProjectPlanningConflict, len(drafts), projectPlanningDraftLimit)
	}
	if err := touchProjectTx(ctx, tx, projectID); err != nil {
		return ProjectPlanningCommitResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ProjectPlanningCommitResult{}, err
	}
	return ProjectPlanningCommitResult{Messages: messages, Drafts: drafts}, nil
}
