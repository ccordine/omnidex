package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"
)

type aiControlState struct {
	Paused       bool             `json:"paused"`
	CanceledJobs int64            `json:"canceled_jobs"`
	Resumed      bool             `json:"resumed"`
	Counts       map[string]int64 `json:"counts"`
	UpdatedAt    time.Time        `json:"updated_at"`
}

type aiControlResponse struct {
	Paused            bool             `json:"paused"`
	CanceledJobs      int64            `json:"canceled_jobs"`
	Resumed           bool             `json:"resumed"`
	Counts            map[string]int64 `json:"counts"`
	UpdatedAt         time.Time        `json:"updated_at"`
	RealtimePublished *bool            `json:"realtime_published,omitempty"`
	RealtimeError     string           `json:"realtime_error,omitempty"`
	OperationError    string           `json:"operation_error,omitempty"`
}

func (s *Server) handleAIControl(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "AI control requires database")
		return
	}
	switch r.Method {
	case http.MethodGet:
		payload, err := s.aiControlPayload(r.Context(), 0, false)
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json body")
			return
		}
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

func (s *Server) pauseAI(ctx context.Context) (aiControlResponse, error) {
	projectIDs, jobIDs, err := s.repo.PauseAI(ctx, "global AI pause")
	if err != nil {
		return aiControlResponse{}, err
	}
	var reconcileErrors []error
	for _, projectID := range projectIDs {
		if err := s.refreshScrumPlayQueueForProject(ctx, projectID, "global AI paused"); err != nil {
			reconcileErrors = append(reconcileErrors, fmt.Errorf("reconcile Scrum project %d: %w", projectID, err))
		}
	}
	response, err := s.aiControlMutationResponse(ctx, int64(len(jobIDs)), false, "paused")
	if err != nil {
		return aiControlResponse{}, err
	}
	if reconcileErr := errors.Join(reconcileErrors...); reconcileErr != nil {
		response.OperationError = reconcileErr.Error()
		log.Printf("AI pause committed with Scrum reconciliation failures: %v", reconcileErr)
	}
	return response, nil
}

func (s *Server) resumeAI(ctx context.Context) (aiControlResponse, error) {
	if s.lifecycleContext == nil {
		return aiControlResponse{}, ErrRealtimeLifecycleUnavailable
	}
	if err := s.lifecycleContext.Err(); err != nil {
		return aiControlResponse{}, fmt.Errorf("AI resume lifecycle is unavailable: %w", err)
	}
	if err := s.repo.SetAIPaused(ctx, false); err != nil {
		return aiControlResponse{}, err
	}
	response, err := s.aiControlMutationResponse(ctx, 0, true, "resumed")
	if err != nil {
		return aiControlResponse{}, err
	}
	if err := s.refreshScrumAutoWorkAsync(s.lifecycleContext); err != nil {
		response.OperationError = err.Error()
		log.Printf("AI resume committed but Scrum auto-work scheduling failed: %v", err)
	}
	return response, nil
}

func (s *Server) aiControlPayload(ctx context.Context, canceledJobs int64, resumed bool) (aiControlState, error) {
	paused, err := s.repo.IsAIPaused(ctx)
	if err != nil {
		return aiControlState{}, err
	}
	counts, err := s.repo.CountJobsByStatuses(ctx)
	if err != nil {
		return aiControlState{}, err
	}
	return aiControlState{
		Paused:       paused,
		CanceledJobs: canceledJobs,
		Resumed:      resumed,
		Counts:       counts,
		UpdatedAt:    time.Now().UTC(),
	}, nil
}

func (s *Server) aiControlMutationResponse(ctx context.Context, canceledJobs int64, resumed bool, reason string) (aiControlResponse, error) {
	state, err := s.aiControlPayload(ctx, canceledJobs, resumed)
	if err != nil {
		return aiControlResponse{}, err
	}
	published := true
	response := aiControlResponse{
		Paused:            state.Paused,
		CanceledJobs:      state.CanceledJobs,
		Resumed:           state.Resumed,
		Counts:            state.Counts,
		UpdatedAt:         state.UpdatedAt,
		RealtimePublished: &published,
	}
	err = s.publishAIControlUpdate(state, reason)
	if err != nil {
		published = false
		response.RealtimePublished = &published
		response.RealtimeError = err.Error()
		log.Printf("AI control saved but realtime publication failed reason=%q paused=%t: %v", reason, state.Paused, err)
	}
	return response, nil
}

func (s *Server) publishAIControlUpdate(state aiControlState, reason string) error {
	_, err := s.broadcastRealtimeChecked([]string{realtimeTopicUI, realtimeTopicJobs}, realtimeMessage{
		EventName: "ai-control-updated",
		StateKey:  "ai-control",
		Reason:    strings.TrimSpace(reason),
		AIControl: &state,
	})
	return err
}
