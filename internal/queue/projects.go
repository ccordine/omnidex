package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

const workspaceActiveProjectKey = "active_project_id"

var ErrProjectNotFound = errors.New("project not found")

func scanProject(row pgx.Row) (model.Project, error) {
	var project model.Project
	var recipe, settings []byte
	err := row.Scan(
		&project.ID,
		&project.Location,
		&project.Name,
		&project.Description,
		&project.RecipeID,
		&recipe,
		&project.ProjectState,
		&settings,
		&project.LastSeenAt,
		&project.CreatedAt,
		&project.UpdatedAt,
	)
	if err != nil {
		return model.Project{}, err
	}
	project.Recipe = json.RawMessage(recipe)
	project.Settings = json.RawMessage(settings)
	return project, nil
}

const projectSelectColumns = `
	id, location, name, description, recipe_id, recipe, project_state, settings,
	last_seen_at, created_at, updated_at
`

func (r *Repository) ListProjects(ctx context.Context, limit, offset int) ([]model.Project, error) {
	if limit <= 0 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+projectSelectColumns+`
		FROM projects
		ORDER BY updated_at DESC, id DESC
		LIMIT $1 OFFSET $2
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	projects := []model.Project{}
	for rows.Next() {
		project, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		projects = append(projects, project)
	}
	return projects, rows.Err()
}

func (r *Repository) GetProject(ctx context.Context, id int64) (model.Project, error) {
	row := r.pool.QueryRow(ctx, `
		SELECT `+projectSelectColumns+`
		FROM projects
		WHERE id = $1
	`, id)
	project, err := scanProject(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Project{}, ErrProjectNotFound
		}
		return model.Project{}, err
	}
	return project, nil
}

func (r *Repository) GetProjectByLocation(ctx context.Context, location string) (model.Project, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return model.Project{}, fmt.Errorf("location is required")
	}
	row := r.pool.QueryRow(ctx, `
		SELECT `+projectSelectColumns+`
		FROM projects
		WHERE location = $1
	`, location)
	project, err := scanProject(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Project{}, ErrProjectNotFound
		}
		return model.Project{}, err
	}
	return project, nil
}

func (r *Repository) CreateProject(ctx context.Context, name, location, description, recipeID string, recipe json.RawMessage) (model.Project, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return model.Project{}, fmt.Errorf("location is required")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = projectNameFromLocation(location)
	}
	if len(recipe) == 0 {
		recipe = json.RawMessage(`{}`)
	}
	row := r.pool.QueryRow(ctx, `
		INSERT INTO projects (location, name, description, recipe_id, recipe, last_seen_at)
		VALUES ($1, $2, $3, $4, $5::jsonb, NOW())
		RETURNING `+projectSelectColumns+`
	`, location, name, strings.TrimSpace(description), strings.TrimSpace(recipeID), string(recipe))
	return scanProject(row)
}

func (r *Repository) UpdateProject(ctx context.Context, id int64, patch model.ProjectPatch) (model.Project, error) {
	current, err := r.GetProject(ctx, id)
	if err != nil {
		return model.Project{}, err
	}
	if patch.Name != nil && strings.TrimSpace(*patch.Name) != "" {
		current.Name = strings.TrimSpace(*patch.Name)
	}
	if patch.Location != nil && strings.TrimSpace(*patch.Location) != "" {
		current.Location = strings.TrimSpace(*patch.Location)
	}
	if patch.Description != nil {
		current.Description = strings.TrimSpace(*patch.Description)
	}
	if patch.RecipeID != nil {
		current.RecipeID = strings.TrimSpace(*patch.RecipeID)
	}
	if patch.Recipe != nil {
		current.Recipe = *patch.Recipe
	}
	if patch.ProjectState != nil {
		current.ProjectState = strings.TrimSpace(*patch.ProjectState)
	}
	if patch.Settings != nil {
		current.Settings = *patch.Settings
	}
	if len(current.Recipe) == 0 {
		current.Recipe = json.RawMessage(`{}`)
	}
	if len(current.Settings) == 0 {
		current.Settings = json.RawMessage(`{}`)
	}

	row := r.pool.QueryRow(ctx, `
		UPDATE projects
		SET name = $2,
		    location = $3,
		    description = $4,
		    recipe_id = $5,
		    recipe = $6::jsonb,
		    project_state = $7,
		    settings = $8::jsonb,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING `+projectSelectColumns+`
	`, id, current.Name, current.Location, current.Description, current.RecipeID, string(current.Recipe), current.ProjectState, string(current.Settings))
	return scanProject(row)
}

func (r *Repository) DeleteProject(ctx context.Context, id int64) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrProjectNotFound
	}
	active, _ := r.GetActiveProjectID(ctx)
	if active == id {
		_ = r.ClearActiveProjectID(ctx)
	}
	return nil
}

func (r *Repository) TouchProject(ctx context.Context, id int64) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE projects
		SET last_seen_at = NOW(), updated_at = NOW()
		WHERE id = $1
	`, id)
	return err
}

