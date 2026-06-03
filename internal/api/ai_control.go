package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func (s *Server) handleAIControl(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "AI control requires database")
		return
	}
	switch r.Method {
	case http.MethodGet:
		payload, err := s.aiControlPayload(r.Context(), 0, 0)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, payload)
	case http.MethodPost, http.MethodPatch:
		var req struct {
			Action string `json:"action"`
			Paused *bool  `json:"paused"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		action := strings.TrimSpace(strings.ToLower(req.Action))
		if req.Paused != nil {
			if *req.Paused {
				action = "pause"
			} else {
				action = "resume"
			}
		}
		switch action {
		case "pause":
			payload, err := s.pauseAI(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, payload)
		case "resume", "play":
			payload, err := s.resumeAI(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, http.StatusOK, payload)
		default:
			writeError(w, http.StatusBadRequest, "action must be pause or resume")
		}
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) pauseAI(ctx context.Context) (map[string]any, error) {
	if err := s.repo.SetAIPaused(ctx, true); err != nil {
		return nil, err
	}
	projectIDs, _ := s.repo.ListRunningScrumPlayProjectIDs(ctx)
	jobIDs, err := s.repo.ListJobIDsByStatuses(ctx, model.JobStatusRunning, model.JobStatusWaiting)
	if err != nil {
		return nil, err
	}
	canceled := int64(0)
	for _, jobID := range jobIDs {
		if _, err := s.repo.CancelJob(ctx, jobID, "global AI pause"); err == nil {
			canceled++
		}
	}
	for _, projectID := range projectIDs {
		_ = s.refreshScrumPlayQueueForProject(ctx, projectID, "global AI paused")
	}
	return s.aiControlPayload(ctx, canceled, 0)
}

func (s *Server) resumeAI(ctx context.Context) (map[string]any, error) {
	if err := s.repo.SetAIPaused(ctx, false); err != nil {
		return nil, err
	}
	s.RefreshScrumAutoWorkAsync()
	return s.aiControlPayload(ctx, 0, 1)
}

func (s *Server) aiControlPayload(ctx context.Context, canceledJobs int64, resumed int64) (map[string]any, error) {
	paused, err := s.repo.IsAIPaused(ctx)
	if err != nil {
		return nil, err
	}
	counts, err := s.repo.CountJobsByStatuses(ctx)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"paused":        paused,
		"canceled_jobs": canceledJobs,
		"resumed":       resumed > 0,
		"counts":        counts,
		"updated_at":    time.Now().UTC(),
	}, nil
}
