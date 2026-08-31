package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

type llmResponseContract struct {
	Protocol        llm.ExactPreparedProtocol
	OutputLimitMode llm.ExactPreparedOutputLimitMode
	PromptHint      string
	ResponseFraming assemblyline.PortableResponseFraming
}

func llmResponseContractForScope(scope string) (llmResponseContract, error) {
	scope = strings.ToLower(strings.TrimSpace(scope))
	if scope == "" {
		return llmResponseContract{}, fmt.Errorf("LLM scope is required")
	}
	if scope == assemblyline.PortableFragmentWorkerScope {
		return llmResponseContract{
			Protocol:        llm.ExactPreparedProtocolRawTextV2,
			OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
			PromptHint:      llm.MinimalGeneratePrompt,
		}, nil
	}
	if scope == assemblyline.PortableSemanticWorkerScope {
		return llmResponseContract{
			Protocol:        llm.ExactPreparedProtocolRawTextV2,
			OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
			PromptHint:      llm.MinimalGeneratePrompt,
		}, nil
	}
	return llmResponseContract{}, fmt.Errorf("LLM scope %q is not registered", scope)
}

func llmResponseContractForPortableJob(job assemblyline.PortableJob) (llmResponseContract, error) {
	scope, err := portableModelScope(job.Kind)
	if err != nil {
		return llmResponseContract{}, err
	}
	contract, err := llmResponseContractForScope(scope)
	if err != nil {
		return llmResponseContract{}, err
	}
	if contract.Protocol != llm.ExactPreparedProtocolRawTextV2 {
		return llmResponseContract{}, fmt.Errorf("portable work requires the raw-text response contract")
	}
	framing, err := assemblyline.PortableResponseFramingForJob(job)
	if err != nil {
		return llmResponseContract{}, err
	}
	switch framing {
	case assemblyline.PortableResponseFramingSingleLine:
		if scope != assemblyline.PortableSemanticWorkerScope {
			return llmResponseContract{}, fmt.Errorf(
				"single-line portable framing requires the unframed semantic transport",
			)
		}
	case assemblyline.PortableResponseFramingNaturalMultiline:
	default:
		return llmResponseContract{}, fmt.Errorf(
			"portable response framing %q is not provider-actionable", framing,
		)
	}
	contract.ResponseFraming = framing
	return contract, nil
}
