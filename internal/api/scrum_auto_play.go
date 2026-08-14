package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/queue"
)

type ScrumAutoWorkConfig struct {
	Enabled       bool     `json:"enabled"`
	SourceColumns []string `json:"source_columns"`
}

func defaultScrumAutoWorkConfig() ScrumAutoWorkConfig {
	stored := queue.DefaultScrumAutoWorkConfig()
	return ScrumAutoWorkConfig{Enabled: stored.Enabled, SourceColumns: stored.SourceColumns}
}

func validateScrumAutoWorkConfig(cfg ScrumAutoWorkConfig) (ScrumAutoWorkConfig, error) {
	stored, err := queue.ValidateScrumAutoWorkConfig(queue.ScrumAutoWorkConfig{
		Enabled: cfg.Enabled, SourceColumns: cfg.SourceColumns,
	})
	return ScrumAutoWorkConfig{Enabled: stored.Enabled, SourceColumns: stored.SourceColumns}, err
}

func loadScrumAutoWorkConfig(settings json.RawMessage) (ScrumAutoWorkConfig, error) {
	stored, err := queue.DecodeScrumAutoWorkConfig(settings)
	return ScrumAutoWorkConfig{Enabled: stored.Enabled, SourceColumns: stored.SourceColumns}, err
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
	result, err := s.repo.ApplyProjectAutoWork(r.Context(), queue.ProjectAutoWorkCommand{
		ProjectID: projectID, Action: queue.ProjectAutoWorkPlay,
	})
	if err != nil {
		return board, err
	}
	if result.ActiveCard == nil {
		return board, nil
	}
	return s.reloadScrumBoardAfterAutoWork(r.Context(), projectID, board)
}

func (s *Server) reloadScrumBoardAfterAutoWork(ctx context.Context, projectID int64, _ ScrumBoard) (ScrumBoard, error) {
	if s.repo == nil || projectID <= 0 {
		return ScrumBoard{}, fmt.Errorf("postgres repository and project are required for Scrum auto-work")
	}
	return s.scrumBoardMetadataFromProject(ctx, projectID)
}
