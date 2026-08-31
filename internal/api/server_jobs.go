package api

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

func (s *Server) replanJob(w http.ResponseWriter, r *http.Request, jobID int64) {
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

	result, err := s.repo.ReplanJob(r.Context(), queue.ReplanJobCommand{
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
		s.publishJobProgressForJob(result.Job, realtimeJobChanged, "Job replanned")
	}

	writeJSON(w, http.StatusOK, receipt)
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

func (s *Server) handleMemoryBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	s.addMemoryBatch(w, r)
}

func (s *Server) handleMemoryCandidates(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	jobIDValue, err := exactChannelQueryInteger(r, "job_id", 0, 1, int(^uint(0)>>1))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	jobID := int64(jobIDValue)
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit, err := exactChannelQueryInteger(r, "limit", 50, 1, 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	items, err := s.repo.ListHistoricalMemoryCandidates(r.Context(), jobID, status, limit)
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
	if err := decodeExactMemoryJSON(
		w, r, "memory request", maxMemoryRequestBodyBytes, &req,
	); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	input := req.input()
	if err := input.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	embedding, err := s.requireMemoryEmbedding(r.Context(), input.Content)
	if err != nil {
		log.Printf("memory ingest rejected source=%q kind=%q content_bytes=%d: %v",
			input.Source, input.Kind, len(input.Content), err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	chunks, err := s.repo.AddMemoryChunks(r.Context(), []queue.MemoryChunkWrite{{
		Input: input, Embedding: embedding,
	}})
	if err != nil {
		writeError(w, memoryWriteStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"memory": chunks[0],
	})
}

func (s *Server) addMemoryBatch(w http.ResponseWriter, r *http.Request) {
	var req memoryBatchRequest
	if err := decodeExactMemoryJSON(
		w, r, "memory batch", maxMemoryBatchBodyBytes, &req,
	); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	inputs, err := validateMemoryBatchRequest(req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writes := make([]queue.MemoryChunkWrite, len(inputs))
	for index, input := range inputs {
		embedding, err := s.requireMemoryEmbedding(r.Context(), input.Content)
		if err != nil {
			log.Printf("memory batch embedding rejected item=%d source=%q kind=%q: %v",
				index+1, input.Source, input.Kind, err)
			writeError(w, http.StatusBadGateway, fmt.Sprintf(
				"memory batch item %d: %v", index+1, err,
			))
			return
		}
		writes[index] = queue.MemoryChunkWrite{Input: input, Embedding: embedding}
	}
	chunks, err := s.repo.AddMemoryChunks(r.Context(), writes)
	if err != nil {
		writeError(w, memoryWriteStatus(err), err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"memories": chunks})
}

func memoryWriteStatus(err error) int {
	if errors.Is(err, queue.ErrInvalidMemoryWrite) {
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}

func (s *Server) requireMemoryEmbedding(ctx context.Context, content string) ([]float64, error) {
	if s.embeddingClient == nil {
		return nil, fmt.Errorf("memory embedding client is not configured")
	}
	embedding, err := s.embeddingClient.Embedding(ctx, content)
	if err != nil {
		return nil, fmt.Errorf("generate memory embedding: %w", err)
	}
	if len(embedding) != model.MemoryEmbeddingDimensions {
		return nil, fmt.Errorf(
			"generate memory embedding: provider returned %d values, expected %d",
			len(embedding), model.MemoryEmbeddingDimensions,
		)
	}
	for _, value := range embedding {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("generate memory embedding: provider returned a non-finite value")
		}
	}
	return embedding, nil
}

func (s *Server) handleMemoryCategories(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, err := exactChannelQueryInteger(r, "limit", 100, 1, 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
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
	limit, err := exactChannelQueryInteger(r, "limit", 100, 1, 500)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	facets, err := s.repo.ListMemoryTags(r.Context(), limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tags": facets})
}

func (s *Server) promoteMemoryCandidate(w http.ResponseWriter, r *http.Request, candidateID int64) {
	var req memoryCandidatePromotionRequest
	if err := decodeExactMemoryJSON(
		w, r, "memory candidate promotion", maxMemoryRequestBodyBytes, &req,
	); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	tier := req.Tier
	if tier != model.MemoryCandidateStatusApproved && tier != model.MemoryCandidateStatusDurable {
		writeError(w, http.StatusBadRequest, "tier must be exact approved or durable")
		return
	}
	authority := req.Authority
	if authority != model.MemoryPromotionAuthorityCurrent &&
		authority != model.MemoryPromotionAuthorityHistorical &&
		authority != model.MemoryPromotionAuthorityGlobal {
		writeError(w, http.StatusBadRequest, "authority must be current_generation, historical_generation, or global")
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
	if authority == model.MemoryPromotionAuthorityGlobal && (candidate.JobID != 0 || candidate.Generation != nil) {
		writeError(w, http.StatusConflict, "global authority requires a global memory candidate")
		return
	}
	if authority != model.MemoryPromotionAuthorityGlobal && (candidate.JobID <= 0 || candidate.Generation == nil) {
		writeError(w, http.StatusConflict, "job generation authority requires a job-scoped memory candidate")
		return
	}

	tags, categories, err := memoryCandidatePromotionMetadata(candidate)
	if err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	embed, err := s.requireMemoryEmbedding(r.Context(), candidate.Content)
	if err != nil {
		log.Printf("memory promotion rejected candidate_id=%d authority=%s: %v", candidate.ID, authority, err)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	promotion := queue.MemoryCandidatePromotion{
		Candidate:  candidate,
		Tier:       tier,
		Tags:       tags,
		Categories: categories,
		Embedding:  embed,
	}
	var result model.MemoryCandidatePromotionResult
	switch authority {
	case model.MemoryPromotionAuthorityCurrent:
		result, err = s.repo.PromoteCurrentMemoryCandidate(r.Context(), promotion)
	case model.MemoryPromotionAuthorityHistorical:
		result, err = s.repo.PromoteHistoricalMemoryCandidate(r.Context(), promotion)
	case model.MemoryPromotionAuthorityGlobal:
		result, err = s.repo.PromoteGlobalMemoryCandidate(r.Context(), promotion)
	}
	if err != nil {
		if errors.Is(err, queue.ErrStaleJobGeneration) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
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
	if err := s.repo.RejectCurrentMemoryCandidate(r.Context(), item); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	item.Status = model.MemoryCandidateStatusRejected
	writeJSON(w, http.StatusOK, map[string]any{
		"memory_candidate": item,
	})
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
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown http server: %w", err)
		}
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("http server: %w", err)
	}
}
