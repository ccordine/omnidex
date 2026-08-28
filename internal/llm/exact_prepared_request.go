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
	// The protocol fixes the station result shape. The structurally attested
	// provider profile separately owns raw/native template framing.
	ExactPreparedProviderBackend            = "ollama"
	ExactPreparedProviderVersion            = "0.24.0"
	ExactPreparedTokenizerProfile           = "ollama-0.24.0-qwen35-gpt2-chatml-boundary-v2"
	ExactPreparedTokenizerProfileQwen3Qwen2 = "ollama-0.24.0-qwen3-qwen2-boundary-v1"
	ExactPreparedPromptJoiner               = "\n"
	// ExactPreparedLineStopV1 terminates one-line semantic result grammars at
	// the provider boundary. Decoders still receive and validate exact bytes.
	ExactPreparedLineStopV1 = "\n"
	// The sole raw provider profile is the exact Qwen 3.5 tokenizer profile.
	// Its registered ChatML controls provide a model-native assistant boundary
	// and end token without adding a semantic delimiter instruction.
	ExactPreparedRawChatUserPrefixV1        = "<|im_start|>user\n"
	ExactPreparedRawChatEndV1               = "<|im_end|>"
	ExactPreparedRawChatAssistantBoundaryV1 = ExactPreparedRawChatEndV1 +
		"\n<|im_start|>assistant\n<think>\n\n</think>\n\n"
	// MaxExactPreparedModelInputBytes is a gross transport/resource ceiling.
	// It is deliberately not a token estimate; the provider's tokenizer owns
	// native context admission and reports the actual counts in its receipt.
	MaxExactPreparedModelInputBytes = 128 * 1024
)

type ExactPreparedProtocol string

const (
	ExactPreparedProtocolRawTextV2 ExactPreparedProtocol = "omnidex.ollama-raw-text-generate-request.v2"
)

func (protocol ExactPreparedProtocol) Validate() error {
	switch protocol {
	case ExactPreparedProtocolRawTextV2:
		return nil
	default:
		return fmt.Errorf("exact prepared protocol is not registered")
	}
}

func ValidateExactPreparedProviderExpectation(expected ProviderIdentityExpectation) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	if expected.Backend != ExactPreparedProviderBackend ||
		expected.BackendVersion != ExactPreparedProviderVersion {
		return fmt.Errorf(
			"exact prepared cognition supports only backend %s %s",
			ExactPreparedProviderBackend, ExactPreparedProviderVersion,
		)
	}
	_, err := exactProviderModelProfileByID(expected.TokenizerProfile)
	return err
}

func ExactPreparedModelInput(systemEnvelope, promptHint string) (string, error) {
	if strings.TrimSpace(systemEnvelope) == "" || promptHint != MinimalGeneratePrompt {
		return "", fmt.Errorf("exact raw cognition input is incomplete")
	}
	return systemEnvelope + ExactPreparedPromptJoiner + promptHint, nil
}

