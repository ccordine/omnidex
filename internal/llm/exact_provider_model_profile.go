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
	ExactPreparedTokenizerProfileQwen2Qwen2BOS = "ollama-0.24.0-qwen2-qwen2-bos-boundary-v1"
	ExactPreparedTokenizerProfileMistral3      = "ollama-0.24.0-mistral3-gpt2-bos-boundary-v1"
	ExactPreparedTokenizerProfilePhi3GPT4O     = "ollama-0.24.0-phi3-gpt2-gpt4o-boundary-v1"
	ExactPreparedTokenizerProfilePhi3DBRX      = "ollama-0.24.0-phi3-gpt2-dbrx-boundary-v1"
	ExactPreparedTokenizerProfileGemma3        = "ollama-0.24.0-gemma3-llama-default-boundary-v1"
	ExactPreparedTokenizerProfileLlama32       = "ollama-0.24.0-llama-gpt2-llama-bpe-boundary-v1"
)

type exactPreparedTransport uint8

const (
	exactPreparedTransportRaw exactPreparedTransport = iota + 1
	exactPreparedTransportNativeThinking
	exactPreparedTransportNativeSystem
)

// ExactPreparedTransportSettings exposes only the provider framing decisions
// that a caller must persist or validate. Output limits remain station-owned.
type ExactPreparedTransportSettings struct {
	NativeTemplate   bool
	SeparateThinking bool
	SeparateSystem   bool
	Temperature      *float64
}

type exactProviderModelProfile struct {
	tokenizerProfile      string
	architecture          string
	tokenizerModel        string
	tokenizerPre          string
	capabilities          []string
	explicitAdd           map[string]bool
	absentAdd             []string
	templateSHA256        string
	parameterSHA256s      []string
	parameterAssignments  map[string]string
	transport             exactPreparedTransport
	requestTemperature    float64
	requestTemperatureSet bool
}

