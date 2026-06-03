package api

import (
	"fmt"
	"net/http"
	"strings"

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

func (s *Server) handleProjectPause(w http.ResponseWriter, r *http.Request, id int64) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "project pause requires database")
		return
	}
	board, paused, message, err := s.pauseProjectAutoWork(r, id)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"project_id": id,
		"board":      board,
		"paused":     paused,
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
		return refreshed, *running, "auto-work is already running for project", nil
	}
	next := s.nextAutoWorkScrumCard(refreshed, cfg)
	if next == nil {
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
	s.RefreshScrumAutoWorkAsync()
	return refreshed, started, "auto-work enabled and job queued", nil
}

func (s *Server) pauseProjectAutoWork(r *http.Request, projectID int64) (ScrumBoard, int, string, error) {
	if projectID <= 0 {
		return ScrumBoard{}, 0, "", fmt.Errorf("project not found")
	}
	project, err := s.repo.GetProject(r.Context(), projectID)
	if err != nil {
		return ScrumBoard{}, 0, "", err
	}
	cfg := loadScrumAutoWorkConfig(project.Settings)
	if cfg.Enabled {
		cfg.Enabled = false
		if err := s.saveScrumAutoWorkConfig(r.Context(), project, cfg); err != nil {
			return ScrumBoard{}, 0, "", err
		}
	}
	board, err := s.scrumBoardFromProject(r.Context(), projectID)
	if err != nil {
		return ScrumBoard{}, 0, "", err
	}
	paused := 0
	for _, card := range board.Cards {
		switch card.PlayState {
		case scrumPlayRunning, scrumPlayQueued, scrumPlayReviewing:
		default:
			continue
		}
		if jobID, err := parseJobID(strings.TrimSpace(card.JobID)); err == nil && jobID > 0 {
			_, _ = s.repo.CancelJob(r.Context(), jobID, "project auto-work paused")
		}
		card.Column = "assigned"
		card.PlayState = scrumPlayPaused
		card.QueueOrder = 0
		card = appendScrumChannelEvent(card, "system", "Project auto-work paused")
		if saved, err := s.persistScrumCard(r, projectID, card); err == nil {
			s.publishScrumModalCardRefresh(r.Context(), projectID, saved, "project auto-work paused")
			paused++
		}
	}
	refreshed, err := s.scrumBoardFromProject(r.Context(), projectID)
	if err != nil {
		return ScrumBoard{}, 0, "", err
	}
	refreshed, err = s.refreshScrumPlayQueue(r, projectID, refreshed)
	if err != nil {
		return ScrumBoard{}, 0, "", err
	}
	if paused > 0 {
		return refreshed, paused, fmt.Sprintf("auto-work paused; stopped %d active cards", paused), nil
	}
	return refreshed, paused, "auto-work paused", nil
}