func (r *Repository) GetActiveProjectID(ctx context.Context) (int64, error) {
	var raw []byte
	err := r.pool.QueryRow(ctx, `
		SELECT value
		FROM workspace_settings
		WHERE key = $1
	`, workspaceActiveProjectKey).Scan(&raw)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, nil
		}
		return 0, err
	}
	var payload struct {
		ProjectID int64 `json:"project_id"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, err
	}
	return payload.ProjectID, nil
}

func (r *Repository) SetActiveProjectID(ctx context.Context, projectID int64) error {
	if projectID > 0 {
		if _, err := r.GetProject(ctx, projectID); err != nil {
			return err
		}
	}
	value, err := json.Marshal(map[string]any{"project_id": projectID})
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		INSERT INTO workspace_settings (key, value, updated_at)
		VALUES ($1, $2::jsonb, NOW())
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value,
		    updated_at = NOW()
	`, workspaceActiveProjectKey, string(value))
	if err == nil && projectID > 0 {
		_ = r.TouchProject(ctx, projectID)
	}
	return err
}

func (r *Repository) ClearActiveProjectID(ctx context.Context) error {
	return r.SetActiveProjectID(ctx, 0)
}

func (r *Repository) CountProjectJobs(ctx context.Context, projectID int64) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM jobs WHERE project_id = $1`, projectID).Scan(&count)
	return count, err
}

func (r *Repository) CountProjectCards(ctx context.Context, projectID int64) (int64, error) {
	var count int64
	err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM scrum_cards WHERE project_id = $1`, projectID).Scan(&count)
	return count, err
}

type DBScrumCard struct {
	ID          string
	ProjectID   int64
	Title       string
	Description string
	Column      string
	Checklist   json.RawMessage
	RefFiles    json.RawMessage
	Chat        json.RawMessage
	ModelConfig json.RawMessage
	AgentConfig json.RawMessage
	JiraTicket   string
	JiraPrompt   string
	RecipeID     string
	Recipe       json.RawMessage
	Tags         json.RawMessage
	PlanningChat json.RawMessage
	CoachConfig  json.RawMessage
	TestCriteria json.RawMessage
	JobID        string
	ConsoleLog  string
	PlayState   string
	QueueOrder  int
	BoardOrder  int
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

func (r *Repository) ListScrumCards(ctx context.Context, projectID int64) ([]DBScrumCard, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT id, project_id, title, description, column_name, checklist, ref_files, chat,
		       model_config, agent_config, jira_ticket, jira_prompt, recipe_id, recipe,
		       tags, planning_chat, coach_config, test_criteria,
		       job_id, console_log, play_state, queue_order, board_order, created_at, updated_at
		FROM scrum_cards
		WHERE project_id = $1
		ORDER BY updated_at DESC, id ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cards := []DBScrumCard{}
	for rows.Next() {
		var card DBScrumCard
		if err := rows.Scan(
			&card.ID,
			&card.ProjectID,
			&card.Title,
			&card.Description,
			&card.Column,
			&card.Checklist,
			&card.RefFiles,
			&card.Chat,
			&card.ModelConfig,
			&card.AgentConfig,
			&card.JiraTicket,
			&card.JiraPrompt,
			&card.RecipeID,
			&card.Recipe,
			&card.Tags,
			&card.PlanningChat,
			&card.CoachConfig,
			&card.TestCriteria,
			&card.JobID,
			&card.ConsoleLog,
			&card.PlayState,
			&card.QueueOrder,
			&card.BoardOrder,
			&card.CreatedAt,
			&card.UpdatedAt,
		); err != nil {
			return nil, err
		}
		cards = append(cards, card)
	}
	return cards, rows.Err()
}

func (r *Repository) GetScrumCard(ctx context.Context, projectID int64, cardID string) (DBScrumCard, error) {
	var card DBScrumCard
	err := r.pool.QueryRow(ctx, `
		SELECT id, project_id, title, description, column_name, checklist, ref_files, chat,
		       model_config, agent_config, jira_ticket, jira_prompt, recipe_id, recipe,
		       tags, planning_chat, coach_config, test_criteria,
		       job_id, console_log, play_state, queue_order, board_order, created_at, updated_at
		FROM scrum_cards
		WHERE project_id = $1 AND id = $2
	`, projectID, strings.TrimSpace(cardID)).Scan(
		&card.ID,
		&card.ProjectID,
		&card.Title,
		&card.Description,
		&card.Column,
		&card.Checklist,
		&card.RefFiles,
		&card.Chat,
		&card.ModelConfig,
		&card.AgentConfig,
		&card.JiraTicket,
		&card.JiraPrompt,
		&card.RecipeID,
		&card.Recipe,
		&card.Tags,
		&card.PlanningChat,
		&card.CoachConfig,
		&card.TestCriteria,
		&card.JobID,
		&card.ConsoleLog,
		&card.PlayState,
		&card.QueueOrder,
		&card.BoardOrder,
		&card.CreatedAt,
		&card.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DBScrumCard{}, fmt.Errorf("card not found")
		}
		return DBScrumCard{}, err
	}
	return card, nil
}

