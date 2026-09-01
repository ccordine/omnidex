package api

import (
	"errors"
	"net/http"

	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

type codingPlanDecisionsRequest struct {
	OperationID       queue.LifecycleOperationID       `json:"operation_id"`
	Generation        int64                            `json:"generation"`
	Revision          int64                            `json:"revision"`
	Decisions         []queue.CodingPlanDecisionChange `json:"decisions"`
	WorkspaceRoot     lifecycleWorkspaceValue          `json:"workspace_root,omitempty"`
	WorkspaceIdentity lifecycleWorkspaceValue          `json:"workspace_identity,omitempty"`
}

type codingPlanFreezeRequest struct {
	OperationID       queue.LifecycleOperationID `json:"operation_id"`
	Generation        int64                      `json:"generation"`
	Revision          int64                      `json:"revision"`
	WorkspaceRoot     lifecycleWorkspaceValue    `json:"workspace_root,omitempty"`
	WorkspaceIdentity lifecycleWorkspaceValue    `json:"workspace_identity,omitempty"`
}

func (s *Server) writeCurrentCodingPlan(w http.ResponseWriter, r *http.Request, jobID int64) {
	workspaceRoot, workspaceIdentity, err := requiredLifecycleWorkspaceQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.requireServerWorkspaceIdentity(workspaceRoot, workspaceIdentity); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	plan, err := s.repo.CurrentCodingPlanForWorkspace(
		r.Context(), jobID, workspaceRoot, workspaceIdentity,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "coding plan not found")
			return
		}
		if errors.Is(err, queue.ErrChannelSessionWorkspace) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, plan)
}

func (s *Server) applyCodingPlanDecisions(w http.ResponseWriter, r *http.Request, jobID int64) {
	var request codingPlanDecisionsRequest
	if err := decodeLifecycleControlObject(
		w, r, "coding plan decisions", &request,
	); err != nil {
		writeError(w, lifecycleControlBodyStatus(err), err.Error())
		return
	}
	if err := validateRequiredLifecycleWorkspaceRequest(
		request.WorkspaceRoot, request.WorkspaceIdentity,
	); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.requireServerWorkspaceIdentity(
		request.WorkspaceRoot.Value, request.WorkspaceIdentity.Value,
	); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	result, err := s.repo.ApplyCodingPlanDecisions(r.Context(), queue.ApplyCodingPlanDecisionsCommand{
		OperationID: request.OperationID, JobID: jobID,
		Generation: request.Generation, Revision: request.Revision,
		Decisions:         request.Decisions,
		WorkspaceRoot:     request.WorkspaceRoot.Value,
		WorkspaceIdentity: request.WorkspaceIdentity.Value,
	})
	if err != nil {
		writeError(w, codingPlanMutationStatus(err), err.Error())
		return
	}
	if result.Applied {
		s.publishJobProgressForJob(result.Job, realtimeJobChanged, "Coding plan decisions updated")
	}
	writeJSON(w, http.StatusOK, result.Plan)
}

func (s *Server) freezeCodingPlan(w http.ResponseWriter, r *http.Request, jobID int64) {
	var request codingPlanFreezeRequest
	if err := decodeLifecycleControlObject(
		w, r, "coding plan freeze", &request,
	); err != nil {
		writeError(w, lifecycleControlBodyStatus(err), err.Error())
		return
	}
	if err := validateRequiredLifecycleWorkspaceRequest(
		request.WorkspaceRoot, request.WorkspaceIdentity,
	); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.requireServerWorkspaceIdentity(
		request.WorkspaceRoot.Value, request.WorkspaceIdentity.Value,
	); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	result, err := s.repo.FreezeCodingPlan(r.Context(), queue.FreezeCodingPlanCommand{
		OperationID: request.OperationID, JobID: jobID,
		Generation: request.Generation, Revision: request.Revision,
		WorkspaceRoot:     request.WorkspaceRoot.Value,
		WorkspaceIdentity: request.WorkspaceIdentity.Value,
	})
	if err != nil {
		writeError(w, codingPlanMutationStatus(err), err.Error())
		return
	}
	if result.Applied {
		s.publishJobProgressForJob(result.Job, realtimeJobChanged, "Coding plan approved and frozen")
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"plan": result.Plan, "job_status": result.Job.Status,
	})
}

func codingPlanMutationStatus(err error) int {
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return http.StatusNotFound
	case errors.Is(err, queue.ErrCodingPlanRevision),
		errors.Is(err, queue.ErrCodingPlanState),
		errors.Is(err, queue.ErrLifecycleOperationConflict),
		errors.Is(err, queue.ErrChannelSessionWorkspace):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}
