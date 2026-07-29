package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/jackc/pgx/v5"
)

func (s *Server) runPersona(ctx context.Context, persona string, req personaRequest, extraSections map[string]string, client llm.Client) (string, string, error) {
	if client == nil {
		return "", "", fmt.Errorf("llm client is not configured")
	}

	systemPrompt := buildPersonaSystemPrompt(persona, req, extraSections)
	prepared, err := client.PrepareContextModel(ctx, strings.TrimSpace(req.Model), systemPrompt)
	if err != nil {
		return "", "", err
	}
	defer client.CleanupPreparedModel(prepared)

	prepared.PromptHint = req.Prompt
	output, err := client.GeneratePrepared(ctx, prepared)
	if err != nil {
		return "", "", err
	}

	return strings.TrimSpace(output), strings.TrimSpace(prepared.BaseModel), nil
}

func buildPersonaSystemPrompt(persona string, req personaRequest, extraSections map[string]string) string {
	sections := []string{
		"PERSONA",
		personaDirective(persona),
		"",
		"REQUEST_SYSTEM_CONTEXT",
		nonEmptyOrPlaceholder(req.System),
		"",
		"REQUEST_HISTORY",
		formatPersonaHistory(req.History),
		"",
		"REQUEST_CONTEXT_JSON",
		formatPersonaJSON(req.Context),
		"",
	}

	if len(extraSections) > 0 {
		keys := make([]string, 0, len(extraSections))
		for key := range extraSections {
			if strings.TrimSpace(key) == "" {
				continue
			}
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			sections = append(sections, "CHAIN_SECTION: "+strings.TrimSpace(key))
			sections = append(sections, nonEmptyOrPlaceholder(extraSections[key]))
			sections = append(sections, "")
		}
	}

	sections = append(sections, "USER_INSTRUCTION_CHANNEL")
	sections = append(sections, "The caller prompt is supplied separately as runtime user input.")
	return strings.Join(sections, "\n")
}

func personaDirective(persona string) string {
	switch strings.ToLower(strings.TrimSpace(persona)) {
	case "instruct":
		return "Follow USER_INSTRUCTION exactly and directly. Do not add preambles."
	case "roleplay":
		return "Write an in-character response that strictly respects REQUEST_SYSTEM_CONTEXT, REQUEST_HISTORY, and REQUEST_CONTEXT_JSON."
	case "narrate":
		return "Write narrative prose that links actions and scene details from REQUEST_SYSTEM_CONTEXT, REQUEST_HISTORY, and REQUEST_CONTEXT_JSON."
	case "reasoning_parse":
		return "Extract intent, constraints, key facts, and unknowns from USER_INSTRUCTION and context. Return concise structured text."
	case "reasoning_deliberate":
		return "Propose a concise approach and rationale using USER_INSTRUCTION and any CHAIN_SECTION entries. Keep reasoning summary brief."
	case "reasoning_final":
		return "Return the best final answer for USER_INSTRUCTION using context and chain sections. Be direct and actionable."
	default:
		return "Use USER_INSTRUCTION as the primary request and context sections as background constraints."
	}
}

func nonEmptyOrPlaceholder(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "(none)"
	}
	return value
}

func formatPersonaHistory(history []personaMessage) string {
	if len(history) == 0 {
		return "(none)"
	}
	lines := make([]string, 0, len(history))
	for _, msg := range history {
		content := strings.TrimSpace(msg.Content)
		if content == "" {
			continue
		}
		role := strings.ToLower(strings.TrimSpace(msg.Role))
		if role == "" {
			role = "unknown"
		}
		lines = append(lines, fmt.Sprintf("[%s] %s", role, content))
	}
	if len(lines) == 0 {
		return "(none)"
	}
	return strings.Join(lines, "\n")
}

func formatPersonaJSON(raw json.RawMessage) string {
	value := strings.TrimSpace(string(raw))
	if value == "" {
		return "(none)"
	}

	var out bytes.Buffer
	if err := json.Indent(&out, raw, "", "  "); err == nil {
		return strings.TrimSpace(out.String())
	}

	return value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

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
	var req enqueueRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return
	}

	req.Instruction = strings.TrimSpace(req.Instruction)
	if req.Instruction == "" {
		writeError(w, http.StatusBadRequest, "instruction is required")
		return
	}

	if len(req.Metadata) == 0 {
		req.Metadata = []byte(`{}`)
	}
	enriched, _, enrichErr := s.enrichJobMetadata(r.Context(), req.Metadata, ScrumCard{})
	if enrichErr != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("model setup failed: %v", enrichErr))
		return
	}
	req.Metadata = enriched

	job, err := s.repo.EnqueueJob(r.Context(), req.Instruction, req.Pipeline, req.Metadata)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, queue.ErrUnsupportedPipeline) {
			status = http.StatusBadRequest
		}
		writeError(w, status, err.Error())
		return
	}
	s.publishJobProgress(job.ID, realtimeJobQueued, "Job queued")

	writeJSON(w, http.StatusCreated, map[string]any{
		"job": job,
	})
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

func (s *Server) listJobs(w http.ResponseWriter, r *http.Request) {
	status := strings.TrimSpace(r.URL.Query().Get("status"))
	limit := parseInt(r.URL.Query().Get("limit"), 20)
	offset := parseInt(r.URL.Query().Get("offset"), 0)

	jobs, err := s.repo.ListJobs(r.Context(), status, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"jobs": jobs,
	})
}

func (s *Server) handleJobByID(w http.ResponseWriter, r *http.Request) {
	idText := strings.TrimPrefix(r.URL.Path, "/v1/jobs/")
	idText = strings.TrimSpace(strings.Trim(idText, "/"))
	if idText == "" {
		writeError(w, http.StatusBadRequest, "job id is required")
		return
	}

	if strings.HasSuffix(idText, "/inspection") {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		idText = strings.TrimSuffix(idText, "/inspection")
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
		s.inspectJob(w, r, id)
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

	details, err := s.repo.GetJobDetails(r.Context(), id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, details)
}

func (s *Server) inspectJob(w http.ResponseWriter, r *http.Request, jobID int64) {
	limit := parseInt(r.URL.Query().Get("limit"), 200)
	if limit < 1 {
		limit = 200
	}
	inspection, err := s.repo.GetJobInspection(r.Context(), jobID, limit)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, inspection)
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

	job, err := s.repo.SubmitJobFeedback(r.Context(), jobID, req.Feedback)
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

	job, err := s.repo.InterruptJob(r.Context(), jobID, req.Feedback)
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

	job, err := s.repo.CancelJob(r.Context(), jobID, req.Reason)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "job not found")
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
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
