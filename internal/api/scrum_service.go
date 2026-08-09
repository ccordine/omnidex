package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
)

func (s *Server) scrumAvailable() bool {
	return s.repo != nil
}

func (s *Server) loadScrumContext(r *http.Request) (ScrumBoard, int64, error) {
	if s.repo == nil {
		return ScrumBoard{}, 0, fmt.Errorf("postgres repository is required for Scrum")
	}
	projectID, err := s.resolveProjectID(r)
	if err != nil {
		return ScrumBoard{}, 0, err
	}
	board, err := s.scrumBoardFromProject(r.Context(), projectID)
	return board, projectID, err
}

func (s *Server) scrumGetCard(r *http.Request, cardID string) (ScrumCard, ScrumBoard, int64, error) {
	board, projectID, err := s.loadScrumContext(r)
	if err != nil {
		return ScrumCard{}, ScrumBoard{}, 0, err
	}
	for _, card := range board.Cards {
		if card.ID == cardID {
			return card, board, projectID, nil
		}
	}
	return ScrumCard{}, board, projectID, fmt.Errorf("card not found")
}

func (s *Server) scrumCreateCard(r *http.Request, title, description, column string) (ScrumCard, error) {
	if s.repo == nil {
		return ScrumCard{}, fmt.Errorf("postgres repository is required for Scrum")
	}
	projectID, err := s.resolveProjectID(r)
	if err != nil {
		return ScrumCard{}, err
	}
	col := normalizeScrumColumn(column)
	if col == "" {
		col = "backlog"
	}
	card, err := s.repo.CreateScrumCard(r.Context(), projectID, "", title, description, col, nil, nil, nil)
	if err != nil {
		return ScrumCard{}, err
	}
	return dbScrumCardToAPI(card)
}

func (s *Server) scrumUpdateCard(r *http.Request, cardID string, patch ScrumCard, raw map[string]json.RawMessage) (ScrumCard, error) {
	if s.repo == nil {
		return ScrumCard{}, fmt.Errorf("postgres repository is required for Scrum")
	}
	projectID, err := s.resolveProjectID(r)
	if err != nil {
		return ScrumCard{}, err
	}
	current, err := s.repo.GetScrumCard(r.Context(), projectID, cardID)
	if err != nil {
		return ScrumCard{}, err
	}
	merged, err := dbScrumCardToAPI(current)
	if err != nil {
		return ScrumCard{}, fmt.Errorf("decode current Scrum card: %w", err)
	}
	if strings.TrimSpace(patch.Title) != "" {
		merged.Title = strings.TrimSpace(patch.Title)
	}
	if _, ok := raw["description"]; ok {
		merged.Description = patch.Description
	}
	if col := normalizeScrumColumn(patch.Column); col != "" {
		merged.Column = col
	}
	if patch.Checklist != nil {
		merged.Checklist = patch.Checklist
	}
	if patch.RefFiles != nil {
		merged.RefFiles = patch.RefFiles
	}
	if patch.Chat != nil {
		merged.Chat = patch.Chat
	}
	if len(patch.ModelConfig) > 0 {
		merged.ModelConfig = patch.ModelConfig
	}
	if len(patch.AgentConfig) > 0 {
		merged.AgentConfig = patch.AgentConfig
	}
	if _, ok := raw["card_ticket"]; ok {
		merged.CardTicket = patch.CardTicket
	}
	if _, ok := raw["recipe_id"]; ok {
		merged.RecipeID = strings.TrimSpace(patch.RecipeID)
	}
	if _, ok := raw["recipe"]; ok {
		if len(patch.Recipe) > 0 {
			merged.Recipe = patch.Recipe
		} else {
			merged.Recipe = json.RawMessage(`{}`)
		}
	}
	if _, ok := raw["card_prompt"]; ok {
		merged.CardPrompt = patch.CardPrompt
	}
	if patch.PlanningChat != nil {
		merged.PlanningChat = patch.PlanningChat
	}
	if patch.Tags != nil {
		merged.Tags = patch.Tags
	}
	if patch.TestCriteria != nil {
		merged.TestCriteria = patch.TestCriteria
	}
	if len(patch.CoachConfig) > 0 {
		merged.CoachConfig = patch.CoachConfig
	}
	if patch.ConsoleLog != "" {
		merged.ConsoleLog = patch.ConsoleLog
	}
	if strings.TrimSpace(patch.JobID) != "" {
		merged.JobID = strings.TrimSpace(patch.JobID)
	}
	merged.PlayState = strings.TrimSpace(patch.PlayState)
	merged.QueueOrder = patch.QueueOrder
	patchMap, err := apiScrumCardToPatch(merged)
	if err != nil {
		return ScrumCard{}, err
	}
	if _, ok := raw["card_ticket"]; ok {
		patchMap["card_ticket"] = merged.CardTicket
	}
	if _, ok := raw["recipe_id"]; ok {
		patchMap["recipe_id"] = merged.RecipeID
	}
	if _, ok := raw["recipe"]; ok {
		patchMap["recipe"] = merged.Recipe
	}
	updated, err := s.repo.UpdateScrumCard(r.Context(), projectID, cardID, patchMap)
	if err != nil {
		return ScrumCard{}, err
	}
	result, err := dbScrumCardToAPI(updated)
	if err != nil {
		return ScrumCard{}, fmt.Errorf("decode updated Scrum card: %w", err)
	}
	previous, err := dbScrumCardToAPI(current)
	if err != nil {
		return ScrumCard{}, fmt.Errorf("decode previous Scrum card: %w", err)
	}
	result.FlowMetrics = s.trackScrumCardFlow(r.Context(), projectID, previous, result, "update")
	return result, nil
}

