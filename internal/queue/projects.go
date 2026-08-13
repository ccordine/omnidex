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
	name = SanitizeUTF8Text(name)
	description = SanitizeUTF8Text(strings.TrimSpace(description))
	recipeID = SanitizeUTF8Text(strings.TrimSpace(recipeID))
	recipe = SanitizeUTF8Bytes(recipe)
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
	if err := validateProjectSettings(current.Settings); err != nil {
		return model.Project{}, err
	}
	current.Name = SanitizeUTF8Text(current.Name)
	current.Location = SanitizeUTF8Text(current.Location)
	current.Description = SanitizeUTF8Text(current.Description)
	current.RecipeID = SanitizeUTF8Text(current.RecipeID)
	current.Recipe = SanitizeUTF8Bytes(current.Recipe)
	current.ProjectState = SanitizeUTF8Text(current.ProjectState)
	current.Settings = SanitizeUTF8Bytes(current.Settings)

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

func (r *Repository) HasRunningScrumPlay(ctx context.Context) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM scrum_cards
			WHERE play_state = 'running'
			LIMIT 1
		)
	`).Scan(&exists)
	return exists, err
}

func (r *Repository) ListRunningScrumPlayProjectIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT DISTINCT project_id
		FROM scrum_cards
		WHERE play_state = 'running'
		ORDER BY project_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type DBScrumCard struct {
	ID                       string          `json:"id"`
	ProjectID                int64           `json:"project_id"`
	Title                    string          `json:"title"`
	Description              string          `json:"description"`
	Column                   string          `json:"column"`
	Checklist                json.RawMessage `json:"checklist"`
	RefFiles                 json.RawMessage `json:"ref_files"`
	Chat                     json.RawMessage `json:"chat"`
	ModelConfig              json.RawMessage `json:"model_config"`
	AgentConfig              json.RawMessage `json:"agent_config"`
	CardTicket               string          `json:"card_ticket"`
	CardPrompt               string          `json:"card_prompt"`
	RecipeID                 string          `json:"recipe_id"`
	Recipe                   json.RawMessage `json:"recipe"`
	Tags                     json.RawMessage `json:"tags"`
	PlanningChat             json.RawMessage `json:"planning_chat"`
	CoachConfig              json.RawMessage `json:"coach_config"`
	TestCriteria             json.RawMessage `json:"test_criteria"`
	FlowMetrics              json.RawMessage `json:"flow_metrics"`
	JobID                    string          `json:"job_id"`
	TagsJobID                string          `json:"tags_job_id"`
	TicketJobID              string          `json:"ticket_job_id"`
	ConsoleLog               string          `json:"console_log"`
	PlayState                string          `json:"play_state"`
	QueueOrder               int             `json:"queue_order"`
	BoardOrder               int             `json:"board_order"`
	SyncJobID                string          `json:"-"`
	AgentStreamChatCursor    int64           `json:"-"`
	AgentStreamConsoleCursor int64           `json:"-"`
	StepContextCursor        int64           `json:"-"`
	CreatedAt                time.Time       `json:"created_at"`
	UpdatedAt                time.Time       `json:"updated_at"`
}

func (r *Repository) GetScrumCard(ctx context.Context, projectID int64, cardID string) (DBScrumCard, error) {
	var card DBScrumCard
	err := r.pool.QueryRow(ctx, `
		SELECT id, project_id, title, description, column_name, checklist, ref_files, chat,
		       model_config, agent_config, card_ticket, card_prompt, recipe_id, recipe,
		       tags, planning_chat, coach_config, test_criteria, flow_metrics,
		       job_id, tags_job_id, ticket_job_id, console_log, play_state, queue_order, board_order,
		       sync_job_id, agent_stream_chat_cursor, agent_stream_console_cursor, step_context_cursor,
		       created_at, updated_at
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
		&card.CardTicket,
		&card.CardPrompt,
		&card.RecipeID,
		&card.Recipe,
		&card.Tags,
		&card.PlanningChat,
		&card.CoachConfig,
		&card.TestCriteria,
		&card.FlowMetrics,
		&card.JobID,
		&card.TagsJobID,
		&card.TicketJobID,
		&card.ConsoleLog,
		&card.PlayState,
		&card.QueueOrder,
		&card.BoardOrder,
		&card.SyncJobID,
		&card.AgentStreamChatCursor,
		&card.AgentStreamConsoleCursor,
		&card.StepContextCursor,
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

