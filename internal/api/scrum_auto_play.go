package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
)

const scrumAutoPlayThroughKey = "scrum_auto_play_through"

var scrumAutoPlayWorkColumns = []string{"backlog", "ready", "assigned", "in_progress", "blocked"}

func loadScrumAutoPlayThrough(settings json.RawMessage) bool {
	if len(settings) == 0 {
		return false
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(settings, &payload); err != nil {
		return false
	}
	raw, ok := payload[scrumAutoPlayThroughKey]
	if !ok || len(raw) == 0 {
		return false
	}
	var enabled bool
	if err := json.Unmarshal(raw, &enabled); err != nil {
		return false
	}
	return enabled
}

func (s *Server) scrumAutoPlayThroughEnabled(ctx context.Context, projectID int64) bool {
	if s.repo == nil || projectID <= 0 {
		return false
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return false
	}
	return loadScrumAutoPlayThrough(project.Settings)
}

func (s *Server) saveScrumAutoPlayThrough(ctx context.Context, project model.Project, enabled bool) error {
	var settings map[string]any
	if len(project.Settings) > 0 {
		_ = json.Unmarshal(project.Settings, &settings)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	settings[scrumAutoPlayThroughKey] = enabled
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingsJSON := json.RawMessage(raw)
	patch := model.ProjectPatch{Settings: &settingsJSON}
	_, err = s.repo.UpdateProject(ctx, project.ID, patch)
	return err
}

// nextAutoPlayThroughScrumCard picks the next card top-to-bottom (column order, then board_order).
func (s *Server) nextAutoPlayThroughScrumCard(board ScrumBoard) *ScrumCard {
	if next := s.nextQueuedScrumCard(board); next != nil {
		return next
	}
	for _, column := range scrumAutoPlayWorkColumns {
		if next := s.nextAutoPlayCardInColumn(board, column); next != nil {
			return next
		}
	}
	return nil
}

func (s *Server) nextAutoPlayCardInColumn(board ScrumBoard, column string) *ScrumCard {
	column = normalizeScrumColumn(column)
	candidates := make([]ScrumCard, 0)
	for _, card := range board.Cards {
		if normalizeScrumColumn(card.Column) != column {
			continue
		}
		switch card.PlayState {
		case scrumPlayRunning, scrumPlayQueued:
			continue
		}
		candidates = append(candidates, card)
	}
	if len(candidates) == 0 {
		return nil
	}
	sortCardsForColumn(column, candidates)
	return &candidates[0]
}

func scrumAutoPlayThroughComplete(board ScrumBoard) bool {
	return scrumAutoPlayThroughCompleteWithReview(board, false)
}

func scrumAutoPlayThroughCompleteWithReview(board ScrumBoard, autoReviewEnabled bool) bool {
	for _, card := range board.Cards {
		col := normalizeScrumColumn(card.Column)
		switch col {
		case "review":
			if autoReviewEnabled && card.PlayState == scrumPlayReviewing {
				return false
			}
			continue
		case "done":
			continue
		default:
			return false
		}
	}
	return len(board.Cards) > 0
}

func (s *Server) prepareScrumCardForAutoPlay(r *http.Request, projectID int64, card ScrumCard) (ScrumCard, error) {
	col := normalizeScrumColumn(card.Column)
	switch col {
	case "backlog", "blocked":
		card.Column = "assigned"
		if card.PlayState == scrumPlayPaused {
			card.PlayState = ""
		}
		card = appendScrumChannelEvent(card, "system", "Auto-play moved card to Assigned")
		return s.persistScrumCard(r, projectID, card)
	case "ready", "assigned", "in_progress":
		if card.PlayState == scrumPlayPaused {
			card.PlayState = ""
			return s.persistScrumCard(r, projectID, card)
		}
		return card, nil
	default:
		return card, fmt.Errorf("card %s is not playable for auto-play", card.ID)
	}
}

func (s *Server) kickoffAutoPlayThrough(r *http.Request, projectID int64, board ScrumBoard) (ScrumBoard, error) {
	if s.findRunningScrumCard(board) != nil {
		return board, nil
	}
	if !s.scrumAutoPlayThroughEnabled(r.Context(), projectID) {
		return board, nil
	}
	reviewCfg := s.scrumAutoReviewConfig(r.Context(), projectID)
	if scrumAutoPlayThroughCompleteWithReview(board, reviewCfg.Enabled) {
		return board, nil
	}
	next := s.nextAutoPlayThroughScrumCard(board)
	if next == nil {
		return board, nil
	}
	prepared, err := s.prepareScrumCardForAutoPlay(r, projectID, *next)
	if err != nil {
		return board, nil
	}
	if _, err := s.startScrumCardPlay(r, board, projectID, prepared.ID, agentconfig.Config{}); err != nil {
		return board, nil
	}
	if projectID > 0 {
		return s.scrumBoardFromProject(r.Context(), projectID)
	}
	if s.scrumStore != nil {
		return s.scrumStore.Board(), nil
	}
	return board, nil
}
