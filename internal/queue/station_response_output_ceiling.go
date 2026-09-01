package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

const minSingleLineCompletionTokens = 8

// ExpectedPortableStationMaxOutputTokens leaves every ordinary station response
// at the provider's native limit. Code validates the one completed response;
// it never truncates or regenerates a semantic result to manage output tokens.
func ExpectedPortableStationMaxOutputTokens(
	job assemblyline.PortableJob,
	contextTokens int,
) (int, error) {
	if err := llm.ValidateExactPreparedContextTokens(contextTokens); err != nil {
		return 0, err
	}
	if err := job.Validate(); err != nil {
		return 0, err
	}
	return -1, nil
}

// ExpectedSourceBodyCorrectionMaxOutputTokens gives an exact opaque choice
// only enough room for its complete ID and line termination. An ordinary
// exact-span replacement uses Ollama's native unlimited generation.
func ExpectedSourceBodyCorrectionMaxOutputTokens(
	opaqueResponseBytes int,
	opaque bool,
	contextTokens int,
) (int, error) {
	if err := llm.ValidateExactPreparedContextTokens(contextTokens); err != nil {
		return 0, err
	}
	if opaque {
		if opaqueResponseBytes < 1 {
			return 0, fmt.Errorf("opaque source correction requires a positive response bound")
		}
		budget := opaqueResponseBytes + 1
		if budget < minSingleLineCompletionTokens {
			budget = minSingleLineCompletionTokens
		}
		return boundedProviderOutputTokens(budget, contextTokens)
	}
	if opaqueResponseBytes != 0 {
		return 0, fmt.Errorf("open source correction cannot carry an opaque response bound")
	}
	return -1, nil
}

func boundedProviderOutputTokens(budget, contextTokens int) (int, error) {
	if budget < 1 {
		return 0, fmt.Errorf("provider output budget must be positive")
	}
	if budget >= contextTokens {
		budget = contextTokens - 1
	}
	if budget < 1 {
		return 0, fmt.Errorf("provider output budget leaves no native input authority")
	}
	return budget, nil
}
