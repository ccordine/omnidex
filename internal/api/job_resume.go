package api

import (
	"github.com/gryph/omnidex/internal/queue"
	"net/http"
)

func (s *Server) resumeJob(w http.ResponseWriter, r *http.Request, jobID int64) {
	req, err := decodeLifecycleFeedbackRequest(w, r)
	if err != nil {
		writeError(w, lifecycleControlBodyStatus(err), err.Error())
		return
	}
	if req.ExpectedGeneration == nil || req.ObservedLedgerRevision < 1 || req.ExpectedInputID != nil || req.ExpectedWaitingAttempt != nil {
		writeError(w, http.StatusBadRequest, "resume requires expected_generation and observed_ledger_revision, without input authority")
		return
	}
	if status, err := s.requireLifecycleWorkspaceIdentity(r.Context(), jobID, req.WorkspaceRoot.Value, req.WorkspaceIdentity.Value); err != nil {
		writeError(w, status, err.Error())
		return
	}
	s.codingAgentsMu.RLock()
	defer s.codingAgentsMu.RUnlock()
	result, err := s.repo.ResumeJob(r.Context(), queue.ResumeJobCommand{OperationID: req.OperationID, JobID: jobID, Feedback: req.Feedback, ExpectedGeneration: *req.ExpectedGeneration, ObservedLedgerRevision: req.ObservedLedgerRevision, WorkspaceRoot: req.WorkspaceRoot.Value, WorkspaceIdentity: req.WorkspaceIdentity.Value})
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	receipt, err := newLifecycleControlReceipt(jobID, req.OperationID, result.Job)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result.Applied {
		s.publishJobProgressForJob(result.Job, realtimeJobChanged, "Job resumed")
	}
	writeJSON(w, http.StatusOK, receipt)
}
