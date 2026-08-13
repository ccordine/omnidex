package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/llm"
)

type llmResponseContract struct {
	Protocol   llm.ExactPreparedProtocol
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
			Protocol:   llm.ExactPreparedProtocolRawTextV1,
			MaxTokens:  4096,
			PromptHint: llm.MinimalGeneratePrompt,
		}, nil
	}
	if scope == "portable_semantic_worker" {
		return llmResponseContract{
			Protocol:   llm.ExactPreparedProtocolStructuredV1,
			Format:     llm.ResponseFormatJSON,
			MaxTokens:  1024,
			PromptHint: llm.MinimalGeneratePrompt,
		}, nil
	}
	return llmResponseContract{}, fmt.Errorf("LLM scope %q is not registered", scope)
}
