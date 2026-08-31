package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func (s *Server) handleJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		s.enqueueJob(w, r)
	case http.MethodGet:
		s.listJobs(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) enqueueJob(w http.ResponseWriter, r *http.Request) {
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	request, err := decodeGenericCodingEnqueue(w, r)
	if err != nil {
		writeError(w, genericCodingEnqueueStatus(err), err.Error())
		return
	}
	job, err := s.repo.EnqueueCodingJob(
		r.Context(), request.Instruction, request.Metadata.ClientCWD,
	)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, queue.ErrInvalidJobInstruction) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	s.publishJobProgress(job.ID, realtimeJobQueued, "Job queued")
	writeJSON(w, http.StatusCreated, map[string]any{"job": job})
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	query, err := decodeJobCollectionQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	jobs, err := s.repo.ListJobs(r.Context(), query.Status, query.Limit, query.Offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}

type jobCollectionQuery struct {
	Status string
	Limit  int
	Offset int
}

func decodeJobCollectionQuery(r *http.Request) (jobCollectionQuery, error) {
	if err := validateExactQuery(r, "status", "limit", "offset"); err != nil {
		return jobCollectionQuery{}, err
	}
	limit, err := exactChannelQueryInteger(r, "limit", 20, 1, queue.MaxJobHistoryPageSize)
	if err != nil {
		return jobCollectionQuery{}, err
	}
	offset, err := exactChannelQueryInteger(r, "offset", 0, 0, 1_000_000)
	if err != nil {
		return jobCollectionQuery{}, err
	}
	status := r.URL.Query().Get("status")
	switch status {
	case "", model.JobStatusPending, model.JobStatusRunning, model.JobStatusWaiting,
		model.JobStatusCompleted, model.JobStatusFailed, model.JobStatusCanceled:
	default:
		return jobCollectionQuery{}, fmt.Errorf("job collection status %q is not registered", status)
	}
	return jobCollectionQuery{Status: status, Limit: limit, Offset: offset}, nil
}