func (r *Repository) CreateScrumCard(ctx context.Context, projectID int64, cardID, title, description, column string, checklist, refFiles, chat json.RawMessage) (DBScrumCard, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return DBScrumCard{}, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(cardID) == "" {
		cardID = fmt.Sprintf("card_%d", time.Now().UnixNano())
	}
	column = strings.TrimSpace(column)
	if column == "" {
		column = "backlog"
	}
	if len(checklist) == 0 {
		checklist = json.RawMessage(`[]`)
	}
	if len(refFiles) == 0 {
		refFiles = json.RawMessage(`[]`)
	}
	if len(chat) == 0 {
		chat = json.RawMessage(`[]`)
	}
	var card DBScrumCard
	err := r.pool.QueryRow(ctx, `
		INSERT INTO scrum_cards (id, project_id, title, description, column_name, checklist, ref_files, chat, board_order)
		VALUES (
			$1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8::jsonb,
			COALESCE((SELECT MAX(board_order) FROM scrum_cards WHERE project_id = $2 AND column_name = $5), -1) + 1
		)
		RETURNING id, project_id, title, description, column_name, checklist, ref_files, chat,
		          model_config, agent_config, jira_ticket, jira_prompt, recipe_id, recipe,
		          tags, planning_chat, coach_config, test_criteria,
		          job_id, console_log, play_state, queue_order, board_order, created_at, updated_at
	`, cardID, projectID, title, strings.TrimSpace(description), column, string(checklist), string(refFiles), string(chat)).Scan(
		&card.ID,
		&card.ProjectID,
		&card.Title,
		&card.Description,
		&card.Column,
		&card.Checklist,
		&card.RefFiles,
		&card.Chat,
		&card.ModelConfig,
		&card.AgentConfig,
		&card.JiraTicket,
		&card.JiraPrompt,
		&card.RecipeID,
		&card.Recipe,
		&card.Tags,
		&card.PlanningChat,
		&card.CoachConfig,
		&card.TestCriteria,
		&card.JobID,
		&card.ConsoleLog,
		&card.PlayState,
		&card.QueueOrder,
		&card.BoardOrder,
		&card.CreatedAt,
		&card.UpdatedAt,
	)
	if err != nil {
		return DBScrumCard{}, err
	}
	_ = r.TouchProject(ctx, projectID)
	return card, nil
}

