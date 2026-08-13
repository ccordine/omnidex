package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
)

const scrumAutoWorkConfigKey = "scrum_auto_work"
const scrumAutoWorkStartFailureNoteLimit = 1200

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

func validateScrumAutoWorkConfig(cfg ScrumAutoWorkConfig) (ScrumAutoWorkConfig, error) {
	if len(cfg.SourceColumns) == 0 {
		cfg.SourceColumns = append([]string{}, defaultScrumAutoWorkColumns...)
		return cfg, nil
	}
	seen := make(map[string]struct{}, len(cfg.SourceColumns))
	columns := make([]string, 0, len(cfg.SourceColumns))
	for _, raw := range cfg.SourceColumns {
		column := normalizeScrumColumn(strings.TrimSpace(raw))
		switch column {
		case "backlog", "ready", "assigned", "in_progress", "blocked":
		default:
			return ScrumAutoWorkConfig{}, fmt.Errorf("unsupported Scrum auto-work source column %q", raw)
		}
		if _, exists := seen[column]; exists {
			return ScrumAutoWorkConfig{}, fmt.Errorf("duplicate Scrum auto-work source column %q", column)
		}
		seen[column] = struct{}{}
		columns = append(columns, column)
	}
	cfg.SourceColumns = columns
	return cfg, nil
}

func loadScrumAutoWorkConfig(settings json.RawMessage) (ScrumAutoWorkConfig, error) {
	cfg := defaultScrumAutoWorkConfig()
	if len(bytes.TrimSpace(settings)) == 0 {
		return cfg, nil
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(settings, &payload); err != nil {
		return ScrumAutoWorkConfig{}, fmt.Errorf("decode project settings for Scrum auto-work: %w", err)
	}
	if _, legacy := payload["scrum_auto_play_through"]; legacy {
		return ScrumAutoWorkConfig{}, fmt.Errorf("legacy scrum_auto_play_through setting is unsupported; apply database migrations")
	}
	if _, removed := payload["scrum_auto_review"]; removed {
		return ScrumAutoWorkConfig{}, fmt.Errorf("removed scrum_auto_review setting has no compatibility path")
	}
	if raw, ok := payload[scrumAutoWorkConfigKey]; ok && len(raw) > 0 {
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return ScrumAutoWorkConfig{}, fmt.Errorf("Scrum auto-work config must be an object")
		}
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return ScrumAutoWorkConfig{}, fmt.Errorf("decode Scrum auto-work config: %w", err)
		}
	}
	return validateScrumAutoWorkConfig(cfg)
}

func (s *Server) saveScrumAutoWorkConfig(ctx context.Context, project model.Project, cfg ScrumAutoWorkConfig) error {
	if s == nil || s.repo == nil || project.ID <= 0 {
		return fmt.Errorf("postgres repository and project are required to save Scrum auto-work config")
	}
	validated, err := validateScrumAutoWorkConfig(cfg)
	if err != nil {
		return err
	}
	if _, err := loadScrumAutoWorkConfig(project.Settings); err != nil {
		return err
	}
	var settings map[string]any
	if len(project.Settings) > 0 {
		if err := json.Unmarshal(project.Settings, &settings); err != nil {
			return fmt.Errorf("decode project %d settings: %w", project.ID, err)
		}
	}
	if settings == nil {
		settings = map[string]any{}
	}
	settings[scrumAutoWorkConfigKey] = validated
	raw, err := json.Marshal(settings)
	if err != nil {
		return err
	}
	settingsJSON := json.RawMessage(raw)
	patch := model.ProjectPatch{Settings: &settingsJSON}
	_, err = s.repo.UpdateProject(ctx, project.ID, patch)
	return err
}

func (s *Server) nextAutoWorkScrumCard(board ScrumBoard, cfg ScrumAutoWorkConfig) *ScrumCard {
	if next := s.nextQueuedScrumCard(board); next != nil {
		return next
	}
	for _, column := range normalizeScrumAutoWorkColumns(cfg.SourceColumns) {
		if next := s.nextAutoWorkCardInColumn(board, column); next != nil {
			return next
		}
	}
	return nil
}

