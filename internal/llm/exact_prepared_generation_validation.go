package llm

import (
	"fmt"
)

// ExactPreparedOutputLimitReachedError is a validated provider completion
// fact. It contains no returned content and is never semantic output. It is a
// terminal provider failure and grants no retry, correction, or workflow
// authority.
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
		(failure.MaxOutputTokens != -1 &&
			(failure.MaxOutputTokens < 1 || failure.MaxOutputTokens > failure.ContextTokens)) ||
		failure.PromptTokens+failure.OutputTokens > failure.ContextTokens ||
		failure.ContentBytes < 1 ||
		failure.ContentBytes > MaxExactPreparedModelContentBytes {
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
	if generation.Protocol != prepared.Protocol {
		return fmt.Errorf("provider generation differs from the frozen request protocol")
	}
	if err := validateExactPreparedRequest(prepared); err != nil {
		return fmt.Errorf("validate exact prepared generation request: %w", err)
	}
	if err := generation.validateSuccessfulContentEvidence(); err != nil {
		return err
	}
	if prepared.Protocol == ExactPreparedProtocolCodexV1 {
		_, native, err := decodeCodexCapture(generation.ProviderResponseCapture)
		if err != nil || native.Model != prepared.BaseModel {
			return fmt.Errorf("Codex response differs from the frozen model route")
		}
		if len(prepared.RetainedProviderContext) != 0 {
			parent, err := ParseCodexContinuation(prepared.RetainedProviderContext)
			if err != nil || parent.ThreadID != native.ThreadID || parent.TurnID == native.TurnID {
				return fmt.Errorf("Codex response differs from its original continuation")
			}
		}
	}
	// Codex owns its native context window. ContextTokens configures Ollama;
	// applying it to a completed Codex turn rejects valid native receipts.
	if prepared.Protocol != ExactPreparedProtocolCodexV1 {
		if err := ValidateExactPreparedNativeUsage(prepared.ContextTokens, prepared.MaxOutputTokens, generation.Usage); err != nil {
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