func sanitizeScrumCardFields(card *DBScrumCard) {
	if card == nil {
		return
	}
	card.Title = SanitizeUTF8Text(card.Title)
	card.Description = SanitizeUTF8Text(card.Description)
	card.Column = SanitizeUTF8Text(card.Column)
	card.Checklist = SanitizeUTF8Bytes(card.Checklist)
	card.RefFiles = SanitizeUTF8Bytes(card.RefFiles)
	card.Chat = SanitizeUTF8Bytes(card.Chat)
	card.ModelConfig = SanitizeUTF8Bytes(card.ModelConfig)
	card.AgentConfig = SanitizeUTF8Bytes(card.AgentConfig)
	card.CardTicket = SanitizeUTF8Text(card.CardTicket)
	card.CardPrompt = SanitizeUTF8Text(card.CardPrompt)
	card.RecipeID = SanitizeUTF8Text(card.RecipeID)
	card.Recipe = SanitizeUTF8Bytes(card.Recipe)
	card.Tags = SanitizeUTF8Bytes(card.Tags)
	card.PlanningChat = SanitizeUTF8Bytes(card.PlanningChat)
	card.CoachConfig = SanitizeUTF8Bytes(card.CoachConfig)
	card.TestCriteria = SanitizeUTF8Bytes(card.TestCriteria)
	card.FlowMetrics = SanitizeUTF8Bytes(card.FlowMetrics)
	card.JobID = SanitizeUTF8Text(card.JobID)
	card.TagsJobID = SanitizeUTF8Text(card.TagsJobID)
	card.TicketJobID = SanitizeUTF8Text(card.TicketJobID)
	card.ConsoleLog = SanitizeUTF8Text(card.ConsoleLog)
	card.PlayState = SanitizeUTF8Text(card.PlayState)
	card.SyncJobID = SanitizeUTF8Text(card.SyncJobID)
}

