package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const (
	ExactPreparedTokenizerProfileQwen2Qwen2BOS   = "ollama-0.24.0-qwen2-qwen2-bos-boundary-v1"
	ExactPreparedTokenizerProfileMistral3        = "ollama-0.24.0-mistral3-gpt2-bos-boundary-v1"
	ExactPreparedTokenizerProfilePhi3GPT4O       = "ollama-0.24.0-phi3-gpt2-gpt4o-boundary-v1"
	ExactPreparedTokenizerProfilePhi3DBRX        = "ollama-0.24.0-phi3-gpt2-dbrx-boundary-v1"
	ExactPreparedTokenizerProfileGemma3          = "ollama-0.24.0-gemma3-llama-default-boundary-v1"
	ExactPreparedTokenizerProfileLlama32         = "ollama-0.24.0-llama-gpt2-llama-bpe-boundary-v1"
	ExactPreparedTokenizerProfileQwen25Coder     = "ollama-0.24.0-qwen2-gpt2-qwen2-no-bos-boundary-v1"
	ExactPreparedTokenizerProfileQwen3Native     = "ollama-0.24.0-qwen3-gpt2-qwen2-no-bos-boundary-v1"
	ExactPreparedTokenizerProfileCodeQwen        = "ollama-0.24.0-qwen2-llama-default-code-boundary-v1"
	ExactPreparedTokenizerProfileCodeGemmaFIM    = "ollama-0.24.0-gemma-llama-default-fim-boundary-v1"
	ExactPreparedTokenizerProfileCodeGemmaChat   = "ollama-0.24.0-gemma-llama-default-chat-boundary-v1"
	ExactPreparedTokenizerProfileCodeLlama       = "ollama-0.24.0-llama-llama-default-code-boundary-v1"
	ExactPreparedTokenizerProfileDeepSeekCoder   = "ollama-0.24.0-llama-gpt2-no-pre-deepseek-code-boundary-v1"
	ExactPreparedTokenizerProfileDeepSeekCoderV2 = "ollama-0.24.0-deepseek2-gpt2-deepseek-llm-code-boundary-v1"
)

type exactPreparedTransport uint8

const (
	exactPreparedTransportRaw exactPreparedTransport = iota + 1
	exactPreparedTransportNativeThinking
	exactPreparedTransportNativeSystem
	exactPreparedTransportNativeSystemThinking
	exactPreparedTransportNativePrompt
)

// ExactPreparedTransportSettings exposes only the provider framing decisions
// that a caller must persist or validate. Output limits remain station-owned.
type ExactPreparedTransportSettings struct {
	NativeTemplate   bool
	SeparateThinking bool
	SeparateSystem   bool
	Temperature      *ExactPreparedTemperature
}

type exactProviderModelProfile struct {
	tokenizerProfile          string
	architecture              string
	tokenizerModel            string
	tokenizerPre              string
	capabilities              []string
	explicitAdd               map[string]bool
	absentAdd                 []string
	templateSHA256            string
	parameterSHA256s          []string
	parameterAssignments      map[string]string
	exactTokenizerFields      map[string]string
	transport                 exactPreparedTransport
	requestTemperature        ExactPreparedTemperature
	requestTemperatureSet     bool
	requestTemperatureCeiling ExactPreparedTemperature
}

func ResolveExactPreparedTransport(
	expected ProviderIdentityExpectation,
) (ExactPreparedTransportSettings, error) {
	if err := ValidateExactPreparedProviderExpectation(expected); err != nil {
		return ExactPreparedTransportSettings{}, err
	}
	profile, err := exactProviderModelProfileByID(expected.TokenizerProfile)
	if err != nil {
		return ExactPreparedTransportSettings{}, err
	}
	return profile.transportSettings(), nil
}

func (profile exactProviderModelProfile) transportSettings() ExactPreparedTransportSettings {
	settings := ExactPreparedTransportSettings{}
	if profile.requestTemperatureSet {
		temperature := profile.requestTemperature
		settings.Temperature = &temperature
	}
	switch profile.transport {
	case exactPreparedTransportRaw:
		return settings
	case exactPreparedTransportNativeThinking:
		settings.NativeTemplate = true
		settings.SeparateThinking = true
		return settings
	case exactPreparedTransportNativeSystem:
		settings.NativeTemplate = true
		settings.SeparateSystem = true
		return settings
	case exactPreparedTransportNativeSystemThinking:
		settings.NativeTemplate = true
		settings.SeparateThinking = true
		settings.SeparateSystem = true
		return settings
	case exactPreparedTransportNativePrompt:
		settings.NativeTemplate = true
		return settings
	default:
		panic("unregistered exact prepared transport")
	}
}

var exactPreparedExplorationTemperatures = [...]ExactPreparedTemperature{
	0.2, 0.4, 0.6, 0.8, 1,
}

// NextExactPreparedTemperature advances only through a structural profile's
// registered sampling authority. The nil baseline preserves a provider's
// attested native default until deterministic exploration is required.
func NextExactPreparedTemperature(
	expected ProviderIdentityExpectation,
	current *ExactPreparedTemperature,
) (*ExactPreparedTemperature, bool, error) {
	if err := ValidateExactPreparedProviderExpectation(expected); err != nil {
		return nil, false, err
	}
	profile, err := exactProviderModelProfileByID(expected.TokenizerProfile)
	if err != nil {
		return nil, false, err
	}
	if err := profile.validatePreparedTemperature(current); err != nil {
		return nil, false, err
	}
	currentValue := ExactPreparedTemperature(-1)
	if current != nil {
		currentValue = *current
	} else if profile.requestTemperatureSet {
		return nil, false, fmt.Errorf("exact prepared temperature omitted its registered profile baseline")
	}
	for _, candidate := range exactPreparedExplorationTemperatures {
		if candidate > currentValue && candidate <= profile.requestTemperatureCeiling {
			next := candidate
			return &next, true, nil
		}
	}
	return nil, false, nil
}

