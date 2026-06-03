package api

import (
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/agentconfig"
)

func (s *Server) handleProjectPlay(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "project play requires database")
		return
	}
	board, card, message, err := s.startProjectAutoWork(r, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id": id,
		"board":      board,
		"card":       card,
		"job_id":     card.JobID,
		"play_queue": scrumPlayQueueSummary(board),
		"message":    message,
	})
}

func (s *Server) startProjectAutoWork(r *http.Request, projectID int64) (ScrumBoard, ScrumCard, string, error) {
	if projectID <= 0 {
		return ScrumBoard{}, ScrumCard{}, "", fmt.Errorf("project not found")
	}
	project, err := s.repo.GetProject(r.Context(), projectID)
	if err != nil {
		return ScrumBoard{}, ScrumCard{}, "", err
	}
	cfg := loadScrumAutoWorkConfig(project.Settings)
	if !cfg.Enabled {
		cfg.Enabled = true
		if err := s.saveScrumAutoWorkConfig(r.Context(), project, cfg); err != nil {
			return ScrumBoard{}, ScrumCard{}, "", err
		}
	}
	board, err := s.scrumBoardFromProject(r.Context(), projectID)
	if err != nil {
		return ScrumBoard{}, ScrumCard{}, "", err
	}
	refreshed, err := s.refreshScrumPlayQueue(r, projectID, board)
	if err != nil {
		return ScrumBoard{}, ScrumCard{}, "", err
	}
	if running := s.findRunningScrumCard(refreshed); running != nil {
		s.publishScrumBoardRefresh(r.Context(), projectID, "auto-work already running", refreshed)
		return refreshed, *running, "auto-work is already running for project", nil
	}
	next := s.nextAutoWorkScrumCard(refreshed, cfg)
	if next == nil {
		s.publishScrumBoardRefresh(r.Context(), projectID, "auto-work started", refreshed)
		s.RefreshScrumAutoWorkAsync()
		return refreshed, ScrumCard{}, "auto-work enabled; no eligible cards found", nil
	}
	started, err := s.startScrumCardPlay(r, refreshed, projectID, next.ID, agentconfig.Config{})
	if err != nil {
		return ScrumBoard{}, ScrumCard{}, "", err
	}
	refreshed, err = s.scrumBoardFromProject(r.Context(), projectID)
	if err != nil {
		return ScrumBoard{}, ScrumCard{}, "", err
	}
	s.publishScrumBoardRefresh(r.Context(), projectID, "auto-work started", refreshed)
	s.RefreshScrumAutoWorkAsync()
	return refreshed, started, "auto-work enabled and job queued", nil
}