func (s *Server) nextAutoWorkCardInColumn(board ScrumBoard, column string) *ScrumCard {
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

func scrumAutoWorkComplete(board ScrumBoard) bool {
	for _, card := range board.Cards {
		col := normalizeScrumColumn(card.Column)
		switch col {
		case "review":
			continue
		case "done":
			continue
		default:
			return false
		}
	}
	return len(board.Cards) > 0
}

func (s *Server) prepareScrumCardForAutoWork(r *http.Request, projectID int64, card ScrumCard) (ScrumCard, error) {
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

func (s *Server) kickoffAutoWorkAfterReconcile(r *http.Request, projectID int64, board ScrumBoard) (ScrumBoard, error) {
	if s == nil || s.repo == nil || projectID <= 0 {
		return board, fmt.Errorf("postgres repository and project are required for Scrum auto-work")
	}
	if r == nil {
		if s.lifecycleContext == nil {
			return board, ErrRealtimeLifecycleUnavailable
		}
		r = scrumRequestForProject(s.lifecycleContext, projectID)
	}
	if scrumAutoWorkHandoffSuppressed(r.Context()) {
		return board, nil
	}
	if scrumAutoWorkLockHeld(r.Context()) {
		return s.startNextScrumAutoWork(r, projectID, board)
	}
	s.scrumAutoWorkMu.Lock()
	defer s.scrumAutoWorkMu.Unlock()
	ctx := context.WithValue(r.Context(), scrumAutoWorkLockHeldKey{}, true)
	return s.startNextScrumAutoWork(r.WithContext(ctx), projectID, board)
}

func (s *Server) startNextScrumAutoWork(r *http.Request, projectID int64, board ScrumBoard) (ScrumBoard, error) {
	if r == nil {
		if s.lifecycleContext == nil {
			return board, ErrRealtimeLifecycleUnavailable
		}
		r = scrumRequestForProject(s.lifecycleContext, projectID)
	}
	automation, err := s.scrumAutomationSettings(r.Context(), projectID)
	if err != nil {
		return board, err
	}
	autoWork := automation.AutoWork
	if !autoWork.Enabled {
		return board, nil
	}
	attempts := len(normalizeScrumAutoWorkColumns(autoWork.SourceColumns)) + 1
	for attempts > 0 {
		attempts--
		running, err := s.runningScrumCard(r.Context(), projectID)
		if err != nil {
			return board, err
		}
		if running != nil {
			return board, nil
		}
		globalPlayActive, err := s.scrumGlobalPlayActive(r.Context())
		if err != nil {
			return board, err
		}
		if globalPlayActive {
			return board, nil
		}
		if paused, err := s.repo.IsAIPaused(r.Context()); err != nil {
			return board, err
		} else if paused {
			return board, nil
		}
		complete, err := s.repo.ScrumProjectComplete(r.Context(), projectID)
		if err != nil {
			return board, err
		}
		if complete {
			return board, nil
		}
		next, err := s.nextScrumWorkCard(r.Context(), projectID, normalizeScrumAutoWorkColumns(autoWork.SourceColumns))
		if err != nil {
			return board, err
		}
		if next == nil {
			return board, nil
		}
		prepared, err := s.prepareScrumCardForAutoWork(r, projectID, *next)
		if err != nil {
			if _, markErr := s.markScrumAutoWorkStartFailure(r, projectID, *next, err); markErr != nil {
				return board, markErr
			}
			board, err = s.reloadScrumBoardAfterAutoWork(r.Context(), projectID, board)
			if err != nil {
				return board, err
			}
			continue
		}
		if _, err := s.startScrumCardPlay(r, board, projectID, prepared.ID, agentconfig.Config{}); err != nil {
			if scrumAutoWorkStartErrorIsGlobalPause(err) {
				log.Printf("scrum auto-work start paused project=%d card=%s: %v", projectID, prepared.ID, err)
				return board, nil
			}
			if _, markErr := s.markScrumAutoWorkStartFailure(r, projectID, prepared, err); markErr != nil {
				return board, markErr
			}
			board, err = s.reloadScrumBoardAfterAutoWork(r.Context(), projectID, board)
			if err != nil {
				return board, err
			}
			continue
		}
		return s.reloadScrumBoardAfterAutoWork(r.Context(), projectID, board)
	}
	return board, nil
}

func (s *Server) reloadScrumBoardAfterAutoWork(ctx context.Context, projectID int64, _ ScrumBoard) (ScrumBoard, error) {
	if s.repo == nil || projectID <= 0 {
		return ScrumBoard{}, fmt.Errorf("postgres repository and project are required for Scrum auto-work")
	}
	return s.scrumBoardFromProject(ctx, projectID)
}

func (s *Server) markScrumAutoWorkStartFailure(r *http.Request, projectID int64, card ScrumCard, cause error) (ScrumCard, error) {
	reason := "unknown error"
	if cause != nil {
		reason = strings.TrimSpace(cause.Error())
	}
	if reason == "" {
		reason = "unknown error"
	}
	if len(reason) > scrumAutoWorkStartFailureNoteLimit {
		reason = truncateScrumChannelText(reason, scrumAutoWorkStartFailureNoteLimit, "...")
	}
	log.Printf("scrum auto-work start failed project=%d card=%s moving_to=error: %s", projectID, card.ID, reason)
	card.Column = "error"
	card.PlayState = ""
	card.QueueOrder = 0
	card = appendScrumChannelEvent(card, "error", "Auto-work failed to start: "+reason)
	saved, err := s.persistScrumCard(r, projectID, card)
	if err != nil {
		return ScrumCard{}, err
	}
	s.publishScrumCardUpdate(r.Context(), projectID, saved, "auto-work start failed")
	board, err := s.reloadScrumBoardAfterAutoWork(r.Context(), projectID, ScrumBoard{})
	if err != nil {
		return ScrumCard{}, fmt.Errorf("reload Scrum board after recording auto-work failure: %w", err)
	}
	if err := s.publishScrumBoardRefreshWithToast(r.Context(), projectID, "auto-work start failed", board, "Auto-work moved a failed start to error", "error"); err != nil {
		return ScrumCard{}, fmt.Errorf("publish auto-work failure board state: %w", err)
	}
	return saved, nil
}

func scrumAutoWorkStartErrorIsGlobalPause(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(strings.ToLower(err.Error()), "ai is globally paused")
}
