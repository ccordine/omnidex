package llm

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	// ExactPreparedLineStopV1 terminates one-line semantic result grammars at
	// the provider boundary. Decoders still receive and validate exact bytes.
	ExactPreparedLineStopV1 = "\n"
)

type ExactPreparedProtocol string

const (
	ExactPreparedProtocolPlainCompletionV4 ExactPreparedProtocol = "omnidex.ollama-plain-completion-request.v4"
	ExactPreparedProtocolCodexV1           ExactPreparedProtocol = "omnidex.codex-app-server-request.v1"
)

func (protocol ExactPreparedProtocol) Validate() error {
	switch protocol {
	case ExactPreparedProtocolPlainCompletionV4, ExactPreparedProtocolCodexV1:
		return nil
	default:
		return fmt.Errorf("exact prepared protocol is not registered")
	}
}

func ExactPreparedModelInput(prompt string) (string, error) {
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("exact semantic prompt is incomplete")
	}
	return prompt, nil
}

// ExactPreparedRequestModelInput returns the one code-owned semantic prompt.
// Retained provider context is carried separately; it never enlarges this
// new semantic question or copies the retained source into the prompt.
func ExactPreparedRequestModelInput(prepared PreparedModel) (string, error) {
	if strings.TrimSpace(prepared.BaseModel) == "" || prepared.ContextModel != prepared.BaseModel ||
		prepared.ContextTokens <= 0 {
		return "", fmt.Errorf("exact request model input is incomplete")
	}
	switch prepared.RawTextStopSequence {
	case "", ExactPreparedLineStopV1:
	default:
		return "", fmt.Errorf("exact completion stop sequence is not registered")
	}
	return ExactPreparedModelInput(prepared.Prompt)
}

func validateExactPreparedRequest(prepared PreparedModel) error {
	if err := prepared.Protocol.Validate(); err != nil {
		return err
	}
	if prepared.Protocol == ExactPreparedProtocolCodexV1 {
		return validateCodexPreparedRequest(prepared)
	}
	if len(prepared.RetainedProviderContext) != 0 {
		return fmt.Errorf("Ollama request contains another provider's continuation")
	}
	if err := prepared.OutputLimitMode.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(prepared.BaseModel) == "" ||
		prepared.ContextModel != prepared.BaseModel || strings.TrimSpace(prepared.Prompt) == "" ||
		prepared.ContextTokens <= 0 {
		return fmt.Errorf("prepared request does not satisfy the exact Ollama generation contract")
	}
	if prepared.Temperature != nil && math.Signbit(float64(*prepared.Temperature)) {
		return fmt.Errorf("prepared request temperature cannot be negative zero")
	}
	if prepared.RawTextStopSequence != "" &&
		prepared.RawTextStopSequence != ExactPreparedLineStopV1 {
		return fmt.Errorf("exact completion stop sequence is not registered")
	}
	if err := ValidateResponseContract(prepared); err != nil {
		return err
	}
	if prepared.RetainedContext != nil {
		if err := validateExactPreparedRetainedContext(prepared.RetainedContext, prepared.ContextTokens); err != nil {
			return err
		}
	}
	rawInput, err := ExactPreparedRequestModelInput(prepared)
	if err != nil {
		return err
	}
	return ValidateExactPreparedInputAuthority(
		prepared.ContextTokens,
		prepared.MaxOutputTokens,
		rawInput,
	)
}

// ValidateExactPreparedInputAuthority validates declared Ollama token controls
// and exact well-formed text, without a prompt byte ceiling or byte-to-token
// estimate. Ollama tokenizes the exact raw request with truncate=false and its
// response supplies the actual prompt/output token counts.
func ValidateExactPreparedInputAuthority(
	contextTokens int,
	maxOutputTokens int,
	rawInput string,
) error {
	if err := ValidateExactPreparedContextTokens(contextTokens); err != nil {
		return err
	}
	if maxOutputTokens != -1 &&
		(maxOutputTokens <= 0) {
		return fmt.Errorf(
			"exact completion num_predict must be provider-unlimited -1 or positive and no greater than native context",
		)
	}
	return validateExactPreparedInputBytes(rawInput)
}

func validateExactPreparedInputBytes(rawInput string) error {
	if !utf8.ValidString(rawInput) || strings.ContainsRune(rawInput, 0) ||
		strings.TrimSpace(rawInput) == "" {
		return fmt.Errorf("exact semantic prompt is invalid")
	}
	return nil
}

// ValidateExactPreparedNativeUsage proves the actual provider-tokenized call
// stayed inside the declared output and aggregate native context authorities.
func ValidateExactPreparedNativeUsage(
	contextTokens int,
	maxOutputTokens int,
	usage ProviderGenerationUsage,
) error {
	if err := ValidateExactPreparedInputAuthority(
		contextTokens, maxOutputTokens, "usage-receipt",
	); err != nil {
		return err
	}
	if usage.PromptEvalCount <= 0 || usage.EvalCount <= 0 {
		return fmt.Errorf("exact native usage requires positive prompt and output token counts")
	}

	if usage.ReasoningEvalCount < 0 {
		return fmt.Errorf(
			"exact provider aggregate native context exceeded: prompt_tokens=%d output_tokens=%d native_context=%d",
			usage.PromptEvalCount, usage.EvalCount, contextTokens,
		)
	}
	return nil
}
