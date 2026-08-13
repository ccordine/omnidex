package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

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
	var request enqueueRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	instruction, err := requireFreeFormAuthority(request.Instruction, "instruction")
	if err != nil {
		writeError(w, http.StatusBadRequest, "instruction is required")
		return
	}
	if err := validateGenericJobPipeline(request.Pipeline); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(request.Metadata) == 0 {
		request.Metadata = []byte(`{}`)
	}
	enriched, _, err := s.enrichJobMetadata(r.Context(), request.Metadata, ScrumCard{})
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("model setup failed: %v", err))
		return
	}
	job, err := s.repo.EnqueueJob(r.Context(), instruction, request.Pipeline, enriched)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, queue.ErrUnsupportedPipeline) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	s.publishJobProgress(job.ID, realtimeJobQueued, "Job queued")
	writeJSON(w, http.StatusCreated, map[string]any{"job": job})
}

func validateGenericJobPipeline(pipeline string) error {
	switch pipeline {
	case model.PipelineCoding, model.PipelineScrum:
		return nil
	case model.PipelineAssistant, model.PipelineChat, model.PipelineStory:
		return fmt.Errorf(
			"pipeline %q requires the server-authoritative /v1/channels/{id}/messages transport",
			pipeline,
		)
	default:
		return fmt.Errorf("generic job pipeline %q is unsupported; expected coding or scrum", pipeline)
	}
}

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := parseInt(r.URL.Query().Get("limit"), 20)
	offset := parseInt(r.URL.Query().Get("offset"), 0)
	jobs, err := s.repo.ListJobs(r.Context(), status, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"jobs": jobs})
}
