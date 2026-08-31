package llm

import (
	"fmt"
)

// ExactPreparedOutputLimitReachedError is a validated provider completion
// fact. It contains no returned content and grants no retry or workflow
// authority. Source-specific code may use it to preserve the unresolved leaf
// while opening one separately persisted bounded replacement responsibility.
type ExactPreparedOutputLimitReachedError struct {
	DoneReason      string
	PromptTokens    int
	OutputTokens    int
	ContextTokens   int
	MaxOutputTokens int
	ContentBytes    int
}

func (failure *ExactPreparedOutputLimitReachedError) Error() string {
	if failure == nil {
		return "exact prepared output limit evidence is absent"
	}
	return fmt.Sprintf(
		"exact prepared output ended with done_reason=%s: prompt_tokens=%d output_tokens=%d native_context=%d max_output_tokens=%d content_bytes=%d",
		failure.DoneReason, failure.PromptTokens, failure.OutputTokens,
		failure.ContextTokens, failure.MaxOutputTokens, failure.ContentBytes,
	)
}

func (failure ExactPreparedOutputLimitReachedError) Validate() error {
	if failure.DoneReason != "length" || failure.PromptTokens < 1 ||
		failure.OutputTokens < 1 || failure.ContextTokens < 1 ||
		failure.MaxOutputTokens < 1 || failure.MaxOutputTokens > failure.ContextTokens ||
		failure.PromptTokens+failure.OutputTokens > failure.ContextTokens ||
		failure.ContentBytes < 1 ||
		failure.ContentBytes > MaxExactPreparedProviderResponseBytes {
		return fmt.Errorf("exact prepared output limit evidence is invalid")
	}
	return nil
}

// ValidateExactPreparedGenerationForRequest binds a successful provider
// generation to the exact request that produced it. Generic receipt validation
// cannot decide whether provider-profile control bytes leaked into content.
func ValidateExactPreparedGenerationForRequest(
	prepared PreparedModel,
	generation PreparedGeneration,
) error {
	if err := validateExactPreparedRequest(prepared); err != nil {
		return fmt.Errorf("validate exact prepared generation request: %w", err)
	}
	if err := generation.validateSuccessfulContentEvidence(); err != nil {
		return err
	}
	requestSHA256, err := ExactPreparedRequestSHA256(prepared)
	if err != nil {
		return err
	}
	if generation.ProviderRequestSHA256 != requestSHA256 {
		return fmt.Errorf("exact prepared generation differs from its request authority")
	}
	if prepared.OutputLimitMode == ExactPreparedOutputLimitNatural {
		if err := ValidateExactPreparedNaturalUsageWithOutputCeiling(
			prepared.ContextTokens, prepared.MaxOutputTokens, generation.Usage,
		); err != nil {
			return err
		}
	}
	if generation.ProviderDoneReason == "length" {
		failure := &ExactPreparedOutputLimitReachedError{
			DoneReason:      generation.ProviderDoneReason,
			PromptTokens:    generation.Usage.PromptEvalCount,
			OutputTokens:    generation.Usage.EvalCount,
			ContextTokens:   prepared.ContextTokens,
			MaxOutputTokens: prepared.MaxOutputTokens,
			ContentBytes:    len(generation.Content),
		}
		if err := failure.Validate(); err != nil {
			return err
		}
		return failure
	}
	return nil
}
