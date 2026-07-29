package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/llm"
)

type llmResponseContract struct {
	Format     string
	MaxTokens  int
	PromptHint string
}

func llmResponseContractForScope(scope string) (llmResponseContract, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return llmResponseContract{}, fmt.Errorf("LLM scope is required")
	}
	if scope == "portable_fragment_worker" {
		return llmResponseContract{
			MaxTokens:  4096,
			PromptHint: "Return only the raw TypeScript function required by the supplied contract. No JSON, Markdown, import, export, path, or commentary.",
		}, nil
	}
	if scope == "portable_semantic_worker" {
		return llmResponseContract{
			Format:     llm.ResponseFormatJSON,
			MaxTokens:  1024,
			PromptHint: "Return only one JSON object that satisfies the supplied response contract.",
		}, nil
	}

	maxTokens := 0
	switch {
	case strings.HasPrefix(scope, "v3_subtask_tool_"):
		maxTokens = 4096
	case strings.HasPrefix(scope, "v3_intent_parse"),
		strings.HasPrefix(scope, "v3_planning"),
		strings.HasPrefix(scope, "v3_independent_verification"),
		strings.HasPrefix(scope, "v3_verification"):
		maxTokens = 2048
	case strings.HasPrefix(scope, "v3_analysis"),
		strings.HasPrefix(scope, "v3_response_draft"):
		maxTokens = 1024
	default:
		return llmResponseContract{}, fmt.Errorf("LLM scope %q is not registered", scope)
	}
	return llmResponseContract{
		Format:     llm.ResponseFormatJSON,
		MaxTokens:  maxTokens,
		PromptHint: "Return only one JSON object that satisfies the supplied response contract.",
	}, nil
}
