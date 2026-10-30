package api

import (
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) planCodingLedgerItem(w http.ResponseWriter, r *http.Request, jobID int64) {
	var request struct {
		Manual            bool                       `json:"manual,omitempty"`
		OperationID       model.LifecycleOperationID `json:"operation_id"`
		ItemID            model.LedgerItemID         `json:"item_id"`
		Text              string                     `json:"text"`
		Revision          int64                      `json:"revision"`
		WorkspaceRoot     lifecycleWorkspaceValue    `json:"workspace_root"`
		WorkspaceIdentity lifecycleWorkspaceValue    `json:"workspace_identity"`
	}
	if err := decodeLifecycleControlObject(w, r, "item planner message", &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if request.ItemID == "" && !request.Manual {
		writeError(w, http.StatusBadRequest, "select an item to adjust")
		return
	}
	if err := validateRequiredLifecycleWorkspaceRequest(request.WorkspaceRoot, request.WorkspaceIdentity); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if status, err := s.requireLifecycleWorkspaceIdentity(r.Context(), jobID, request.WorkspaceRoot.Value, request.WorkspaceIdentity.Value); err != nil {
		writeError(w, status, err.Error())
		return
	}
	result, err := s.repo.ReplanJob(r.Context(), queue.ReplanJobCommand{OperationID: request.OperationID, JobID: jobID, Feedback: request.Text, ObservedLedgerRevision: request.Revision, WorkspaceRoot: request.WorkspaceRoot.Value, WorkspaceIdentity: request.WorkspaceIdentity.Value, PlanningItemID: request.ItemID, ManualPlanning: request.Manual})
	if err != nil {
		writeError(w, codingPlanMutationStatus(err), err.Error())
		return
	}
	receipt, err := newLifecycleControlReceipt(jobID, request.OperationID, result.Job)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result.Applied {
		s.publishJobProgressForJob(result.Job, realtimeJobChanged, "Planner adjusting selected ledger item")
	}
	writeJSON(w, http.StatusOK, receipt)
}
