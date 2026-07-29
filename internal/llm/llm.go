package llm

import (
	"context"
	"fmt"
	"strings"
)

const MinimalGeneratePrompt = "Return only the requested output."

const ResponseFormatJSON = "json"

type PreparedModel struct {
	BaseModel       string
	ContextModel    string
	ModelfilePath   string
	PromptHint      string
	Prompt          string
	MaxOutputTokens int
	ContextTokens   int
	ResponseFormat  string
	ResponseSchema  map[string]any
}

func ValidateResponseContract(prepared PreparedModel) error {
	if prepared.ResponseFormat != "" && prepared.ResponseFormat != ResponseFormatJSON {
		return fmt.Errorf("unsupported response format %q", prepared.ResponseFormat)
	}
	if len(prepared.ResponseSchema) == 0 {
		return nil
	}
	if prepared.ResponseFormat != ResponseFormatJSON {
		return fmt.Errorf("response schema requires response format %q", ResponseFormatJSON)
	}
	schemaType, ok := prepared.ResponseSchema["type"].(string)
	if !ok || strings.TrimSpace(schemaType) == "" {
		return fmt.Errorf("response schema requires a non-empty type")
	}
	return nil
}

type Client interface {
	Generate(ctx context.Context, model, prompt string) (string, error)
	PrepareContextModel(ctx context.Context, model, prompt string) (PreparedModel, error)
	GeneratePrepared(ctx context.Context, prepared PreparedModel) (string, error)
	CleanupPreparedModel(prepared PreparedModel)
	Embedding(ctx context.Context, content string) ([]float64, error)
}

type GenerationProgress struct {
	OutputBytes int
}

type PreparedStreamingClient interface {
	GeneratePreparedStream(ctx context.Context, prepared PreparedModel, observe func(GenerationProgress) error) (string, error)
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
