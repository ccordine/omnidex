package scrumcardllm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"
)

type LLMClient interface {
	Generate(ctx context.Context, model, prompt string) (string, error)
}

type TagsSuggestResult struct {
	Suggested []string
	Notes     string
}

func RunTagsSuggest(ctx context.Context, client LLMClient, modelName, system, user string) (TagsSuggestResult, error) {
	if err := validateLLMRequest(modelName, system, user); err != nil {
		return TagsSuggestResult{}, err
	}
	ctx, cancel, err := TicketLLMContext(ctx)
	if err != nil {
		return TagsSuggestResult{}, err
	}
	defer cancel()
	raw, err := chatOrGenerate(ctx, client, strings.TrimSpace(modelName), system, user)
	if err != nil {
		return TagsSuggestResult{}, err
	}
	return ParseTagsSuggestResponse(raw)
}

func RunCardTicket(ctx context.Context, client LLMClient, modelName, system, user string) (string, error) {
	if err := validateLLMRequest(modelName, system, user); err != nil {
		return "", err
	}
	ctx, cancel, err := TicketLLMContext(ctx)
	if err != nil {
		return "", err
	}
	defer cancel()
	generated, err := chatOrGenerate(ctx, client, strings.TrimSpace(modelName), system, user)
	if err != nil {
		return "", err
	}
	generated = strings.TrimSpace(generated)
	if generated == "" {
		return "", fmt.Errorf("card ticket model returned an empty response")
	}
	return generated, nil
}

func ParseTagsSuggestResponse(raw string) (TagsSuggestResult, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return TagsSuggestResult{}, fmt.Errorf("tag suggestion model returned an empty response")
	}
	var payload struct {
		Tags  []string `json:"tags"`
		Notes string   `json:"notes"`
	}
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&payload); err != nil {
		return TagsSuggestResult{}, fmt.Errorf("decode tag suggestion response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return TagsSuggestResult{}, fmt.Errorf("tag suggestion response contains trailing JSON")
		}
		return TagsSuggestResult{}, fmt.Errorf("tag suggestion response contains trailing data: %w", err)
	}
	if len(payload.Tags) > 12 {
		return TagsSuggestResult{}, fmt.Errorf("tag suggestion response exceeds the 12-tag limit")
	}
	seen := make(map[string]struct{}, len(payload.Tags))
	tags := make([]string, 0, len(payload.Tags))
	for index, tag := range payload.Tags {
		tag = strings.ToLower(strings.TrimSpace(tag))
		if err := validateSuggestedTag(tag); err != nil {
			return TagsSuggestResult{}, fmt.Errorf("tag %d: %w", index, err)
		}
		if _, exists := seen[tag]; exists {
			return TagsSuggestResult{}, fmt.Errorf("tag %d duplicates %q", index, tag)
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	notes := strings.TrimSpace(payload.Notes)
	if len(notes) > 1_000 {
		return TagsSuggestResult{}, fmt.Errorf("tag suggestion notes exceed 1000 characters")
	}
	if len(tags) == 0 && notes == "" {
		return TagsSuggestResult{}, fmt.Errorf("tag suggestion response contains no tags or notes")
	}
	return TagsSuggestResult{Suggested: tags, Notes: notes}, nil
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

func PromptCharCount(system, user string) int {
	return len(strings.TrimSpace(system + "\n\n" + user))
}

func validateLLMRequest(modelName, system, user string) error {
	if strings.TrimSpace(modelName) == "" {
		return fmt.Errorf("model name is required")
	}
	if strings.TrimSpace(system) == "" {
		return fmt.Errorf("system prompt is required")
	}
	if strings.TrimSpace(user) == "" {
		return fmt.Errorf("user prompt is required")
	}
	return nil
}

func validateSuggestedTag(tag string) error {
	if tag == "" {
		return fmt.Errorf("value is required")
	}
	if len(tag) > 64 {
		return fmt.Errorf("value exceeds 64 characters")
	}
	for _, char := range tag {
		if unicode.IsLetter(char) || unicode.IsDigit(char) || strings.ContainsRune(" -_./", char) {
			continue
		}
		return fmt.Errorf("value %q contains unsupported character %q", tag, char)
	}
	return nil
}
