package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/research"
	"github.com/jackc/pgx/v5"
)

func (s *Server) replanJob(w http.ResponseWriter, r *http.Request, jobID int64) {
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

	job, err := s.repo.ReplanJob(r.Context(), jobID, req.Feedback)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	response := map[string]any{"job": job}
	if job.ID != jobID {
		response["superseded_job_id"] = jobID
		s.publishJobProgress(jobID, realtimeJobFinished, "Job superseded by revised user authority")
		s.publishJobProgress(job.ID, realtimeJobQueued, "Revised job queued from replanning feedback")
	} else {
		s.publishJobProgress(jobID, realtimeJobChanged, "Job replanned")
	}

	writeJSON(w, http.StatusOK, response)
}

func (s *Server) handleMemory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.listMemory(w, r)
	case http.MethodPost:
		s.addMemory(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleMemoryCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobID, _ := strconv.ParseInt(strings.TrimSpace(r.URL.Query().Get("job_id")), 10, 64)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := parseInt(r.URL.Query().Get("limit"), 50)
	items, err := s.repo.ListMemoryCandidates(r.Context(), jobID, status, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"memory_candidates": items,
	})
}

func (s *Server) handleMemoryCandidateByID(w http.ResponseWriter, r *http.Request) {
	idText := strings.TrimPrefix(r.URL.Path, "/v1/memory-candidates/")
	idText = strings.TrimSpace(strings.Trim(idText, "/"))
	if idText == "" {
		writeError(w, http.StatusBadRequest, "candidate id is required")
		return
	}

	if strings.HasSuffix(idText, "/promote") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/promote")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid candidate id")
			return
		}
		s.promoteMemoryCandidate(w, r, id)
		return
	}

	if strings.HasSuffix(idText, "/reject") {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/reject")
		idText = strings.TrimSpace(strings.Trim(idText, "/"))
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid candidate id")
			return
		}
		s.rejectMemoryCandidate(w, r, id)
		return
	}

	if r.Method == http.MethodDelete {
		id, err := strconv.ParseInt(idText, 10, 64)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid candidate id")
			return
		}
		if err := s.repo.DeleteMemoryCandidate(r.Context(), id); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusNotFound, "memory candidate not found")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": id})
		return
	}

	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid candidate id")
		return
	}
	item, err := s.repo.GetMemoryCandidate(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "memory candidate not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"memory_candidate": item,
	})
}

func (s *Server) addMemory(w http.ResponseWriter, r *http.Request) {
	var req memoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	req.Content = strings.TrimSpace(req.Content)
	if req.Content == "" {
		writeError(w, http.StatusBadRequest, "content is required")
		return
	}

	embedding, err := s.requireMemoryEmbedding(r.Context(), req.Content)
	if err != nil {
		log.Printf("memory ingest rejected source=%q kind=%q content_bytes=%d: %v", req.Source, req.Kind, len(req.Content), err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}

	chunk, err := s.repo.AddMemoryChunk(r.Context(), req.Source, req.Kind, req.Content, req.Tags, embedding)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"memory": chunk,
	})
}

func (s *Server) requireMemoryEmbedding(ctx context.Context, content string) ([]float64, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("memory embedding client is not configured")
	}
	embedding, err := s.llmClient.Embedding(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("generate memory embedding: %w", err)
	}
	if len(embedding) == 0 {
		return nil, fmt.Errorf("generate memory embedding: provider returned an empty vector")
	}
	return embedding, nil
}

func (s *Server) handleMemoryCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 100)
	facets, err := s.repo.ListMemoryCategories(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"categories": facets})
}

