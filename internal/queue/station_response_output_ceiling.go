package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

const (
	// These are resource limits, not model response grammars. A provider that
	// reaches one returns incomplete evidence which is rejected before semantic
	// decoding. Ordinary responses end earlier with done_reason=stop.
	maxPortableStationOutputTokens      = 2048
	maxSourceBodyCorrectionOutputTokens = 512
	minSingleLineCompletionTokens       = 8
)

// ExpectedPortableStationMaxOutputTokens uses Ollama's native unlimited
// generation for an ordinary source response. Intrinsically bounded station
// answers retain their complete-answer limit.
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
	if job.Kind == assemblyline.WorkFragmentGeneration {
		return -1, nil
	}
	responseBytes, err := assemblyline.PortableResponseMaximumBytesForJob(job)
	if err != nil {
		return 0, fmt.Errorf("derive portable response resource bound: %w", err)
	}
	framing, err := assemblyline.PortableResponseFramingForJob(job)
	if err != nil {
		return 0, fmt.Errorf("derive portable response termination reserve: %w", err)
	}
	reserve := 0
	if framing == assemblyline.PortableResponseFramingSingleLine {
		reserve = 1
	} else if framing != assemblyline.PortableResponseFramingNaturalMultiline {
		return 0, fmt.Errorf("portable response framing %q has no generation budget", framing)
	}
	budget := responseBytes + reserve
	if framing == assemblyline.PortableResponseFramingSingleLine &&
		budget < minSingleLineCompletionTokens {
		budget = minSingleLineCompletionTokens
	}
	return boundedProviderOutputTokens(budget, maxPortableStationOutputTokens, contextTokens)
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
		return boundedProviderOutputTokens(
			budget,
			maxSourceBodyCorrectionOutputTokens,
			contextTokens,
		)
	}
	if opaqueResponseBytes != 0 {
		return 0, fmt.Errorf("open source correction cannot carry an opaque response bound")
	}
	return -1, nil
}

func boundedProviderOutputTokens(budget, ceiling, contextTokens int) (int, error) {
	if budget < 1 || ceiling < 1 {
		return 0, fmt.Errorf("provider output budget must be positive")
	}
	if budget > ceiling {
		budget = ceiling
	}
	if budget >= contextTokens {
		budget = contextTokens - 1
	}
	if budget < 1 {
		return 0, fmt.Errorf("provider output budget leaves no native input authority")
	}
	return budget, nil
}
