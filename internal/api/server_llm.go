package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func safeStatusError(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unavailable"
	}
	if len(value) > 300 {
		return value[:300] + "...[truncated]"
	}
	return value
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	id, action, err := decodeJobItemRoute(r)
	if err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if action == jobItemHistory {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.jobHistory(w, r, id)
		return
	}
	if action != jobItemRead {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := validateExactQuery(r); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		switch action {
		case jobItemFeedback:
			s.submitJobFeedback(w, r, id)
		case jobItemInterrupt:
			s.interruptJob(w, r, id)
		case jobItemReplan:
			s.replanJob(w, r, id)
		case jobItemCancel:
			s.cancelJob(w, r, id)
		}
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.writeCurrentJobDetails(w, r, id)
}

func (s *Server) writeCurrentJobDetails(w http.ResponseWriter, r *http.Request, id int64) {
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	details, err := s.repo.CurrentJobDetails(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, details)
}

func (s *Server) submitJobFeedback(w http.ResponseWriter, r *http.Request, jobID int64) {
	req, err := decodeLifecycleFeedbackRequest(w, r)
	if err != nil {
		writeError(w, lifecycleControlBodyStatus(err), err.Error())
		return
	}
	if err := s.requireOptionalLifecycleWorkspaceIdentity(
		req.WorkspaceRoot.Value,
		req.WorkspaceIdentity.Value,
	); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	result, err := s.repo.SubmitJobFeedback(r.Context(), queue.SubmitJobFeedbackCommand{
		OperationID: req.OperationID, JobID: jobID, Feedback: req.Feedback,
		WorkspaceRoot: req.WorkspaceRoot.Value, WorkspaceIdentity: req.WorkspaceIdentity.Value,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job has no pending input request")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	receipt, err := newLifecycleControlReceipt(jobID, req.OperationID, result.Job)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result.Applied {
		s.publishJobProgressForJob(result.Job, realtimeJobChanged, "Job feedback accepted")
	}

	writeJSON(w, http.StatusOK, receipt)
}

func (s *Server) interruptJob(w http.ResponseWriter, r *http.Request, jobID int64) {
	req, err := decodeLifecycleFeedbackRequest(w, r)
	if err != nil {
		writeError(w, lifecycleControlBodyStatus(err), err.Error())
		return
	}
	if err := s.requireOptionalLifecycleWorkspaceIdentity(
		req.WorkspaceRoot.Value,
		req.WorkspaceIdentity.Value,
	); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	result, err := s.repo.InterruptJob(r.Context(), queue.ReplanJobCommand{
		OperationID: req.OperationID, JobID: jobID, Feedback: req.Feedback,
		WorkspaceRoot: req.WorkspaceRoot.Value, WorkspaceIdentity: req.WorkspaceIdentity.Value,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	receipt, err := newLifecycleControlReceipt(jobID, req.OperationID, result.Job)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result.Applied {
		s.publishJobProgressForJob(result.Job, realtimeJobChanged, "Job interruption accepted")
	}

	writeJSON(w, http.StatusOK, receipt)
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request, jobID int64) {
	req, err := decodeLifecycleCancelRequest(w, r)
	if err != nil {
		writeError(w, lifecycleControlBodyStatus(err), err.Error())
		return
	}
	if err := s.requireOptionalLifecycleWorkspaceIdentity(
		req.WorkspaceRoot.Value,
		req.WorkspaceIdentity.Value,
	); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	result, err := s.repo.CancelJob(r.Context(), queue.CancelJobCommand{
		OperationID: req.OperationID, JobID: jobID, Reason: req.Reason,
		WorkspaceRoot: req.WorkspaceRoot.Value, WorkspaceIdentity: req.WorkspaceIdentity.Value,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, cancelJobHTTPStatus(err), err.Error())
		return
	}
	receipt, err := newLifecycleControlReceipt(jobID, req.OperationID, result.Job)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result.Applied {
		s.publishJobProgressForJob(result.Job, realtimeJobFinished, "Job canceled")
	}

	writeJSON(w, http.StatusOK, receipt)
}

func cancelJobHTTPStatus(err error) int {
	if errors.Is(err, queue.ErrLifecycleOperationConflict) || errors.Is(err, queue.ErrStepNotWritable) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}