func (s *Server) scrumDeleteCard(r *http.Request, cardID string) error {
	if s.repo == nil {
		return fmt.Errorf("postgres repository is required for Scrum")
	}
	projectID, err := s.resolveProjectID(r)
	if err != nil {
		return err
	}
	return s.repo.DeleteScrumCard(r.Context(), projectID, cardID)
}

func (s *Server) scrumSetCardJob(r *http.Request, cardID, jobID, column, consoleLog string) (ScrumCard, error) {
	card, _, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		return ScrumCard{}, err
	}
	if strings.TrimSpace(jobID) != "" {
		card.JobID = strings.TrimSpace(jobID)
	}
	if col := normalizeScrumColumn(column); col != "" {
		card.Column = col
	}
	if consoleLog != "" {
		card.ConsoleLog = consoleLog
	}
	card.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if s.repo == nil || projectID <= 0 {
		return ScrumCard{}, fmt.Errorf("postgres repository and project are required for Scrum")
	}
	patch, err := apiScrumCardToPatch(card)
	if err != nil {
		return ScrumCard{}, err
	}
	updated, err := s.repo.UpdateScrumCard(r.Context(), projectID, cardID, patch)
	if err != nil {
		return ScrumCard{}, err
	}
	return dbScrumCardToAPI(updated)
}

func (s *Server) scrumUpdateBoard(r *http.Request, name, projectDirectory string) (ScrumBoard, error) {
	if s.repo == nil {
		return ScrumBoard{}, fmt.Errorf("postgres repository is required for Scrum")
	}
	projectID, err := s.resolveProjectID(r)
	if err != nil {
		return ScrumBoard{}, err
	}
	patch := model.ProjectPatch{}
	if strings.TrimSpace(name) != "" {
		v := strings.TrimSpace(name)
		patch.Name = &v
	}
	if strings.TrimSpace(projectDirectory) != "" {
		v := strings.TrimSpace(projectDirectory)
		patch.Location = &v
	}
	if patch.Name != nil || patch.Location != nil {
		if _, err := s.repo.UpdateProject(r.Context(), projectID, patch); err != nil {
			return ScrumBoard{}, err
		}
	}
	return s.scrumBoardFromProject(r.Context(), projectID)
}

