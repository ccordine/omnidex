package api

import (
	"errors"
	"net/http"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

type codingPlanDecisionsRequest struct {
	OperationID       string                           `json:"operation_id"`
	OwnerTurnID       string                           `json:"owner_turn_id"`
	OwnerTurnText     string                           `json:"owner_turn_text"`
	Revision          int64                            `json:"revision"`
	Action            codingagent.DirectPlanningAction `json:"action"`
	WorkspaceRoot     lifecycleWorkspaceValue          `json:"workspace_root,omitempty"`
	WorkspaceIdentity lifecycleWorkspaceValue          `json:"workspace_identity,omitempty"`
}

type codingPlanFreezeRequest struct {
	OperationID       string                  `json:"operation_id"`
	OwnerTurnID       string                  `json:"owner_turn_id"`
	OwnerTurnText     string                  `json:"owner_turn_text"`
	Revision          int64                   `json:"revision"`
	WorkspaceRoot     lifecycleWorkspaceValue `json:"workspace_root,omitempty"`
	WorkspaceIdentity lifecycleWorkspaceValue `json:"workspace_identity,omitempty"`
}

func (s *Server) writeCurrentCodingPlan(w http.ResponseWriter, r *http.Request, jobID int64) {
	workspaceRoot, workspaceIdentity, err := requiredLifecycleWorkspaceQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.requireJobWorkspaceIdentity(r.Context(), jobID, workspaceRoot, workspaceIdentity); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	plan, found, err := s.repo.CurrentCodingLedgerForJob(r.Context(), jobID)
	if err != nil || !found {
		if errors.Is(err, pgx.ErrNoRows) || !found {
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
	if err := s.requireJobWorkspaceIdentity(r.Context(), jobID,
		request.WorkspaceRoot.Value, request.WorkspaceIdentity.Value,
	); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.codingAgentsMu.RLock()
	defer s.codingAgentsMu.RUnlock()
	current, found, err := s.repo.CurrentCodingLedgerForJob(r.Context(), jobID)
	if err != nil || !found {
		if err == nil {
			err = pgx.ErrNoRows
		}
		writeError(w, codingPlanMutationStatus(err), err.Error())
		return
	}
	turn := codingagent.PlanningTurn{OperationID: request.OperationID, OwnerTurnID: request.OwnerTurnID,
		Text: request.OwnerTurnText, ObservedRevision: request.Revision}
	mutation, err := codingagent.DirectPlanningMutation(turn, request.Action)
	if err == nil {
		var result queue.CodingLedgerRevision
		result, err = s.repo.ApplyCodingLedgerMutation(r.Context(), queue.ApplyCodingLedgerMutationCommand{
			LedgerID: current.LedgerID, OwnerTurnText: request.OwnerTurnText, Mutation: mutation,
			CoveragePassed: !codingagent.RequiresPlanningCoverage(mutation),
		})
		if err == nil && codingagent.RequiresPlanningCoverage(mutation) {
			if !result.Applied {
				var found bool
				result, found, err = s.repo.CurrentCodingLedgerForJob(r.Context(), jobID)
				if err == nil && !found {
					err = pgx.ErrNoRows
				}
			}
		}
		if err == nil && codingagent.RequiresPlanningCoverage(mutation) {
			result, err = s.analyzeDirectPlanningCoverage(r.Context(), jobID, request.WorkspaceRoot.Value, turn, result)
		}
		if err == nil {
			writeJSON(w, http.StatusOK, result)
			return
		}
	}
	if err != nil {
		writeError(w, codingPlanMutationStatus(err), err.Error())
		return
	}
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
	if err := s.requireJobWorkspaceIdentity(r.Context(), jobID,
		request.WorkspaceRoot.Value, request.WorkspaceIdentity.Value,
	); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	s.codingAgentsMu.RLock()
	defer s.codingAgentsMu.RUnlock()
	result, err := s.auditAndFreezeCodingLedger(r.Context(), jobID, request)
	if err != nil {
		writeError(w, codingPlanMutationStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
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
	case errors.Is(err, queue.ErrCodingLedgerRevision), errors.Is(err, queue.ErrCodingLedgerState):
		return http.StatusConflict
	default:
		return http.StatusBadRequest
	}
}
