package api

import (
	"context"
	"os"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

const (
	llmContextCharsPerToken         = 24
	llmContextSourceScrumPilot        = "scrum_pilot"
	llmContextSourceScrumCoach        = "scrum_coach"
	llmContextSourceCardTicket        = "scrum_card_ticket"
	llmContextSourceTagsSuggest       = "scrum_tags_suggest"
	llmContextSourceOutcomeClassifier = "scrum_outcome_classifier"
	llmContextSourceScrumGeneric      = "scrum_llm"
	llmContextSourceProjectPlanning   = "project_planning_chat"
)

type llmContextTelemetryMeta struct {
	ProjectID int64
	CardID    string
	Metadata  map[string]any
}

func llmPromptCharCount(system, user string) int {
	system = strings.TrimSpace(system)
	user = strings.TrimSpace(user)
	total := len(system) + len(user)
	if system != "" && user != "" {
		total += 2
	}
	return total
}

func (s *Server) defaultContextLimitChars() int {
	raw := strings.TrimSpace(os.Getenv("OMNI_OLLAMA_NUM_CTX"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("OMNI_PLANNER_NUM_CTX"))
	}
	numCtx := 2048
	if raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			numCtx = parsed
		}
	}
	return numCtx * llmContextCharsPerToken
}

func classifyAPIContextError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	switch {
	case strings.Contains(msg, "context") && (strings.Contains(msg, "length") || strings.Contains(msg, "overflow") || strings.Contains(msg, "exceed")):
		return "context_overflow"
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline"):
		return "timeout"
	case strings.Contains(msg, "connection") || strings.Contains(msg, "eof") || strings.Contains(msg, "unavailable"):
		return "backend_unavailable"
	case strings.Contains(msg, "json") || strings.Contains(msg, "parse") || strings.Contains(msg, "unmarshal"):
		return "malformed_response"
	default:
		return "llm_error"
	}
}

func (s *Server) recordLLMContextUsage(ctx context.Context, source, model, provider string, meta llmContextTelemetryMeta, promptChars, sentChars int, shrunk bool, savedPct float64, callErr error) {
	if s.repo == nil {
		return
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return
	}
	if sentChars <= 0 {
		sentChars = promptChars
	}
	if promptChars <= 0 {
		promptChars = sentChars
	}
	limit := s.defaultContextLimitChars()
	success := callErr == nil
	metadata := meta.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	if callErr != nil {
		metadata["error"] = callErr.Error()
	}
	_ = s.repo.RecordLLMContextUsage(ctx, queue.LLMContextUsageRecord{
		Source:            source,
		Model:             strings.TrimSpace(model),
		Provider:          strings.TrimSpace(provider),
		ProjectID:         meta.ProjectID,
		CardID:            meta.CardID,
		PromptChars:       promptChars,
		SentChars:         sentChars,
		ContextLimitChars: limit,
		Shrunk:            shrunk,
		SavedPct:          savedPct,
		Success:           success,
		ErrorClass:        classifyAPIContextError(callErr),
		Metadata:          metadata,
	})
}

func (s *Server) llmProviderName() string {
	return strings.TrimSpace(s.defaultProvider)
}