func (profile exactProviderModelProfile) validatePreparedTemperature(
	temperature *ExactPreparedTemperature,
) error {
	if profile.requestTemperatureCeiling <= 0 || profile.requestTemperatureCeiling > 2 {
		return fmt.Errorf("exact provider model profile has invalid temperature authority")
	}
	if temperature == nil {
		if profile.requestTemperatureSet {
			return fmt.Errorf("exact prepared request omitted its registered profile temperature")
		}
		return nil
	}
	if profile.requestTemperatureSet && *temperature == profile.requestTemperature {
		return nil
	}
	for _, candidate := range exactPreparedExplorationTemperatures {
		if *temperature == candidate && candidate <= profile.requestTemperatureCeiling &&
			(!profile.requestTemperatureSet || candidate > profile.requestTemperature) {
			return nil
		}
	}
	return fmt.Errorf("exact prepared temperature %v is outside its registered profile policy", *temperature)
}

func deriveExactProviderModelProfile(
	response exactIdentityShowResponse,
) (exactProviderModelProfile, error) {
	architecture, err := exactTokenizerString(response.ModelInfo, "general.architecture")
	if err != nil {
		return exactProviderModelProfile{}, err
	}
	tokenizerModel, err := exactTokenizerString(response.ModelInfo, "tokenizer.ggml.model")
	if err != nil {
		return exactProviderModelProfile{}, err
	}
	tokenizerPre, err := exactTokenizerOptionalString(response.ModelInfo, "tokenizer.ggml.pre")
	if err != nil {
		return exactProviderModelProfile{}, err
	}
	for _, profile := range exactProviderModelProfiles {
		if profile.matches(response, architecture, tokenizerModel, tokenizerPre) {
			return profile, nil
		}
	}
	return exactProviderModelProfile{}, fmt.Errorf(
		"provider model profile architecture=%q tokenizer=%q pre=%q template_sha256=%s parameters_sha256=%s is not registered",
		architecture, tokenizerModel, tokenizerPre,
		exactProfileSHA256(response.Template), exactProfileSHA256(response.Parameters),
	)
}

func (profile exactProviderModelProfile) matches(
	response exactIdentityShowResponse,
	architecture string,
	tokenizerModel string,
	tokenizerPre string,
) bool {
	parametersMatch := slices.Contains(profile.parameterSHA256s, exactProfileSHA256(response.Parameters))
	if len(profile.parameterAssignments) > 0 {
		parametersMatch = exactParameterAssignmentsMatch(response.Parameters, profile.parameterAssignments)
	}
	if architecture != profile.architecture || tokenizerModel != profile.tokenizerModel ||
		tokenizerPre != profile.tokenizerPre || !slices.Equal(response.Capabilities, profile.capabilities) ||
		exactProfileSHA256(response.Template) != profile.templateSHA256 ||
		!parametersMatch {
		return false
	}
	return exactTokenizerBoundariesMatch(response.ModelInfo, profile.explicitAdd, profile.absentAdd) &&
		profile.matchesExactTokenizerFields(response.ModelInfo)
}

func exactParameterAssignmentsMatch(parameters string, expected map[string]string) bool {
	if parameters == "" || parameters != strings.TrimSpace(parameters) {
		return false
	}
	actual := make(map[string]string, len(expected))
	for _, line := range strings.Split(parameters, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || fields[0] == "" || fields[1] == "" {
			return false
		}
		if _, exists := actual[fields[0]]; exists {
			return false
		}
		actual[fields[0]] = fields[1]
	}
	if len(actual) != len(expected) {
		return false
	}
	for key, want := range expected {
		if actual[key] != want {
			return false
		}
	}
	return true
}

func exactProviderModelProfileByID(id string) (exactProviderModelProfile, error) {
	for _, profile := range exactProviderModelProfiles {
		if profile.tokenizerProfile == id {
			return profile, nil
		}
	}
	return exactProviderModelProfile{}, fmt.Errorf("exact provider model profile %q is not registered", id)
}

func exactTokenizerString(info map[string]json.RawMessage, key string) (string, error) {
	var value string
	raw, exists := info[key]
	if !exists || json.Unmarshal(raw, &value) != nil || value == "" || value != strings.TrimSpace(value) {
		return "", fmt.Errorf("tokenizer profile field %q is not exact text", key)
	}
	return value, nil
}

func exactTokenizerOptionalString(info map[string]json.RawMessage, key string) (string, error) {
	if _, exists := info[key]; !exists {
		return "", nil
	}
	return exactTokenizerString(info, key)
}

func validateExactTokenizerPayloads(info map[string]json.RawMessage) error {
	for _, key := range []string{
		"tokenizer.ggml.tokens", "tokenizer.ggml.token_type", "tokenizer.ggml.merges",
	} {
		if raw, exists := info[key]; !exists || string(raw) != "null" {
			return fmt.Errorf("tokenizer profile field %q must be explicit null", key)
		}
	}
	return nil
}

func exactTokenizerBoundariesMatch(
	info map[string]json.RawMessage,
	explicit map[string]bool,
	absent []string,
) bool {
	for key, want := range explicit {
		var got bool
		if raw, exists := info[key]; !exists || json.Unmarshal(raw, &got) != nil || got != want {
			return false
		}
	}
	for _, key := range absent {
		if _, exists := info[key]; exists {
			return false
		}
	}
	for key := range info {
		if strings.HasPrefix(key, "tokenizer.ggml.add_") {
			if _, allowed := explicit[key]; !allowed {
				return false
			}
		}
	}
	return true
}

func exactProfileSHA256(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