func (s *Server) handleMemoryTags(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := parsePositiveInt(r.URL.Query().Get("limit"), 100)
	facets, err := s.repo.ListMemoryTags(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": facets})
}

func (s *Server) handleResearchIngest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.repo == nil {
		writeError(w, http.StatusServiceUnavailable, "repository is not configured")
		return
	}

	var req researchIngestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	topic := strings.TrimSpace(req.Topic)
	if topic == "" {
		writeError(w, http.StatusBadRequest, "topic is required")
		return
	}

	includeOfficial := true
	if req.IncludeOfficialSources != nil {
		includeOfficial = *req.IncludeOfficialSources
	}
	sourcePrefix := strings.TrimSpace(req.Source)
	if sourcePrefix == "" {
		sourcePrefix = research.DefaultSourcePrefix
	}
	kind := strings.TrimSpace(req.Kind)
	if kind == "" {
		kind = model.MemoryKindReference
	}
	slug := research.SanitizeToken(topic)
	if slug == "" {
		slug = fmt.Sprintf("topic-%d", time.Now().Unix())
	}

	documents := []research.Document{}
	warnings := []string{}
	if includeOfficial {
		fetched, fetchWarnings, err := research.FetchOfficialDocuments(r.Context(), topic)
		warnings = append(warnings, fetchWarnings...)
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		documents = append(documents, fetched...)
	}
	if len(documents) == 0 {
		writeError(w, http.StatusBadRequest, "no research documents found for topic")
		return
	}

	prepared := research.PrepareChunks(documents, research.PrepareOptions{
		Topic:        topic,
		Slug:         slug,
		SourcePrefix: sourcePrefix,
		Tags:         req.Tags,
		ChunkSize:    req.ChunkSize,
		Overlap:      req.Overlap,
		MaxChunks:    req.MaxChunks,
	})
	if len(prepared) == 0 {
		writeError(w, http.StatusBadRequest, "no ingestible research chunks produced")
		return
	}
	embeddings := make([][]float64, len(prepared))
	for index, chunk := range prepared {
		embedding, err := s.requireMemoryEmbedding(r.Context(), chunk.Content)
		if err != nil {
			log.Printf("research ingest rejected topic=%q chunk=%d source=%q content_bytes=%d: %v", topic, index, chunk.Source, len(chunk.Content), err)
			writeError(w, http.StatusBadGateway, fmt.Sprintf("embed research chunk %d: %v", index, err))
			return
		}
		embeddings[index] = embedding
	}

	storedSources := make([]string, 0, len(prepared))
	var storedTags []string
	for index, chunk := range prepared {
		if _, err := s.repo.AddMemoryChunk(r.Context(), chunk.Source, kind, chunk.Content, chunk.Tags, embeddings[index]); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		storedSources = append(storedSources, chunk.Source)
		if len(storedTags) == 0 {
			storedTags = chunk.Tags
		}
	}

	sources := make([]string, 0, len(documents))
	for _, doc := range documents {
		if url := research.DocumentURL(doc.Content); url != "" {
			sources = append(sources, url)
		}
	}

	writeJSON(w, http.StatusCreated, researchIngestResponse{
		Topic:             topic,
		Slug:              slug,
		SourcePrefix:      sourcePrefix,
		StoredChunks:      len(storedSources),
		Tags:              storedTags,
		Warnings:          warnings,
		Dossier:           research.BuildDossier(topic, 0, time.Now(), documents, storedTags, sourcePrefix, len(storedSources)),
		Sources:           sources,
		StoredChunkSource: storedSources,
	})
}

func (s *Server) promoteMemoryCandidate(w http.ResponseWriter, r *http.Request, candidateID int64) {
	var req memoryCandidatePromotionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}
	tier := strings.ToLower(strings.TrimSpace(req.Tier))
	if tier == "" {
		tier = model.MemoryCandidateStatusApproved
	}
	if tier != model.MemoryCandidateStatusApproved && tier != model.MemoryCandidateStatusDurable {
		writeError(w, http.StatusBadRequest, "tier must be approved or durable")
		return
	}

	candidate, err := s.repo.GetMemoryCandidate(r.Context(), candidateID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "memory candidate not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	embed, err := s.llmClient.Embedding(r.Context(), candidate.Content)
	if err != nil {
		embed = nil
	}
	tags := append(memoryCandidateScopeTags(candidate), candidate.CandidateKind)
	source := fmt.Sprintf("job:%d:reviewed:%s", candidate.JobID, tier)
	chunk, err := s.repo.AddMemoryChunk(r.Context(), source, candidate.CandidateKind, candidate.Content, tags, embed)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.repo.UpdateMemoryCandidateStatus(r.Context(), candidate.ID, tier); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated, err := s.repo.GetMemoryCandidate(r.Context(), candidate.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, model.MemoryCandidatePromotionResult{
		Candidate: updated,
		Memory:    &chunk,
	})
}

func (s *Server) rejectMemoryCandidate(w http.ResponseWriter, r *http.Request, candidateID int64) {
	item, err := s.repo.GetMemoryCandidate(r.Context(), candidateID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "memory candidate not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.repo.UpdateMemoryCandidateStatus(r.Context(), candidateID, model.MemoryCandidateStatusRejected); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	item.Status = model.MemoryCandidateStatusRejected
	writeJSON(w, http.StatusOK, map[string]any{
		"memory_candidate": item,
	})
}

func memoryCandidateScopeTags(candidate model.MemoryCandidate) []string {
	if len(candidate.Provenance) == 0 || !json.Valid(candidate.Provenance) {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(candidate.Provenance, &payload); err != nil {
		return nil
	}
	raw, ok := payload["scope_tags"]
	if !ok {
		return nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	tags := make([]string, 0, len(items))
	for _, item := range items {
		tag := strings.TrimSpace(fmt.Sprintf("%v", item))
		if tag == "" {
			continue
		}
		tags = append(tags, tag)
	}
	return tags
}

func (s *Server) handleAdminMigrateFresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.repo.MigrateFresh(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": "ok",
	})
}

func parseInt(v string, fallback int) int {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return parsed
}

func parsePositiveInt(v string, fallback int) int {
	parsed := parseInt(v, fallback)
	if parsed <= 0 {
		return fallback
	}
	return parsed
}

func writeError(w http.ResponseWriter, code int, message string) {
	writeJSON(w, code, map[string]any{
		"error": message,
	})
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func Run(ctx context.Context, addr string, handler http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		IdleTimeout:       30 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http server: %w", err)
	}
}
