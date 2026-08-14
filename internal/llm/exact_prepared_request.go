package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	// ExactPreparedProtocolStructuredV1 bypasses model-specific chat templates by
	// using Ollama raw generation. The fixed prompt joiner is itself visible and
	// included in every byte/hash/budget authority.
	ExactPreparedProviderBackend         = "ollama"
	ExactPreparedProviderVersion         = "0.24.0"
	ExactPreparedTokenizerProfile        = "ollama-0.24.0-qwen35-gpt2-boundary-v1"
	ExactPreparedPromptJoiner            = "\n"
	ExactPreparedObjectiveAdvisoryStopV1 = "\n<END_OBJECTIVE_ADVISORY_V1>"
	ExactPreparedCodeStopV1              = "<|endoftext|>"
	// MaxExactPreparedModelInputBytes is a gross transport/resource ceiling.
	// It is deliberately not a token estimate; the provider's tokenizer owns
	// native context admission and reports the actual counts in its receipt.
	MaxExactPreparedModelInputBytes = 128 * 1024
)

type ExactPreparedProtocol string

const (
	ExactPreparedProtocolStructuredV1 ExactPreparedProtocol = "omnidex.ollama-raw-generate-request.v1"
	ExactPreparedProtocolRawTextV1    ExactPreparedProtocol = "omnidex.ollama-raw-text-generate-request.v1"
)

func (protocol ExactPreparedProtocol) Validate() error {
	switch protocol {
	case ExactPreparedProtocolStructuredV1, ExactPreparedProtocolRawTextV1:
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
		expected.BackendVersion != ExactPreparedProviderVersion ||
		expected.TokenizerProfile != ExactPreparedTokenizerProfile {
		return fmt.Errorf(
			"exact raw cognition supports only backend %s %s",
			ExactPreparedProviderBackend, ExactPreparedProviderVersion,
		)
	}
	return nil
}

type exactPreparedRequestOptions struct {
	NumCtx      int      `json:"num_ctx"`
	NumPredict  int      `json:"num_predict"`
	Stop        []string `json:"stop,omitempty"`
	Temperature float64  `json:"temperature"`
}

type exactPreparedRequest struct {
	Model    string                      `json:"model"`
	Options  exactPreparedRequestOptions `json:"options"`
	Prompt   string                      `json:"prompt"`
	Raw      bool                        `json:"raw"`
	Shift    bool                        `json:"shift"`
	Stream   bool                        `json:"stream"`
	Think    bool                        `json:"think"`
	Truncate bool                        `json:"truncate"`
}

type exactStructuredPreparedRequest struct {
	Format   map[string]any              `json:"format"`
	Model    string                      `json:"model"`
	Options  exactPreparedRequestOptions `json:"options"`
	Prompt   string                      `json:"prompt"`
	Raw      bool                        `json:"raw"`
	Shift    bool                        `json:"shift"`
	Stream   bool                        `json:"stream"`
	Think    bool                        `json:"think"`
	Truncate bool                        `json:"truncate"`
}

func ExactPreparedModelInput(systemEnvelope, promptHint string) (string, error) {
	if strings.TrimSpace(systemEnvelope) == "" || promptHint != MinimalGeneratePrompt {
		return "", fmt.Errorf("exact raw cognition input is incomplete")
	}
	return systemEnvelope + ExactPreparedPromptJoiner + promptHint, nil
}

// ExactPreparedRequestBytes renders the exact raw /api/generate request body.
// Both policy authority and the Ollama adapter call this sole function.
func ExactPreparedRequestBytes(prepared PreparedModel) ([]byte, error) {
	if err := validateExactPreparedRequest(prepared); err != nil {
		return nil, err
	}
	prompt, err := ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
	if err != nil {
		return nil, err
	}
	base := exactPreparedRequest{
		Model: prepared.ContextModel,
		Options: exactPreparedRequestOptions{
			NumCtx: prepared.ContextTokens, NumPredict: prepared.MaxOutputTokens,
			Temperature: *prepared.Temperature,
		},
		Prompt: prompt, Raw: true, Shift: false, Stream: false, Think: false, Truncate: false,
	}
	if prepared.RawTextStopSequence != "" {
		base.Options.Stop = []string{prepared.RawTextStopSequence}
	}
	if prepared.Protocol == ExactPreparedProtocolRawTextV1 {
		return exactjson.Canonical(base)
	}
	return exactjson.Canonical(exactStructuredPreparedRequest{
		Format: prepared.ResponseSchema, Model: base.Model, Options: base.Options,
		Prompt: base.Prompt, Raw: base.Raw, Shift: base.Shift, Stream: base.Stream,
		Think: base.Think, Truncate: base.Truncate,
	})
}

func validateExactPreparedRequest(prepared PreparedModel) error {
	if err := prepared.Protocol.Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(prepared.BaseModel) == "" ||
		prepared.ContextModel != prepared.BaseModel || strings.TrimSpace(prepared.Prompt) == "" ||
		prepared.PromptHint != MinimalGeneratePrompt || prepared.MaxOutputTokens <= 0 ||
		prepared.ContextTokens <= 0 || prepared.ThinkingEnabled ||
		prepared.Temperature == nil || *prepared.Temperature != 0 ||
		math.Signbit(*prepared.Temperature) {
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
	if err := (ProviderIdentityObservationRequest{
		Expectation:     *prepared.ProviderIdentityExpectation,
		ChallengeSHA256: prepared.ProviderObservationChallenge,
	}).Validate(); err != nil {
		return fmt.Errorf("prepared request has an invalid provider observation authority: %w", err)
	}
	switch prepared.Protocol {
	case ExactPreparedProtocolStructuredV1:
		if prepared.ResponseFormat != ResponseFormatJSON || len(prepared.ResponseSchema) == 0 ||
			prepared.RawTextStopSequence != "" {
			return fmt.Errorf("exact structured protocol requires a JSON response schema")
		}
	case ExactPreparedProtocolRawTextV1:
		if prepared.ResponseFormat != "" || prepared.ResponseSchema != nil {
			return fmt.Errorf("exact raw-text protocol forbids response format and schema")
		}
		if prepared.RawTextStopSequence != "" &&
			prepared.RawTextStopSequence != ExactPreparedObjectiveAdvisoryStopV1 &&
			prepared.RawTextStopSequence != ExactPreparedCodeStopV1 {
			return fmt.Errorf("exact raw-text protocol stop sequence is not registered")
		}
	}
	if err := ValidateResponseContract(prepared); err != nil {
		return err
	}
	rawInput, err := ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
	if err != nil {
		return err
	}
	return ValidateExactPreparedInputAuthority(
		prepared.ContextTokens,
		prepared.ContextTokens-prepared.MaxOutputTokens,
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
	maxInputTokens int,
	maxOutputTokens int,
	rawInput string,
) error {
	if err := ValidateInferenceContextTokens(contextTokens); err != nil {
		return err
	}
	if maxInputTokens <= 0 || maxOutputTokens <= 0 ||
		maxInputTokens != contextTokens-maxOutputTokens {
		return fmt.Errorf("exact raw cognition token ceilings must be positive")
	}
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
