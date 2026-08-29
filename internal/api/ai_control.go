package api

import (
	"context"
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
	CommitState       aiControlCommitState `json:"commit_state"`
	Paused            bool                 `json:"paused"`
	CanceledJobs      int64                `json:"canceled_jobs"`
	Resumed           bool                 `json:"resumed"`
	Counts            map[string]int64     `json:"counts,omitempty"`
	UpdatedAt         time.Time            `json:"updated_at"`
	RealtimePublished *bool                `json:"realtime_published,omitempty"`
	RealtimeError     string               `json:"realtime_error,omitempty"`
	OperationError    string               `json:"operation_error,omitempty"`
}

type aiControlCommitState string

const (
	aiControlCommitted         aiControlCommitState = "committed"
	aiControlCommittedDegraded aiControlCommitState = "committed_degraded"
)

func (s *Server) handleAIControl(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "AI control requires database")
		return
	}
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
		request, err := decodeAIControlRequest(w, r)
		if err != nil {
			writeError(w, aiControlRequestErrorStatus(err), err.Error())
			return
		}
		switch request.Action {
		case aiControlActionPause:
			payload, err := s.pauseAI(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, aiControlMutationHTTPStatus(payload), payload)
		case aiControlActionResume:
			payload, err := s.resumeAI(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, err.Error())
				return
			}
			writeJSON(w, aiControlMutationHTTPStatus(payload), payload)
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
	response, err := s.aiControlMutationResponse(ctx, int64(len(jobIDs)), false, true, "paused")
	if err != nil {
		return aiControlResponse{}, err
	}
	if reconcileErr := errors.Join(reconcileErrors...); reconcileErr != nil {
		addAIControlOperationError(&response, reconcileErr)
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
	response, err := s.aiControlMutationResponse(ctx, 0, true, false, "resumed")
	if err != nil {
		return aiControlResponse{}, err
	}
	if err := s.refreshScrumAutoWorkAsync(s.lifecycleContext); err != nil {
		addAIControlOperationError(&response, err)
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

func (s *Server) aiControlMutationResponse(
	ctx context.Context,
	canceledJobs int64,
	resumed bool,
	committedPaused bool,
	reason string,
) (aiControlResponse, error) {
	now := time.Now().UTC()
	published := false
	response := aiControlResponse{
		CommitState:       aiControlCommitted,
		Paused:            committedPaused,
		CanceledJobs:      canceledJobs,
		Resumed:           resumed,
		UpdatedAt:         now,
		RealtimePublished: &published,
	}
	state, err := s.aiControlPayload(ctx, canceledJobs, resumed)
	if err != nil {
		addAIControlOperationError(&response, fmt.Errorf(
			"AI control state committed, but authoritative status reconciliation failed: %w", err,
		))
		log.Printf("AI control committed but authoritative state could not be reloaded reason=%q paused=%t: %v", reason, committedPaused, err)
		return response, nil
	}
	if state.Paused != committedPaused {
		addAIControlOperationError(&response, fmt.Errorf(
			"AI control committed paused=%t, but authoritative reconciliation returned paused=%t",
			committedPaused, state.Paused,
		))
		return response, nil
	}
	response.Counts = state.Counts
	response.UpdatedAt = state.UpdatedAt
	published = true
	response.RealtimePublished = &published
	err = s.publishAIControlUpdate(state, reason)
	if err != nil {
		response.CommitState = aiControlCommittedDegraded
		published = false
		response.RealtimePublished = &published
		response.RealtimeError = err.Error()
		log.Printf("AI control saved but realtime publication failed reason=%q paused=%t: %v", reason, state.Paused, err)
	}
	return response, nil
}

func addAIControlOperationError(response *aiControlResponse, err error) {
	if response == nil || err == nil {
		return
	}
	response.CommitState = aiControlCommittedDegraded
	if response.OperationError == "" {
		response.OperationError = err.Error()
		return
	}
	response.OperationError = errors.Join(errors.New(response.OperationError), err).Error()
}

func aiControlMutationHTTPStatus(response aiControlResponse) int {
	if response.CommitState == aiControlCommittedDegraded ||
		response.OperationError != "" || response.RealtimeError != "" {
		return http.StatusMultiStatus
	}
	return http.StatusOK
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