func (s *Server) scrumPlayMetadata(ctx context.Context, board ScrumBoard, card ScrumCard, projectID int64, instance agentconfig.Config) ([]byte, []string, error) {
	checklistLines := make([]string, 0, len(card.Checklist))
	for _, item := range card.Checklist {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		state := "[ ]"
		if item.Done {
			state = "[x]"
		}
		checklistLines = append(checklistLines, state+" "+item.Text)
	}
	testLines := make([]string, 0, len(card.TestCriteria))
	for _, item := range card.TestCriteria {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		state := "[ ]"
		if item.Done {
			state = "[x]"
		}
		testLines = append(testLines, state+" "+item.Text)
	}
	payload := map[string]any{
		"source":                 "omni-scrum",
		"scrum_card_id":          card.ID,
		"scrum_card_title":       card.Title,
		"scrum_card_description": card.Description,
		"scrum_card_ticket":      card.CardTicket,
		"scrum_checklist":        strings.Join(checklistLines, "\n"),
		"scrum_test_criteria":    strings.Join(testLines, "\n"),
		"project_directory":      board.ProjectDirectory,
		"client_cwd":             board.ProjectDirectory,
	}
	if projectID > 0 {
		payload["project_id"] = projectID
	}
	if len(card.RefFiles) > 0 {
		payload["ref_files"] = card.RefFiles
	}
	if len(card.Tags) > 0 {
		payload["scrum_card_tags"] = card.Tags
	}
	if len(instance) > 0 {
		payload["instance_agent_config"] = instance.ToMap()
	}
	if strings.TrimSpace(card.RecipeID) != "" || len(card.Recipe) > 2 {
		payload["recipe_id"] = strings.TrimSpace(card.RecipeID)
		if len(card.Recipe) > 2 {
			var recipe map[string]any
			if err := json.Unmarshal(card.Recipe, &recipe); err != nil {
				return nil, nil, fmt.Errorf("parse Scrum card recipe: %w", err)
			}
			if recipe == nil {
				return nil, nil, fmt.Errorf("Scrum card recipe must be a JSON object")
			}
			if len(recipe) > 0 {
				payload["recipe"] = recipe
			}
		}
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, err
	}
	enriched, pulled, err := s.enrichJobMetadata(ctx, raw, card)
	if err != nil {
		return nil, nil, err
	}
	var meta map[string]any
	if err := json.Unmarshal(enriched, &meta); err != nil {
		return nil, pulled, fmt.Errorf("parse enriched Scrum metadata: %w", err)
	}
	resolvedAgent, err := agentconfig.FromJobMetadata(enriched)
	if err != nil {
		return nil, pulled, err
	}
	if resolvedAgent.System() == agentconfig.SystemOmnidex {
		meta["omnidex_no_delegate"] = true
	} else if resolvedAgent.IsExternal() {
		meta["scrum_raw_play"] = true
	} else {
		return nil, pulled, fmt.Errorf("unsupported resolved agent system %q", resolvedAgent.System())
	}
	out, err := json.Marshal(meta)
	if err != nil {
		return nil, pulled, fmt.Errorf("encode Scrum job metadata: %w", err)
	}
	return out, pulled, nil
}

func (s *Server) scrumProjectDirectory(r *http.Request) (string, error) {
	board, _, err := s.loadScrumContext(r)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(board.ProjectDirectory), nil
}

func (s *Server) scrumBoardResponse(r *http.Request) (map[string]any, error) {
	board, projectID, err := s.loadScrumContext(r)
	if err != nil {
		return nil, err
	}
	board, err = s.refreshScrumPlayQueue(r, projectID, board)
	if err != nil {
		return nil, err
	}
	board, err = s.refreshScrumCardLlmJobs(r.Context(), projectID, board)
	if err != nil {
		return nil, err
	}
	if err := s.refreshScrumFlowMetricsForBoard(r.Context(), projectID, &board); err != nil {
		return nil, err
	}
	fullBoard := board
	visibleColumn := scrumViewportColumn(r, board.Columns)
	columnCounts := scrumColumnCounts(cardsByColumn(fullBoard))
	if visibleColumn != "" {
		board = scrumBoardColumnViewport(board, visibleColumn)
	}
	payload := map[string]any{
		"board":          board,
		"cards_by_col":   cardsByColumn(board),
		"play_queue":     scrumPlayQueueSummary(fullBoard),
		"flow_summary":   summarizeScrumFlowMetrics(fullBoard.Cards),
		"all_columns":    append([]string(nil), fullBoard.Columns...),
		"visible_column": visibleColumn,
		"column_counts":  columnCounts,
	}
	if projectID > 0 {
		automation, err := s.scrumAutomationSettings(r.Context(), projectID)
		if err != nil {
			return nil, err
		}
		payload["project_id"] = projectID
		payload["auto_work"] = automation.AutoWork
		payload["auto_review"] = automation.AutoReview
		payload["create_ticket"] = automation.CreateTicket
	}
	scrumBoardFragmentsForPayload(payload, fullBoard)
	return payload, nil
}