// ExactPreparedRequestModelInput returns the exact joined model input bound to
// one discovered provider profile. Every raw transport receives one code-owned
// result boundary. Native-template transports retain the unmodified joined
// input. Result-grammar stops remain separate provider options.
func ExactPreparedRequestModelInput(prepared PreparedModel) (string, error) {
	if prepared.ProviderIdentityExpectation == nil {
		return "", fmt.Errorf("exact request model input requires one frozen provider identity")
	}
	expected := *prepared.ProviderIdentityExpectation
	if strings.TrimSpace(prepared.BaseModel) == "" || prepared.ContextModel != prepared.BaseModel ||
		expected.Model != prepared.BaseModel || prepared.ContextTokens != expected.NativeContextLimit {
		return "", fmt.Errorf("exact request model input differs from its frozen provider identity")
	}
	if err := ValidateExactPreparedProviderExpectation(expected); err != nil {
		return "", err
	}
	profile, err := exactProviderModelProfileByID(expected.TokenizerProfile)
	if err != nil {
		return "", err
	}
	switch prepared.RawTextStopSequence {
	case "", ExactPreparedLineStopV1, ExactPreparedRawChatEndV1:
	default:
		return "", fmt.Errorf("exact raw-text protocol stop sequence is not registered")
	}
	modelInput, err := ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
	if err != nil {
		return "", err
	}
	if profile.transport != exactPreparedTransportRaw {
		if prepared.RawTextStopSequence != "" &&
			prepared.RawTextStopSequence != ExactPreparedLineStopV1 {
			return "", fmt.Errorf("native provider received a raw-only response stop")
		}
		return modelInput, nil
	}
	if prepared.RawTextStopSequence != ExactPreparedLineStopV1 &&
		prepared.RawTextStopSequence != ExactPreparedRawChatEndV1 {
		return "", fmt.Errorf("exact raw provider requires one registered ChatML result stop")
	}
	if strings.Contains(modelInput, "<|im_start|>") ||
		strings.Contains(modelInput, ExactPreparedRawChatEndV1) {
		return "", fmt.Errorf("exact raw model input contains a reserved ChatML control token")
	}
	return ExactPreparedRawChatUserPrefixV1 + modelInput +
		ExactPreparedRawChatAssistantBoundaryV1, nil
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
		prepared.PromptHint != MinimalGeneratePrompt || prepared.MaxOutputTokens <= 0 ||
		prepared.ContextTokens <= 0 {
		return fmt.Errorf("prepared request does not satisfy the exact Ollama generation contract")
	}
	if prepared.ProviderIdentityExpectation == nil ||
		prepared.ProviderIdentityExpectation.Model != prepared.BaseModel ||
		prepared.ProviderIdentityExpectation.NativeContextLimit != prepared.ContextTokens {
		return fmt.Errorf("prepared request lacks its frozen provider identity")
	}
	if err := ValidateExactPreparedProviderExpectation(*prepared.ProviderIdentityExpectation); err != nil {
		return err
	}
	profile, err := exactProviderModelProfileByID(
		prepared.ProviderIdentityExpectation.TokenizerProfile,
	)
	if err != nil {
		return err
	}
	if err := profile.validatePreparedTemperature(prepared.Temperature); err != nil {
		return err
	}
	if prepared.Temperature != nil && math.Signbit(float64(*prepared.Temperature)) {
		return fmt.Errorf("prepared request temperature cannot be negative zero")
	}
	wantThinking := profile.transport == exactPreparedTransportNativeThinking ||
		profile.transport == exactPreparedTransportNativeSystemThinking
	if prepared.ThinkingEnabled != wantThinking {
		return fmt.Errorf("prepared reasoning mode differs from its exact provider profile")
	}
	if err := (ProviderIdentityObservationRequest{
		Expectation:     *prepared.ProviderIdentityExpectation,
		ChallengeSHA256: prepared.ProviderObservationChallenge,
	}).Validate(); err != nil {
		return fmt.Errorf("prepared request has an invalid provider observation authority: %w", err)
	}
	if prepared.RawTextStopSequence != "" &&
		prepared.RawTextStopSequence != ExactPreparedLineStopV1 &&
		prepared.RawTextStopSequence != ExactPreparedRawChatEndV1 {
		return fmt.Errorf("exact raw-text protocol stop sequence is not registered")
	}
	if err := ValidateResponseContract(prepared); err != nil {
		return err
	}
	rawInput, err := ExactPreparedRequestModelInput(prepared)
	if err != nil {
		return err
	}
	if prepared.OutputLimitMode == ExactPreparedOutputLimitNatural {
		if prepared.MaxOutputTokens > prepared.ContextTokens {
			return fmt.Errorf("natural output ceiling exceeds the native context")
		}
		return ValidateExactPreparedNaturalInputAuthority(prepared.ContextTokens, rawInput)
	}
	return ValidateExactPreparedInputAuthority(
		prepared.ContextTokens,
		prepared.ContextTokens-prepared.MaxOutputTokens,
		prepared.MaxOutputTokens,
		rawInput,
	)
}