func (r *Repository) CreateScrumCard(ctx context.Context, projectID int64, cardID, title, description, column string, checklist, refFiles, chat json.RawMessage) (DBScrumCard, error) {
	title = SanitizeUTF8Text(strings.TrimSpace(title))
	if title == "" {
		return DBScrumCard{}, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(cardID) == "" {
		cardID = fmt.Sprintf("card_%d", time.Now().UnixNano())
	}
	column = SanitizeUTF8Text(strings.TrimSpace(column))
	if column == "" {
		column = "backlog"
	}
	description = SanitizeUTF8Text(description)
	checklist = SanitizeUTF8Bytes(defaultJSON(checklist, `[]`))
	refFiles = SanitizeUTF8Bytes(defaultJSON(refFiles, `[]`))
	chat = SanitizeUTF8Bytes(defaultJSON(chat, `[]`))
	var card DBScrumCard
	err := r.pool.QueryRow(ctx, `
		WITH inserted_card AS (
			INSERT INTO scrum_cards (id, project_id, title, description, column_name, checklist, ref_files, chat, board_order)
			VALUES (
				$1, $2, $3, $4, $5, $6::jsonb, $7::jsonb, $8::jsonb,
				COALESCE((SELECT MAX(board_order) FROM scrum_cards WHERE project_id = $2 AND column_name = $5), -1) + 1
			)
			RETURNING id, project_id, title, description, column_name, checklist, ref_files, chat,
			          model_config, agent_config, card_ticket, card_prompt, recipe_id, recipe,
			          tags, planning_chat, coach_config, test_criteria, flow_metrics,
			          job_id, tags_job_id, ticket_job_id, console_log, play_state, queue_order, board_order,
			          sync_job_id, agent_stream_chat_cursor, agent_stream_console_cursor, step_context_cursor,
			          created_at, updated_at
		),
		touched_project AS (
			UPDATE projects
			SET last_seen_at = NOW(), updated_at = NOW()
			WHERE id = $2 AND EXISTS (SELECT 1 FROM inserted_card)
			RETURNING id
		)
		SELECT inserted_card.*
		FROM inserted_card
		INNER JOIN touched_project ON touched_project.id = inserted_card.project_id
	`, cardID, projectID, title, description, column, string(checklist), string(refFiles), string(chat)).Scan(
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
		&card.CardTicket,
		&card.CardPrompt,
		&card.RecipeID,
		&card.Recipe,
		&card.Tags,
		&card.PlanningChat,
		&card.CoachConfig,
		&card.TestCriteria,
		&card.FlowMetrics,
		&card.JobID,
		&card.TagsJobID,
		&card.TicketJobID,
		&card.ConsoleLog,
		&card.PlayState,
		&card.QueueOrder,
		&card.BoardOrder,
		&card.SyncJobID,
		&card.AgentStreamChatCursor,
		&card.AgentStreamConsoleCursor,
		&card.StepContextCursor,
		&card.CreatedAt,
		&card.UpdatedAt,
	)
	if err != nil {
		return DBScrumCard{}, err
	}
	return card, nil
}

func (r *Repository) UpdateScrumCard(ctx context.Context, projectID int64, cardID string, patch map[string]any) (DBScrumCard, error) {
	current, err := r.GetScrumCard(ctx, projectID, cardID)
	if err != nil {
		return DBScrumCard{}, err
	}
	if title, ok := patch["title"].(string); ok && strings.TrimSpace(title) != "" {
		current.Title = SanitizeUTF8Text(strings.TrimSpace(title))
	}
	if description, ok := patch["description"].(string); ok {
		current.Description = SanitizeUTF8Text(description)
	}
	if column, ok := patch["column"].(string); ok && strings.TrimSpace(column) != "" {
		current.Column = strings.TrimSpace(column)
	}
	if checklist, ok := patch["checklist"].(json.RawMessage); ok {
		current.Checklist = SanitizeUTF8Bytes(checklist)
	}
	if refFiles, ok := patch["ref_files"].(json.RawMessage); ok {
		current.RefFiles = SanitizeUTF8Bytes(refFiles)
	}
	if chat, ok := patch["chat"].(json.RawMessage); ok {
		current.Chat = SanitizeUTF8Bytes(chat)
	}
	if modelConfig, ok := patch["model_config"].(json.RawMessage); ok {
		canonical, err := canonicalModelConfig(SanitizeUTF8Bytes(modelConfig))
		if err != nil {
			return DBScrumCard{}, fmt.Errorf("validate Scrum card model configuration: %w", err)
		}
		current.ModelConfig = canonical
	}
	if agentConfig, ok := patch["agent_config"].(json.RawMessage); ok {
		canonical, err := canonicalAgentConfig(SanitizeUTF8Bytes(agentConfig))
		if err != nil {
			return DBScrumCard{}, fmt.Errorf("validate Scrum card agent configuration: %w", err)
		}
		current.AgentConfig = canonical
	}
	if jobID, ok := patch["job_id"].(string); ok {
		current.JobID = strings.TrimSpace(jobID)
	}
	if tagsJobID, ok := patch["tags_job_id"].(string); ok {
		current.TagsJobID = strings.TrimSpace(tagsJobID)
	}
	if ticketJobID, ok := patch["ticket_job_id"].(string); ok {
		current.TicketJobID = strings.TrimSpace(ticketJobID)
	}
	if consoleLog, ok := patch["console_log"].(string); ok {
		current.ConsoleLog = SanitizeUTF8Text(consoleLog)
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
	if syncJobID, ok := patch["sync_job_id"].(string); ok {
		current.SyncJobID = strings.TrimSpace(syncJobID)
	}
	for key, target := range map[string]*int64{
		"agent_stream_chat_cursor":    &current.AgentStreamChatCursor,
		"agent_stream_console_cursor": &current.AgentStreamConsoleCursor,
		"step_context_cursor":         &current.StepContextCursor,
	} {
		if raw, exists := patch[key]; exists {
			cursor, err := exactScrumCursor(raw, key)
			if err != nil {
				return DBScrumCard{}, err
			}
			*target = cursor
		}
	}
	if cardTicket, ok := patch["card_ticket"].(string); ok {
		current.CardTicket = SanitizeUTF8Text(cardTicket)
	}
	if recipeID, ok := patch["recipe_id"].(string); ok {
		current.RecipeID = SanitizeUTF8Text(strings.TrimSpace(recipeID))
	}
	if recipe, ok := patch["recipe"].(json.RawMessage); ok {
		current.Recipe = SanitizeUTF8Bytes(recipe)
	}
	if cardPrompt, ok := patch["card_prompt"].(string); ok {
		current.CardPrompt = SanitizeUTF8Text(cardPrompt)
	}
	if tags, ok := patch["tags"].(json.RawMessage); ok {
		current.Tags = SanitizeUTF8Bytes(tags)
	}
	if planningChat, ok := patch["planning_chat"].(json.RawMessage); ok {
		current.PlanningChat = SanitizeUTF8Bytes(planningChat)
	}
	if coachConfig, ok := patch["coach_config"].(json.RawMessage); ok {
		current.CoachConfig = SanitizeUTF8Bytes(coachConfig)
	}
	if testCriteria, ok := patch["test_criteria"].(json.RawMessage); ok {
		current.TestCriteria = SanitizeUTF8Bytes(testCriteria)
	}

	sanitizeScrumCardFields(&current)
	if err := validateDBScrumCursorAuthority(current); err != nil {
		return DBScrumCard{}, err
	}

	var card DBScrumCard
	err = r.pool.QueryRow(ctx, `
		WITH updated_card AS (
			UPDATE scrum_cards
			SET title = $3,
		    description = $4,
		    column_name = $5,
		    checklist = $6::jsonb,
		    ref_files = $7::jsonb,
		    chat = $8::jsonb,
		    model_config = $9::jsonb,
		    agent_config = $10::jsonb,
		    card_ticket = $11,
		    card_prompt = $12,
		    recipe_id = $13,
		    recipe = $14::jsonb,
		    tags = $15::jsonb,
		    planning_chat = $16::jsonb,
		    coach_config = $17::jsonb,
		    test_criteria = $18::jsonb,
		    job_id = $19,
		    tags_job_id = $20,
		    ticket_job_id = $21,
		    console_log = $22,
		    play_state = $23,
		    queue_order = $24,
		    board_order = $25,
		    sync_job_id = $26,
		    agent_stream_chat_cursor = $27,
		    agent_stream_console_cursor = $28,
		    step_context_cursor = $29,
			    updated_at = NOW()
			WHERE project_id = $1 AND id = $2 AND updated_at = $30
			RETURNING id, project_id, title, description, column_name, checklist, ref_files, chat,
			          model_config, agent_config, card_ticket, card_prompt, recipe_id, recipe,
			          tags, planning_chat, coach_config, test_criteria, flow_metrics,
			          job_id, tags_job_id, ticket_job_id, console_log, play_state, queue_order, board_order,
			          sync_job_id, agent_stream_chat_cursor, agent_stream_console_cursor, step_context_cursor,
			          created_at, updated_at
		),
		touched_project AS (
			UPDATE projects
			SET last_seen_at = NOW(), updated_at = NOW()
			WHERE id = $1 AND EXISTS (SELECT 1 FROM updated_card)
			RETURNING id
		)
		SELECT updated_card.*
		FROM updated_card
		INNER JOIN touched_project ON touched_project.id = updated_card.project_id
	`, projectID, cardID, current.Title, current.Description, current.Column, string(current.Checklist), string(current.RefFiles), string(current.Chat), string(current.ModelConfig), string(current.AgentConfig), current.CardTicket, current.CardPrompt, current.RecipeID, string(current.Recipe), string(defaultJSON(current.Tags, `[]`)), string(defaultJSON(current.PlanningChat, `[]`)), string(defaultJSON(current.CoachConfig, `{}`)), string(defaultJSON(current.TestCriteria, `[]`)), current.JobID, current.TagsJobID, current.TicketJobID, current.ConsoleLog, current.PlayState, current.QueueOrder, current.BoardOrder, current.SyncJobID, current.AgentStreamChatCursor, current.AgentStreamConsoleCursor, current.StepContextCursor, current.UpdatedAt).Scan(
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
		&card.CardTicket,
		&card.CardPrompt,
		&card.RecipeID,
		&card.Recipe,
		&card.Tags,
		&card.PlanningChat,
		&card.CoachConfig,
		&card.TestCriteria,
		&card.FlowMetrics,
		&card.JobID,
		&card.TagsJobID,
		&card.TicketJobID,
		&card.ConsoleLog,
		&card.PlayState,
		&card.QueueOrder,
		&card.BoardOrder,
		&card.SyncJobID,
		&card.AgentStreamChatCursor,
		&card.AgentStreamConsoleCursor,
		&card.StepContextCursor,
		&card.CreatedAt,
		&card.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return DBScrumCard{}, fmt.Errorf("Scrum card %q changed concurrently; reload server state and retry", cardID)
		}
		return DBScrumCard{}, err
	}
	return card, nil
}

func (r *Repository) DeleteScrumCard(ctx context.Context, projectID int64, cardID string) error {
	var touchedProjectID int64
	err := r.pool.QueryRow(ctx, `
		WITH deleted_card AS (
			DELETE FROM scrum_cards
			WHERE project_id = $1 AND id = $2
			RETURNING project_id
		),
		touched_project AS (
			UPDATE projects
			SET last_seen_at = NOW(), updated_at = NOW()
			FROM deleted_card
			WHERE projects.id = deleted_card.project_id
			RETURNING projects.id
		)
		SELECT id FROM touched_project
	`, projectID, cardID).Scan(&touchedProjectID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("card not found")
	}
	return err
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
