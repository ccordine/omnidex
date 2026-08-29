package llm

import (
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/exactjson"
)

type exactPreparedRequestOptions struct {
	NumCtx      int                       `json:"num_ctx"`
	NumPredict  int                       `json:"num_predict,omitempty"`
	Stop        []string                  `json:"stop,omitempty"`
	Temperature *ExactPreparedTemperature `json:"temperature,omitempty"`
}

type exactPreparedRequest struct {
	Model    string                      `json:"model"`
	Options  exactPreparedRequestOptions `json:"options"`
	Prompt   string                      `json:"prompt"`
	Raw      bool                        `json:"raw"`
	Shift    bool                        `json:"shift"`
	Stream   bool                        `json:"stream"`
	System   string                      `json:"system,omitempty"`
	Think    *bool                       `json:"think,omitempty"`
	Truncate bool                        `json:"truncate"`
}

// ExactPreparedRequestBytes renders the sole exact /api/generate transport.
// The attested structural profile owns framing; the station owns output mode.
func ExactPreparedRequestBytes(prepared PreparedModel) ([]byte, error) {
	if err := validateExactPreparedRequest(prepared); err != nil {
		return nil, err
	}
	profile, err := exactProviderModelProfileByID(
		prepared.ProviderIdentityExpectation.TokenizerProfile,
	)
	if err != nil {
		return nil, err
	}
	request, err := profile.preparedRequest(prepared)
	if err != nil {
		return nil, err
	}
	if request.Think != nil && *request.Think {
		return nil, fmt.Errorf("exact prepared request enabled forbidden provider thinking")
	}
	if slices.Contains(profile.capabilities, "thinking") && request.Think == nil {
		return nil, fmt.Errorf("thinking-capable provider request omitted think=false")
	}
	return exactjson.Canonical(request)
}

func (profile exactProviderModelProfile) preparedRequest(
	prepared PreparedModel,
) (exactPreparedRequest, error) {
	request := exactPreparedRequest{
		Model:   prepared.ContextModel,
		Options: exactPreparedRequestOptions{NumCtx: prepared.ContextTokens},
		Shift:   false, Stream: false, Truncate: false,
	}
	// LF is the sole cross-transport result boundary. Raw-only multiline and
	// source stops must never override an attested native template's controls.
	if prepared.RawTextStopSequence == ExactPreparedLineStopV1 ||
		(profile.transport == exactPreparedTransportRaw &&
			prepared.RawTextStopSequence != "") {
		request.Options.Stop = []string{prepared.RawTextStopSequence}
	}
	// Raw profiles have no attested template/EOS liveness guarantee. Always
	// send their finite code-owned ceiling; natural mode still shares the
	// remaining native context and validates the provider's measured usage.
	if prepared.OutputLimitMode == ExactPreparedOutputLimitExplicit ||
		profile.naturalOutputCeiling {
		request.Options.NumPredict = prepared.MaxOutputTokens
	}
	request.Options.Temperature = prepared.Temperature
	switch profile.transport {
	case exactPreparedTransportRaw:
		return profile.rawPreparedRequest(prepared, request)
	case exactPreparedTransportNativeNoThinking:
		return profile.noThinkingPreparedRequest(prepared, request)
	case exactPreparedTransportNativeSystem:
		request.Prompt = prepared.PromptHint
		request.System = prepared.Prompt
		return request, nil
	case exactPreparedTransportNativeSystemNoThinking:
		request.Prompt = prepared.PromptHint
		request.System = prepared.Prompt
		think := false
		request.Think = &think
		return request, nil
	case exactPreparedTransportNativePrompt:
		prompt, err := ExactPreparedRequestModelInput(prepared)
		if err != nil {
			return exactPreparedRequest{}, err
		}
		request.Prompt = prompt
		return request, nil
	default:
		return exactPreparedRequest{}, fmt.Errorf(
			"exact provider model profile %q has no registered transport",
			profile.tokenizerProfile,
		)
	}
}

func (profile exactProviderModelProfile) rawPreparedRequest(
	prepared PreparedModel,
	request exactPreparedRequest,
) (exactPreparedRequest, error) {
	prompt, err := ExactPreparedRequestModelInput(prepared)
	if err != nil {
		return exactPreparedRequest{}, err
	}
	request.Prompt = prompt
	request.Raw = true
	think := false
	request.Think = &think
	if prepared.RawTextStopSequence != "" {
		request.Options.Stop = []string{prepared.RawTextStopSequence}
	}
	return request, nil
}

func (profile exactProviderModelProfile) noThinkingPreparedRequest(
	prepared PreparedModel,
	request exactPreparedRequest,
) (exactPreparedRequest, error) {
	prompt, err := ExactPreparedRequestModelInput(prepared)
	if err != nil {
		return exactPreparedRequest{}, err
	}
	request.Prompt = prompt
	think := false
	request.Think = &think
	return request, nil
}
