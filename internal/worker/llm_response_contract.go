package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

const (
	maxFragmentCorrectionOutputTokens       = 2048
	maxFragmentRegionCorrectionOutputTokens = 1024
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

func llmResponseContractForPortableJob(
	job assemblyline.PortableJob,
	responseSchema map[string]any,
) (llmResponseContract, error) {
	contract, err := llmResponseContractForScope(portableModelScope(responseSchema))
	if err != nil {
		return llmResponseContract{}, err
	}
	if job.Kind != assemblyline.WorkFragmentCorrection {
		return contract, nil
	}
	var input assemblyline.FragmentCorrectionInput
	if err := json.Unmarshal(job.Payload, &input); err != nil {
		return llmResponseContract{}, fmt.Errorf("decode fragment correction response contract: %w", err)
	}
	if input.Language == "typescript" {
		if input.RepairRegion != nil {
			if responseSchema == nil || contract.Protocol != llm.ExactPreparedProtocolStructuredV1 ||
				contract.Format != llm.ResponseFormatJSON {
				return llmResponseContract{}, fmt.Errorf("localized TypeScript fragment correction requires the structured JSON response contract")
			}
			contract.MaxTokens = maxFragmentRegionCorrectionOutputTokens
		} else {
			if responseSchema != nil || contract.Protocol != llm.ExactPreparedProtocolRawTextV1 {
				return llmResponseContract{}, fmt.Errorf("whole TypeScript fragment correction requires the raw-text response contract")
			}
			contract.MaxTokens = maxFragmentCorrectionOutputTokens
		}
	} else if responseSchema != nil || contract.Protocol != llm.ExactPreparedProtocolRawTextV1 {
		return llmResponseContract{}, fmt.Errorf("fragment correction requires the raw-text response contract")
	}
	return contract, nil
}
