package llm

import (
	"context"
	"strings"
)

const MinimalGeneratePrompt = "Return only the requested output."

type PreparedModel struct {
	BaseModel     string
	ContextModel  string
	ModelfilePath string
	PromptHint    string
	Prompt        string
}

type Client interface {
	Generate(ctx context.Context, model, prompt string) (string, error)
	PrepareContextModel(ctx context.Context, model, prompt string) (PreparedModel, error)
	GeneratePrepared(ctx context.Context, prepared PreparedModel) (string, error)
	CleanupPreparedModel(prepared PreparedModel)
	Embedding(ctx context.Context, content string) ([]float64, error)
	SuggestTags(ctx context.Context, content string, maxTags int) ([]string, error)
	SuggestTagsWithModel(ctx context.Context, model, content string, maxTags int) ([]string, error)
}

func DerivePreparedModelPromptHint(fullPrompt string) string {
	fullPrompt = strings.TrimSpace(fullPrompt)
	if fullPrompt == "" {
		return MinimalGeneratePrompt
	}
	for _, block := range []string{
		"AUTHORITATIVE_USER_INSTRUCTION_END",
		"AUTHORITATIVE_USER_INSTRUCTION_START",
		"USER_INSTRUCTION",
		"BLOCKING_QUESTION",
	} {
		if value := ExtractPromptBlock(fullPrompt, block); value != "" && value != "(empty)" {
			return TruncatePromptHint("User request: "+value, 700)
		}
	}
	for _, block := range []string{
		"AUTHORITATIVE_USER_FEEDBACK_END",
		"AUTHORITATIVE_USER_FEEDBACK_START",
		"USER_FEEDBACK",
	} {
		if value := ExtractPromptBlock(fullPrompt, block); value != "" && value != "(empty)" {
			return TruncatePromptHint("User feedback: "+value, 500)
		}
	}
	return MinimalGeneratePrompt
}

func ExtractPromptBlock(fullPrompt string, blockName string) string {
	blockName = strings.TrimSpace(blockName)
	if blockName == "" {
		return ""
	}
	startTag := "<" + blockName + ">"
	endTag := "</" + blockName + ">"
	start := strings.Index(fullPrompt, startTag)
	if start < 0 {
		return ""
	}
	start += len(startTag)
	end := strings.Index(fullPrompt[start:], endTag)
	if end < 0 {
		return ""
	}
	value := strings.TrimSpace(fullPrompt[start : start+end])
	if value == "" {
		return ""
	}
	return value
}

func TruncatePromptHint(value string, maxChars int) string {
	value = strings.TrimSpace(value)
	if maxChars <= 0 || len(value) <= maxChars {
		return value
	}
	return strings.TrimSpace(value[:maxChars]) + " ..."
}

func ParseSuggestedTags(result, fallbackContent string, maxTags int) []string {
	if maxTags <= 0 {
		maxTags = 8
	}

	parts := strings.Split(result, ",")
	tags := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, raw := range parts {
		tag := strings.ToLower(strings.TrimSpace(raw))
		tag = strings.Trim(tag, "\"'`[](){}")
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
		if len(tags) >= maxTags {
			break
		}
	}

	if len(tags) > 0 {
		return tags
	}

	for _, token := range strings.Fields(strings.ToLower(fallbackContent)) {
		token = strings.Trim(token, ",.!?:;\"'`()[]{}")
		if len(token) < 4 {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		tags = append(tags, token)
		if len(tags) >= maxTags {
			break
		}
	}

	return tags
}
