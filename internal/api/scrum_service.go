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
	return s.repo != nil || s.scrumStore != nil
}

func (s *Server) loadScrumContext(r *http.Request) (ScrumBoard, int64, error) {
	if s.repo != nil {
		projectID, err := s.resolveProjectID(r)
		if err == nil {
			board, err := s.scrumBoardFromProject(r.Context(), projectID)
			return board, projectID, err
		}
	}
	if s.scrumStore != nil {
		return s.scrumStore.Board(), 0, nil
	}
	return ScrumBoard{}, 0, fmt.Errorf("scrum store unavailable")
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
	if s.repo != nil {
		projectID, err := s.resolveProjectID(r)
		if err == nil {
			col := normalizeScrumColumn(column)
			if col == "" {
				col = "backlog"
			}
			card, err := s.repo.CreateScrumCard(r.Context(), projectID, "", title, description, col, nil, nil, nil)
			if err != nil {
				return ScrumCard{}, err
			}
			return dbScrumCardToAPI(card), nil
		}
	}
	if s.scrumStore == nil {
		return ScrumCard{}, fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.CreateCard(title, description, column)
}

func (s *Server) scrumUpdateCard(r *http.Request, cardID string, patch ScrumCard, raw map[string]json.RawMessage) (ScrumCard, error) {
	if s.repo != nil {
		projectID, err := s.resolveProjectID(r)
		if err == nil {
			current, err := s.repo.GetScrumCard(r.Context(), projectID, cardID)
			if err != nil {
				return ScrumCard{}, err
			}
			merged := dbScrumCardToAPI(current)
			if strings.TrimSpace(patch.Title) != "" {
				merged.Title = strings.TrimSpace(patch.Title)
			}
			if patch.Description != "" {
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
			patchMap := apiScrumCardToPatch(merged)
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
			result := dbScrumCardToAPI(updated)
			result.FlowMetrics = s.trackScrumCardFlow(r.Context(), projectID, dbScrumCardToAPI(current), result, "update")
			return result, nil
		}
	}
	if s.scrumStore == nil {
		return ScrumCard{}, fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.UpdateCard(cardID, patch)
}

func (s *Server) scrumDeleteCard(r *http.Request, cardID string) error {
	if s.repo != nil {
		projectID, err := s.resolveProjectID(r)
		if err == nil {
			return s.repo.DeleteScrumCard(r.Context(), projectID, cardID)
		}
	}
	if s.scrumStore == nil {
		return fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.DeleteCard(cardID)
}

func (s *Server) scrumAppendChat(r *http.Request, cardID, role, content string) (ScrumCard, error) {
	card, _, projectID, err := s.scrumGetCard(r, cardID)
	if err != nil {
		return ScrumCard{}, err
	}
	card.Chat = append(card.Chat, ScrumChatMessage{
		Role:      strings.TrimSpace(role),
		Content:   strings.TrimSpace(content),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
	card.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if s.repo != nil && projectID > 0 {
		updated, err := s.repo.UpdateScrumCard(r.Context(), projectID, cardID, apiScrumCardToPatch(card))
		if err != nil {
			return ScrumCard{}, err
		}
		return dbScrumCardToAPI(updated), nil
	}
	if s.scrumStore == nil {
		return ScrumCard{}, fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.AppendChat(cardID, role, content)
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
	if s.repo != nil && projectID > 0 {
		updated, err := s.repo.UpdateScrumCard(r.Context(), projectID, cardID, apiScrumCardToPatch(card))
		if err != nil {
			return ScrumCard{}, err
		}
		return dbScrumCardToAPI(updated), nil
	}
	if s.scrumStore == nil {
		return ScrumCard{}, fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.SetCardJob(cardID, jobID, column, consoleLog)
}

func (s *Server) scrumUpdateBoard(r *http.Request, name, projectDirectory string) (ScrumBoard, error) {
	if s.repo != nil {
		projectID, err := s.resolveProjectID(r)
		if err == nil {
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
	}
	if s.scrumStore == nil {
		return ScrumBoard{}, fmt.Errorf("scrum store unavailable")
	}
	return s.scrumStore.UpdateBoard(name, projectDirectory)
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
		"runtime":                "v3",
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
			if err := json.Unmarshal(card.Recipe, &recipe); err == nil && len(recipe) > 0 {
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
		return enriched, pulled, nil
	}
	meta["review_always"] = "off"
	if executionAgent, _ := meta["execution_agent"].(string); executionAgent == agentconfig.SystemOmnidex {
		meta["omnidex_no_delegate"] = true
	} else if executionAgent == agentconfig.SystemCursor || executionAgent == agentconfig.SystemCodex {
		meta["scrum_raw_play"] = true
		delete(meta, "runtime")
	}
	out, err := json.Marshal(meta)
	if err != nil {
		return enriched, pulled, nil
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
	s.refreshScrumFlowMetricsForBoard(r.Context(), projectID, &board)
	payload := map[string]any{
		"board":        board,
		"cards_by_col": cardsByColumn(board),
		"play_queue":   scrumPlayQueueSummary(board),
		"flow_summary": summarizeScrumFlowMetrics(board.Cards),
	}
	if projectID > 0 {
		payload["project_id"] = projectID
		payload["auto_play_through"] = s.scrumAutoPlayThroughEnabled(r.Context(), projectID)
		payload["auto_review"] = s.scrumAutoReviewConfig(r.Context(), projectID)
	}
	return payload, nil
}

func (s *Server) initializeProjectIfNeeded(ctx context.Context, projectID int64) {
	if s.repo == nil || projectID <= 0 {
		return
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return
	}
	_, _ = s.initializeProjectState(ctx, project)
}