var exactProviderModelProfiles = []exactProviderModelProfile{
	{
		tokenizerProfile: ExactPreparedTokenizerProfile,
		architecture:     "qwen35", tokenizerModel: "gpt2", tokenizerPre: "qwen35",
		capabilities:   []string{"completion", "vision", "tools", "thinking"},
		explicitAdd:    map[string]bool{"tokenizer.ggml.add_eos_token": false, "tokenizer.ggml.add_padding_token": false},
		absentAdd:      []string{"tokenizer.ggml.add_bos_token"},
		templateSHA256: "b507b9c2f6ca642bffcd06665ea7c91f235fd32daeefdf875a0f938db05fb315",
		parameterAssignments: map[string]string{
			"presence_penalty": "1.5",
			"temperature":      "1",
			"top_k":            "20",
			"top_p":            "0.95",
		},
		transport: exactPreparedTransportRaw, requestTemperature: 0, requestTemperatureSet: true,
	},
	{
		tokenizerProfile: ExactPreparedTokenizerProfileQwen3Qwen2,
		architecture:     "qwen3", tokenizerModel: "gpt2", tokenizerPre: "qwen2",
		capabilities:       []string{"completion", "thinking"},
		explicitAdd:        map[string]bool{"tokenizer.ggml.add_bos_token": false, "tokenizer.ggml.add_eos_token": false},
		absentAdd:          []string{"tokenizer.ggml.add_padding_token"},
		templateSHA256:     "c5ad996bda6eed4df6e3b605a9869647624851ac248209d22fd5e2c0cc1121d3",
		parameterSHA256s:   []string{"b3669fdd1d59cec39ead1d59150e66c4791f54b7ad95d034521d2011670ad2e1"},
		transport:          exactPreparedTransportNativeThinking,
		requestTemperature: 0.6, requestTemperatureSet: true,
	},
	{
		tokenizerProfile: ExactPreparedTokenizerProfileQwen2Qwen2BOS,
		architecture:     "qwen2", tokenizerModel: "gpt2", tokenizerPre: "qwen2",
		capabilities:       []string{"completion", "thinking"},
		explicitAdd:        map[string]bool{"tokenizer.ggml.add_bos_token": true, "tokenizer.ggml.add_eos_token": false},
		absentAdd:          []string{"tokenizer.ggml.add_padding_token"},
		templateSHA256:     "c5ad996bda6eed4df6e3b605a9869647624851ac248209d22fd5e2c0cc1121d3",
		parameterSHA256s:   []string{"52cf8b77cb8c5fe26ae7cbb18482e4c5ff22a861b6ee8e6e7a7085e7f6844e2f"},
		transport:          exactPreparedTransportNativeThinking,
		requestTemperature: 0.6, requestTemperatureSet: true,
	},
	{
		tokenizerProfile: ExactPreparedTokenizerProfileMistral3,
		architecture:     "mistral3", tokenizerModel: "gpt2", tokenizerPre: "default",
		capabilities: []string{"completion", "vision", "tools"},
		explicitAdd: map[string]bool{
			"tokenizer.ggml.add_bos_token": true, "tokenizer.ggml.add_eos_token": false,
			"tokenizer.ggml.add_padding_token": false, "tokenizer.ggml.add_unknown_token": false,
		},
		templateSHA256:   "5b74c26b9e0b6358e73d0a7eb2f955105097e7bd10b45fa0d2a50f0a906e0798",
		parameterSHA256s: []string{"e96fd3cee0f18f63a5df2dc1115f0bdf681fd2c9f75aa0f082f840f974111b9a"},
		transport:        exactPreparedTransportNativeSystem,
	},
	{
		tokenizerProfile: ExactPreparedTokenizerProfilePhi3GPT4O,
		architecture:     "phi3", tokenizerModel: "gpt2", tokenizerPre: "gpt-4o",
		capabilities: []string{"completion", "tools"},
		explicitAdd: map[string]bool{
			"tokenizer.ggml.add_bos_token": false, "tokenizer.ggml.add_eos_token": false,
		},
		absentAdd:        []string{"tokenizer.ggml.add_padding_token"},
		templateSHA256:   "813f53fdc6e58d35bb1c3853c93266380e9ca918a993e8eab193e8ede5d3a603",
		parameterSHA256s: []string{"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		transport:        exactPreparedTransportNativeSystem,
	},
	{
		tokenizerProfile: ExactPreparedTokenizerProfilePhi3DBRX,
		architecture:     "phi3", tokenizerModel: "gpt2", tokenizerPre: "dbrx",
		capabilities: []string{"completion"},
		absentAdd: []string{
			"tokenizer.ggml.add_bos_token", "tokenizer.ggml.add_eos_token", "tokenizer.ggml.add_padding_token",
		},
		templateSHA256:   "32695b892af87ef8fca6e13a1a31c67c1441d7398be037e366e2fc763857c06a",
		parameterSHA256s: []string{"0f21334ec3cf79cbaebd7b2c69ad7a38398074b109076af04b35c7925abe2675"},
		transport:        exactPreparedTransportNativeSystem,
	},
	{
		tokenizerProfile: ExactPreparedTokenizerProfileGemma3,
		architecture:     "gemma3", tokenizerModel: "llama", tokenizerPre: "default",
		capabilities: []string{"completion", "vision"},
		explicitAdd: map[string]bool{
			"tokenizer.ggml.add_bos_token": true, "tokenizer.ggml.add_eos_token": false,
			"tokenizer.ggml.add_padding_token": false, "tokenizer.ggml.add_unknown_token": false,
		},
		templateSHA256:   "e0a42594d802e5d31cdc786deb4823edb8adff66094d49de8fffe976d753e348",
		parameterSHA256s: []string{"82be0d39faf8dbd5f010de5f8619825954ef45533a1df7db4973110e71cef2d6"},
		transport:        exactPreparedTransportNativeSystem,
	},
	{
		tokenizerProfile: ExactPreparedTokenizerProfileLlama32,
		architecture:     "llama", tokenizerModel: "gpt2", tokenizerPre: "llama-bpe",
		capabilities: []string{"completion", "tools"},
		absentAdd: []string{
			"tokenizer.ggml.add_bos_token", "tokenizer.ggml.add_eos_token", "tokenizer.ggml.add_padding_token",
		},
		templateSHA256:     "966de95ca8a62200913e3f8bfbf84c8494536f1b94b49166851e76644e966396",
		parameterSHA256s:   []string{"2801e61a8848e505a6e20beeaea63cca1600200f6720e5f916ba7d6da5c3ba39"},
		transport:          exactPreparedTransportNativeSystem,
		requestTemperature: 0, requestTemperatureSet: true,
	},
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
	default:
		panic("unregistered exact prepared transport")
	}
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
	tokenizerPre, err := exactTokenizerString(response.ModelInfo, "tokenizer.ggml.pre")
	if err != nil {
		return exactProviderModelProfile{}, err
	}
	if err := validateExactTokenizerPayloads(response.ModelInfo); err != nil {
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
	return exactTokenizerBoundariesMatch(response.ModelInfo, profile.explicitAdd, profile.absentAdd)
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