// ValidateExactPreparedNaturalInputAuthority enforces only native context and
// transport bounds. Natural output shares the remaining native context and is
// checked from the provider's tokenizer-owned receipt.
func ValidateExactPreparedNaturalInputAuthority(contextTokens int, rawInput string) error {
	if err := ValidateExactPreparedContextTokens(contextTokens); err != nil {
		return err
	}
	return validateExactPreparedInputBytes(rawInput)
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
	maxInputTokens int,
	maxOutputTokens int,
	rawInput string,
) error {
	if err := ValidateExactPreparedContextTokens(contextTokens); err != nil {
		return err
	}
	if maxInputTokens <= 0 || maxOutputTokens <= 0 ||
		maxInputTokens != contextTokens-maxOutputTokens {
		return fmt.Errorf("exact raw cognition token ceilings must be positive")
	}
	return validateExactPreparedInputBytes(rawInput)
}

func validateExactPreparedInputBytes(rawInput string) error {
	if !utf8.ValidString(rawInput) || strings.ContainsRune(rawInput, 0) ||
		strings.TrimSpace(rawInput) == "" {
		return fmt.Errorf("exact raw cognition input is invalid")
	}
	if len(rawInput) > MaxExactPreparedModelInputBytes {
		return fmt.Errorf(
			"exact raw cognition input exceeds %d-byte transport ceiling",
			MaxExactPreparedModelInputBytes,
		)
	}
	return nil
}

// ValidateExactPreparedNativeUsage proves the actual provider-tokenized call
// stayed inside the declared input, output, and native context authorities.
func ValidateExactPreparedNativeUsage(
	contextTokens int,
	maxInputTokens int,
	maxOutputTokens int,
	usage ProviderGenerationUsage,
) error {
	if err := ValidateExactPreparedInputAuthority(
		contextTokens, maxInputTokens, maxOutputTokens, "usage-receipt",
	); err != nil {
		return err
	}
	if usage.PromptEvalCount <= 0 || usage.EvalCount <= 0 {
		return fmt.Errorf("exact native usage requires positive prompt and output token counts")
	}
	if usage.PromptEvalCount > maxInputTokens || usage.EvalCount > maxOutputTokens ||
		usage.PromptEvalCount+usage.EvalCount > contextTokens {
		return fmt.Errorf(
			"exact provider context exceeded: prompt_tokens=%d input_ceiling=%d output_tokens=%d output_ceiling=%d native_context=%d",
			usage.PromptEvalCount, maxInputTokens, usage.EvalCount, maxOutputTokens, contextTokens,
		)
	}
	return nil
}

// ValidateExactPreparedNaturalUsage validates a natural-stop receipt without
// inventing separate prompt/output sub-ceilings that were not sent to Ollama.
func ValidateExactPreparedNaturalUsage(
	contextTokens int,
	usage ProviderGenerationUsage,
) error {
	if err := ValidateExactPreparedContextTokens(contextTokens); err != nil {
		return err
	}
	if usage.PromptEvalCount <= 0 || usage.EvalCount <= 0 {
		return fmt.Errorf("exact natural usage requires positive prompt and output token counts")
	}
	if usage.PromptEvalCount+usage.EvalCount > contextTokens {
		return fmt.Errorf(
			"exact provider natural context exceeded: prompt_tokens=%d output_tokens=%d native_context=%d",
			usage.PromptEvalCount, usage.EvalCount, contextTokens,
		)
	}
	return nil
}

// ValidateExactPreparedNaturalUsageWithOutputCeiling validates natural shared
// context plus the finite output ceiling actually sent to the provider. It is
// illegal to use this function for a native profile whose request omitted
// num_predict.
func ValidateExactPreparedNaturalUsageWithOutputCeiling(
	contextTokens int,
	maxOutputTokens int,
	usage ProviderGenerationUsage,
) error {
	if maxOutputTokens < 1 || maxOutputTokens > contextTokens {
		return fmt.Errorf("exact natural output ceiling must be within the native context")
	}
	if err := ValidateExactPreparedNaturalUsage(contextTokens, usage); err != nil {
		return err
	}
	if usage.EvalCount > maxOutputTokens {
		return fmt.Errorf(
			"exact provider natural output exceeded: output_tokens=%d output_ceiling=%d native_context=%d",
			usage.EvalCount, maxOutputTokens, contextTokens,
		)
	}
	return nil
}