func (r *Repository) UpdateScrumCard(ctx context.Context, projectID int64, cardID string, patch map[string]any) (DBScrumCard, error) {
	current, err := r.GetScrumCard(ctx, projectID, cardID)
	if err != nil {
		return DBScrumCard{}, err
	}
	if title, ok := patch["title"].(string); ok && strings.TrimSpace(title) != "" {
		current.Title = strings.TrimSpace(title)
	}
	if description, ok := patch["description"].(string); ok {
		current.Description = description
	}
	if column, ok := patch["column"].(string); ok && strings.TrimSpace(column) != "" {
		current.Column = strings.TrimSpace(column)
	}
	if checklist, ok := patch["checklist"].(json.RawMessage); ok {
		current.Checklist = checklist
	}
	if refFiles, ok := patch["ref_files"].(json.RawMessage); ok {
		current.RefFiles = refFiles
	}
	if chat, ok := patch["chat"].(json.RawMessage); ok {
		current.Chat = chat
	}
	if modelConfig, ok := patch["model_config"].(json.RawMessage); ok {
		current.ModelConfig = modelConfig
	}
	if agentConfig, ok := patch["agent_config"].(json.RawMessage); ok {
		current.AgentConfig = agentConfig
	}
	if jobID, ok := patch["job_id"].(string); ok {
		current.JobID = strings.TrimSpace(jobID)
	}
	if consoleLog, ok := patch["console_log"].(string); ok {
		current.ConsoleLog = consoleLog
	}
	if playState, ok := patch["play_state"].(string); ok {
		current.PlayState = strings.TrimSpace(playState)
	}
	if queueOrder, ok := patch["queue_order"]; ok {
		switch v := queueOrder.(type) {
		case int:
			current.QueueOrder = v
		case float64:
			current.QueueOrder = int(v)
		}
	}
	if boardOrder, ok := patch["board_order"]; ok {
		switch v := boardOrder.(type) {
		case int:
			current.BoardOrder = v
		case float64:
			current.BoardOrder = int(v)
		}
	}
	if jiraTicket, ok := patch["jira_ticket"].(string); ok {
		current.JiraTicket = jiraTicket
	}
	if recipeID, ok := patch["recipe_id"].(string); ok {
		current.RecipeID = strings.TrimSpace(recipeID)
	}
	if recipe, ok := patch["recipe"].(json.RawMessage); ok {
		current.Recipe = recipe
	}
	if jiraPrompt, ok := patch["jira_prompt"].(string); ok {
		current.JiraPrompt = jiraPrompt
	}
	if tags, ok := patch["tags"].(json.RawMessage); ok {
		current.Tags = tags
	}
	if planningChat, ok := patch["planning_chat"].(json.RawMessage); ok {
		current.PlanningChat = planningChat
	}
	if coachConfig, ok := patch["coach_config"].(json.RawMessage); ok {
		current.CoachConfig = coachConfig
	}
	if testCriteria, ok := patch["test_criteria"].(json.RawMessage); ok {
		current.TestCriteria = testCriteria
	}

	var card DBScrumCard
	err = r.pool.QueryRow(ctx, `
		UPDATE scrum_cards
		SET title = $3,
		    description = $4,
		    column_name = $5,
		    checklist = $6::jsonb,
		    ref_files = $7::jsonb,
		    chat = $8::jsonb,
		    model_config = $9::jsonb,
		    agent_config = $10::jsonb,
		    jira_ticket = $11,
		    jira_prompt = $12,
		    recipe_id = $13,
		    recipe = $14::jsonb,
		    tags = $15::jsonb,
		    planning_chat = $16::jsonb,
		    coach_config = $17::jsonb,
		    test_criteria = $18::jsonb,
		    job_id = $19,
		    console_log = $20,
		    play_state = $21,
		    queue_order = $22,
		    board_order = $23,
		    updated_at = NOW()
		WHERE project_id = $1 AND id = $2
		RETURNING id, project_id, title, description, column_name, checklist, ref_files, chat,
		          model_config, agent_config, jira_ticket, jira_prompt, recipe_id, recipe,
		          tags, planning_chat, coach_config, test_criteria,
		          job_id, console_log, play_state, queue_order, board_order, created_at, updated_at
	`, projectID, cardID, current.Title, current.Description, current.Column, string(current.Checklist), string(current.RefFiles), string(current.Chat), string(current.ModelConfig), string(current.AgentConfig), current.JiraTicket, current.JiraPrompt, current.RecipeID, string(current.Recipe), string(defaultJSON(current.Tags, `[]`)), string(defaultJSON(current.PlanningChat, `[]`)), string(defaultJSON(current.CoachConfig, `{}`)), string(defaultJSON(current.TestCriteria, `[]`)), current.JobID, current.ConsoleLog, current.PlayState, current.QueueOrder, current.BoardOrder).Scan(
		&card.ID,
		&card.ProjectID,
		&card.Title,
		&card.Description,
		&card.Column,
		&card.Checklist,
		&card.RefFiles,
		&card.Chat,
		&card.ModelConfig,
		&card.AgentConfig,
		&card.JiraTicket,
		&card.JiraPrompt,
		&card.RecipeID,
		&card.Recipe,
		&card.Tags,
		&card.PlanningChat,
		&card.CoachConfig,
		&card.TestCriteria,
		&card.JobID,
		&card.ConsoleLog,
		&card.PlayState,
		&card.QueueOrder,
		&card.BoardOrder,
		&card.CreatedAt,
		&card.UpdatedAt,
	)
	if err != nil {
		return DBScrumCard{}, err
	}
	_ = r.TouchProject(ctx, projectID)
	return card, nil
}

func (r *Repository) DeleteScrumCard(ctx context.Context, projectID int64, cardID string) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM scrum_cards WHERE project_id = $1 AND id = $2`, projectID, cardID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("card not found")
	}
	_ = r.TouchProject(ctx, projectID)
	return nil
}

func defaultJSON(raw json.RawMessage, fallback string) json.RawMessage {
	if len(raw) > 0 {
		return raw
	}
	return json.RawMessage(fallback)
}

func ProjectNameFromLocation(location string) string {
	return projectNameFromLocation(location)
}

func NormalizeProjectLocation(location string) (string, error) {
	location = strings.TrimSpace(location)
	if location == "" {
		return "", fmt.Errorf("location is required")
	}
	abs, err := filepath.Abs(location)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}
