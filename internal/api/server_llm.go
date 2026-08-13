package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
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
	idText := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
	idText = strings.TrimSpace(strings.Trim(idText, "/"))
	if idText == "" {
		writeError(w, http.StatusBadRequest, "job id is required")
		return
	}

	if strings.HasSuffix(idText, "/history") {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/history")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		if idText == "" {
			writeError(w, http.StatusBadRequest, "job id is required")
			return
		}
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		s.jobHistory(w, r, id)
		return
	}

	if strings.HasSuffix(idText, "/feedback") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/feedback")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		if idText == "" {
			writeError(w, http.StatusBadRequest, "job id is required")
			return
		}
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		s.submitJobFeedback(w, r, id)
		return
	}

	if strings.HasSuffix(idText, "/interrupt") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/interrupt")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		if idText == "" {
			writeError(w, http.StatusBadRequest, "job id is required")
			return
		}
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		s.interruptJob(w, r, id)
		return
	}

	if strings.HasSuffix(idText, "/replan") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/replan")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		if idText == "" {
			writeError(w, http.StatusBadRequest, "job id is required")
			return
		}
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		s.replanJob(w, r, id)
		return
	}

	if strings.HasSuffix(idText, "/cancel") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/cancel")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		if idText == "" {
			writeError(w, http.StatusBadRequest, "job id is required")
			return
		}
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid job id")
			return
		}
		s.cancelJob(w, r, id)
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid job id")
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
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	req.Feedback = strings.TrimSpace(req.Feedback)
	if req.Feedback == "" {
		writeError(w, http.StatusBadRequest, "feedback is required")
		return
	}
	if _, err := queue.ParseLifecycleOperationID(string(req.OperationID)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	job, err := s.repo.SubmitJobFeedback(r.Context(), queue.SubmitJobFeedbackCommand{
		OperationID: req.OperationID, JobID: jobID, Feedback: req.Feedback,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job has no pending input request")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateSameJobAuthority(jobID, job); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishJobProgress(jobID, realtimeJobChanged, "Job feedback accepted")

	writeJSON(w, http.StatusOK, map[string]any{"job": job})
}

func (s *Server) interruptJob(w http.ResponseWriter, r *http.Request, jobID int64) {
	var req feedbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	req.Feedback = strings.TrimSpace(req.Feedback)
	if req.Feedback == "" {
		writeError(w, http.StatusBadRequest, "feedback is required")
		return
	}
	if _, err := queue.ParseLifecycleOperationID(string(req.OperationID)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	job, err := s.repo.InterruptJob(r.Context(), queue.ReplanJobCommand{
		OperationID: req.OperationID, JobID: jobID, Feedback: req.Feedback,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateSameJobAuthority(jobID, job); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.publishJobProgress(jobID, realtimeJobChanged, "Job interruption accepted")

	writeJSON(w, http.StatusOK, map[string]any{"job": job})
}

func (s *Server) cancelJob(w http.ResponseWriter, r *http.Request, jobID int64) {
	var req cancelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	if _, err := queue.ParseLifecycleOperationID(string(req.OperationID)); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	job, err := s.repo.CancelJob(r.Context(), queue.CancelJobCommand{
		OperationID: req.OperationID, JobID: jobID, Reason: req.Reason,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, cancelJobHTTPStatus(err), err.Error())
		return
	}
	if err := validateSameJobAuthority(jobID, job); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	coalescer, coalescerErr := s.ensureJobOutputCoalescer()
	if coalescerErr != nil {
		log.Printf("job cancel output flush rejected job=%d: %v", jobID, coalescerErr)
	} else {
		coalescer.FlushNow(jobID)
	}
	s.publishJobProgress(jobID, realtimeJobFinished, "Job canceled")

	writeJSON(w, http.StatusOK, map[string]any{
		"job": job,
	})
}

func cancelJobHTTPStatus(err error) int {
	if errors.Is(err, queue.ErrLifecycleOperationConflict) || errors.Is(err, queue.ErrStepNotWritable) {
		return http.StatusConflict
	}
	return http.StatusBadRequest
}
