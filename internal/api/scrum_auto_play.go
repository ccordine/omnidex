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

const scrumAutoPlayThroughKey = "scrum_auto_play_through"
const scrumAutoWorkConfigKey = "scrum_auto_work"

var defaultScrumAutoWorkColumns = []string{"assigned"}

type ScrumAutoWorkConfig struct {
	Enabled       bool     `json:"enabled"`
	SourceColumns []string `json:"source_columns"`
}

func defaultScrumAutoWorkConfig() ScrumAutoWorkConfig {
	return ScrumAutoWorkConfig{
		Enabled:       false,
		SourceColumns: append([]string{}, defaultScrumAutoWorkColumns...),
	}
}

func normalizeScrumAutoWorkColumns(columns []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, raw := range columns {
		column := normalizeScrumColumn(strings.TrimSpace(raw))
		switch column {
		case "backlog", "ready", "assigned", "in_progress", "blocked":
		default:
			continue
		}
		if _, ok := seen[column]; ok {
			continue
		}
		seen[column] = struct{}{}
		out = append(out, column)
	}
	if len(out) == 0 {
		return append([]string{}, defaultScrumAutoWorkColumns...)
	}
	return out
}

func loadScrumAutoWorkConfig(settings json.RawMessage) ScrumAutoWorkConfig {
	cfg := defaultScrumAutoWorkConfig()
	if len(settings) == 0 {
		return cfg
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(settings, &payload); err != nil {
		return cfg
	}
	if raw, ok := payload[scrumAutoPlayThroughKey]; ok && len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg.Enabled)
	}
	if raw, ok := payload[scrumAutoWorkConfigKey]; ok && len(raw) > 0 {
		var stored ScrumAutoWorkConfig
		if err := json.Unmarshal(raw, &stored); err == nil {
			cfg.Enabled = stored.Enabled
			cfg.SourceColumns = normalizeScrumAutoWorkColumns(stored.SourceColumns)
		}
	}
	cfg.SourceColumns = normalizeScrumAutoWorkColumns(cfg.SourceColumns)
	return cfg
}

func loadScrumAutoPlayThrough(settings json.RawMessage) bool {
	return loadScrumAutoWorkConfig(settings).Enabled
}

func (s *Server) scrumAutoPlayThroughEnabled(ctx context.Context, projectID int64) bool {
	return s.scrumAutoWorkConfig(ctx, projectID).Enabled
}

func (s *Server) scrumAutoWorkConfig(ctx context.Context, projectID int64) ScrumAutoWorkConfig {
	if s.repo == nil || projectID <= 0 {
		return defaultScrumAutoWorkConfig()
	}
	project, err := s.repo.GetProject(ctx, projectID)
	if err != nil {
		return defaultScrumAutoWorkConfig()
	}
	return loadScrumAutoWorkConfig(project.Settings)
}

func (s *Server) saveScrumAutoPlayThrough(ctx context.Context, project model.Project, enabled bool) error {
	cfg := loadScrumAutoWorkConfig(project.Settings)
	cfg.Enabled = enabled
	return s.saveScrumAutoWorkConfig(ctx, project, cfg)
}

func (s *Server) saveScrumAutoWorkConfig(ctx context.Context, project model.Project, cfg ScrumAutoWorkConfig) error {
	var settings map[string]any
	if len(project.Settings) > 0 {
		_ = json.Unmarshal(project.Settings, &settings)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	cfg.SourceColumns = normalizeScrumAutoWorkColumns(cfg.SourceColumns)
	settings[scrumAutoPlayThroughKey] = cfg.Enabled
	settings[scrumAutoWorkConfigKey] = cfg
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
	for _, column := range normalizeScrumAutoWorkColumns(defaultScrumAutoWorkColumns) {
		if next := s.nextAutoPlayCardInColumn(board, column); next != nil {
			return next
		}
	}
	return nil
}

func (s *Server) nextAutoWorkScrumCard(board ScrumBoard, cfg ScrumAutoWorkConfig) *ScrumCard {
	if next := s.nextQueuedScrumCard(board); next != nil {
		return next
	}
	for _, column := range normalizeScrumAutoWorkColumns(cfg.SourceColumns) {
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
	case "backlog", "ready", "assigned", "in_progress", "blocked":
		if card.PlayState == scrumPlayPaused {
			card.PlayState = ""
		}
		card = appendScrumChannelEvent(card, "system", fmt.Sprintf("Auto-work pulled card from %s", col))
		return s.persistScrumCard(r, projectID, card)
	default:
		return card, fmt.Errorf("card %s is not playable for auto-play", card.ID)
	}
}

func (s *Server) kickoffAutoPlayThrough(r *http.Request, projectID int64, board ScrumBoard) (ScrumBoard, error) {
	if s.findRunningScrumCard(board) != nil {
		return board, nil
	}
	autoWork := s.scrumAutoWorkConfig(r.Context(), projectID)
	if !autoWork.Enabled {
		return board, nil
	}
	reviewCfg := s.scrumAutoReviewConfig(r.Context(), projectID)
	if scrumAutoPlayThroughCompleteWithReview(board, reviewCfg.Enabled) {
		return board, nil
	}
	next := s.nextAutoWorkScrumCard(board, autoWork)
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
