package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/exactjson"
)

const (
	// ExactPreparedRequestProtocolV1 bypasses model-specific chat templates by
	// using Ollama raw generation. The fixed prompt joiner is itself visible and
	// included in every byte/hash/budget authority.
	ExactPreparedRequestProtocolV1 = "omnidex.ollama-raw-generate-request.v1"
	ExactPreparedProviderBackend   = "ollama"
	ExactPreparedProviderVersion   = "0.24.0"
	ExactPreparedTokenizerProfile  = "ollama-0.24.0-qwen35-gpt2-boundary-v1"
	ExactPreparedPromptJoiner      = "\n"
	MaxRawInputSpecialTokenReserve = 2
)

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
	NumCtx      int     `json:"num_ctx"`
	NumPredict  int     `json:"num_predict"`
	Temperature float64 `json:"temperature"`
}

type exactPreparedRequest struct {
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
	if strings.TrimSpace(prepared.BaseModel) == "" ||
		prepared.ContextModel != prepared.BaseModel || strings.TrimSpace(prepared.Prompt) == "" ||
		prepared.PromptHint != MinimalGeneratePrompt || prepared.MaxOutputTokens <= 0 ||
		prepared.ContextTokens <= 0 || prepared.ResponseFormat != ResponseFormatJSON ||
		len(prepared.ResponseSchema) == 0 || prepared.ThinkingEnabled ||
		prepared.Temperature == nil || *prepared.Temperature != 0 {
		return nil, fmt.Errorf("prepared request does not satisfy the exact raw Ollama cognition contract")
	}
	if err := ValidateResponseContract(prepared); err != nil {
		return nil, err
	}
	prompt, err := ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
	if err != nil {
		return nil, err
	}
	return exactjson.Canonical(exactPreparedRequest{
		Format: prepared.ResponseSchema, Model: prepared.ContextModel,
		Options: exactPreparedRequestOptions{
			NumCtx: prepared.ContextTokens, NumPredict: prepared.MaxOutputTokens,
			Temperature: *prepared.Temperature,
		},
		Prompt: prompt, Raw: true, Shift: false, Stream: false, Think: false, Truncate: false,
	})
}

func ExactPreparedRequestSHA256(prepared PreparedModel) (string, error) {
	raw, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:]), nil
}

// ModelInputTokenUpperBound is hard only for the registered raw input
// protocol. Raw mode removes the model chat template; the frozen Brain binds a
// conservative reserve for the at-most BOS/EOS tokens the model may add.
func ModelInputTokenUpperBound(rawInput string, specialTokenReserve int) (int, error) {
	if rawInput == "" || specialTokenReserve < 0 ||
		specialTokenReserve > MaxRawInputSpecialTokenReserve {
		return 0, fmt.Errorf("raw model input token authority is invalid")
	}
	return len([]byte(rawInput)) + specialTokenReserve, nil
}

// ValidateExactPreparedInputBudget is the sole hard input-token authority for
// raw cognition calls. Each UTF-8 byte is conservatively treated as one token,
// and the frozen model contract reserves at most the registered BOS/EOS count.
func ValidateExactPreparedInputBudget(
	contextTokens int,
	maxInputTokens int,
	maxOutputTokens int,
	rawInput string,
	specialTokenReserve int,
) error {
	if err := ValidateInferenceContextTokens(contextTokens); err != nil {
		return err
	}
	if maxInputTokens <= 0 || maxOutputTokens <= 0 {
		return fmt.Errorf("exact raw cognition token ceilings must be positive")
	}
	upperBound, err := ModelInputTokenUpperBound(rawInput, specialTokenReserve)
	if err != nil {
		return err
	}
	if upperBound > maxInputTokens || upperBound+maxOutputTokens > contextTokens {
		return fmt.Errorf(
			"exact raw cognition input exceeds token authority: input_upper_bound=%d input_ceiling=%d output_ceiling=%d native_context=%d",
			upperBound, maxInputTokens, maxOutputTokens, contextTokens,
		)
	}
	return nil
}
