package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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
	stored, err := s.repo.GetScrumCard(r.Context(), projectID, cardID)
	if err != nil {
		return ScrumCard{}, board, projectID, err
	}
	card, err := dbScrumCardToAPI(stored)
	return card, board, projectID, err
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
	cardOffset, err := exactChannelQueryInteger(r, "card_offset", 0, 0, 1<<30)
	if err != nil {
		return nil, err
	}
	board, projectID, err := s.loadScrumContext(r)
	if err != nil {
		return nil, err
	}
	board, err = s.refreshScrumPlayQueue(r, projectID, board)
	if err != nil {
		return nil, err
	}
	if err := s.refreshScrumFlowMetricsForBoard(r.Context(), projectID, &board); err != nil {
		return nil, err
	}
	fullBoard := board
	visibleColumn := scrumViewportColumn(r, board.Columns)
	columnCounts, err := s.scrumColumnCountsFromRepository(r.Context(), projectID)
	if err != nil {
		return nil, err
	}
	pageCards, cardHasMore, err := s.scrumCardColumnPage(r.Context(), projectID, visibleColumn, cardOffset)
	if err != nil {
		return nil, err
	}
	board.Columns = []string{visibleColumn}
	board.Cards = pageCards
	payload := map[string]any{
		"board":          board,
		"cards_by_col":   cardsByColumn(board),
		"play_queue":     scrumPlayQueueSummary(fullBoard),
		"flow_summary":   summarizeScrumFlowMetrics(fullBoard.Cards),
		"all_columns":    append([]string(nil), fullBoard.Columns...),
		"visible_column": visibleColumn,
		"column_counts":  columnCounts,
		"card_offset":    cardOffset,
		"card_has_more":  cardHasMore,
	}
	if projectID > 0 {
		automation, err := s.scrumAutomationSettings(r.Context(), projectID)
		if err != nil {
			return nil, err
		}
		payload["project_id"] = projectID
		payload["auto_work"] = automation.AutoWork
	}
	scrumBoardFragmentsForPayload(payload, fullBoard)
	return payload, nil
}
