package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

const (
	// ExactPreparedLineStopV1 terminates one-line semantic result grammars at
	// the provider boundary. Decoders still receive and validate exact bytes.
	ExactPreparedLineStopV1 = "\n"
	// MaxExactPreparedModelInputBytes is a gross transport/resource ceiling.
	// It is deliberately not a token estimate; the provider's tokenizer owns
	// native context admission and reports the actual counts in its receipt.
	MaxExactPreparedModelInputBytes = 128 * 1024
)

type ExactPreparedProtocol string

const (
	ExactPreparedProtocolPlainCompletionV4 ExactPreparedProtocol = "omnidex.ollama-plain-completion-request.v4"
)

func (protocol ExactPreparedProtocol) Validate() error {
	switch protocol {
	case ExactPreparedProtocolPlainCompletionV4:
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
// Model-native provider framing carries no application state or response
// contract and does not participate in semantic admission.
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

func ExactPreparedRequestSHA256(prepared PreparedModel) (string, error) {
	raw, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// ValidateExactPreparedInputAuthority validates the declared native token
// ceilings and one coarse byte safety ceiling. It never interprets bytes as
// tokens. Ollama tokenizes the exact raw request with truncate=false and its
// immutable receipt supplies the actual prompt/output token counts.
func ValidateExactPreparedInputAuthority(
	contextTokens int,
	maxOutputTokens int,
	rawInput string,
) error {
	if err := ValidateExactPreparedContextTokens(contextTokens); err != nil {
		return err
	}
	if maxOutputTokens != -1 &&
		(maxOutputTokens <= 0 || maxOutputTokens > contextTokens) {
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
	if len(rawInput) > MaxExactPreparedModelInputBytes {
		return fmt.Errorf(
			"exact semantic prompt exceeds %d-byte transport ceiling",
			MaxExactPreparedModelInputBytes,
		)
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
	if maxOutputTokens > 0 && usage.EvalCount > maxOutputTokens {
		return fmt.Errorf(
			"exact provider output exceeded: output_tokens=%d output_ceiling=%d",
			usage.EvalCount, maxOutputTokens,
		)
	}
	if usage.PromptEvalCount > contextTokens-usage.EvalCount {
		return fmt.Errorf(
			"exact provider aggregate native context exceeded: prompt_tokens=%d output_tokens=%d native_context=%d",
			usage.PromptEvalCount, usage.EvalCount, contextTokens,
		)
	}
	return nil
}
