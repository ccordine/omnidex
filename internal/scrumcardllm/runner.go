package scrumcardllm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type LLMClient interface {
	Generate(ctx context.Context, model, prompt string) (string, error)
}

type TagsSuggestResult struct {
	Suggested []string
	Notes     string
}

func CoachModelName(raw string, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		return raw
	}
	return strings.TrimSpace(fallback)
}

func TicketModelName(raw string, fallback string) string {
	raw = strings.TrimSpace(raw)
	if raw != "" {
		return raw
	}
	return strings.TrimSpace(fallback)
}

func RunTagsSuggest(ctx context.Context, client LLMClient, modelName, system, user string) (TagsSuggestResult, error) {
	if client == nil {
		return TagsSuggestResult{}, fmt.Errorf("no llm client configured")
	}
	modelName = CoachModelName(modelName, "qwen3:4b-thinking")
	prompt := strings.TrimSpace(system + "\n\n" + user)
	raw, err := client.Generate(ctx, modelName, prompt)
	if err != nil {
		return TagsSuggestResult{}, err
	}
	return ParseTagsSuggestResponse(raw), nil
}

func RunCardTicket(ctx context.Context, client LLMClient, modelName, system, user string) (string, error) {
	if client == nil {
		return "", fmt.Errorf("no llm client configured")
	}
	modelName = TicketModelName(modelName, "llama3.2")
	prompt := strings.TrimSpace(system + "\n\n" + user)
	generated, err := client.Generate(ctx, modelName, prompt)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(generated), nil
}

func ParseTagsSuggestResponse(raw string) TagsSuggestResult {
	out := TagsSuggestResult{}
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start < 0 || end <= start {
		return out
	}
	var payload struct {
		Tags  []string `json:"tags"`
		Notes string   `json:"notes"`
	}
	if err := json.Unmarshal([]byte(raw[start:end+1]), &payload); err != nil {
		return out
	}
	out.Suggested = payload.Tags
	out.Notes = payload.Notes
	return out
}

func MergeTags(existing []string, sets ...[]string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0)
	add := func(items []string) {
		for _, item := range items {
			item = strings.TrimSpace(strings.ToLower(item))
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	add(existing)
	for _, set := range sets {
		add(set)
	}
	return out
}

func ParseCoachModel(raw json.RawMessage, fallback string) string {
	if len(raw) == 0 {
		return CoachModelName("", fallback)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return CoachModelName("", fallback)
	}
	if v, ok := payload["model"].(string); ok {
		return CoachModelName(v, fallback)
	}
	return CoachModelName("", fallback)
}

func PromptCharCount(system, user string) int {
	return len(strings.TrimSpace(system + "\n\n" + user))
}
