package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

type llmResponseContract struct {
	Protocol            llm.ExactPreparedProtocol
	Format              string
	MaxTokens           int
	OutputLimitMode     llm.ExactPreparedOutputLimitMode
	PromptHint          string
	RawTextStopSequence string
}

func llmResponseContractForScope(scope string) (llmResponseContract, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return llmResponseContract{}, fmt.Errorf("LLM scope is required")
	}
	if scope == "portable_fragment_worker" {
		return llmResponseContract{
			Protocol:            llm.ExactPreparedProtocolRawTextV1,
			MaxTokens:           4096,
			OutputLimitMode:     llm.ExactPreparedOutputLimitNatural,
			PromptHint:          llm.MinimalGeneratePrompt,
			RawTextStopSequence: llm.ExactPreparedCodeStopV1,
		}, nil
	}
	if scope == "portable_semantic_worker" {
		return llmResponseContract{
			Protocol:        llm.ExactPreparedProtocolStructuredV1,
			Format:          llm.ResponseFormatJSON,
			MaxTokens:       1024,
			OutputLimitMode: llm.ExactPreparedOutputLimitExplicit,
			PromptHint:      llm.MinimalGeneratePrompt,
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
	if job.Kind == assemblyline.WorkFragmentCorrection &&
		(responseSchema != nil || contract.Protocol != llm.ExactPreparedProtocolRawTextV1) {
		return llmResponseContract{}, fmt.Errorf("fragment correction requires the raw-text response contract")
	}
	return contract, nil
}
