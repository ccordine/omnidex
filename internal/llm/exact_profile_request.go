package llm

import (
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
)

type exactPreparedRequestOptions struct {
	NumCtx      int                       `json:"num_ctx"`
	NumPredict  int                       `json:"num_predict,omitempty"`
	Stop        []string                  `json:"stop,omitempty"`
	Temperature *ExactPreparedTemperature `json:"temperature,omitempty"`
}

type exactPreparedRequest struct {
	Format   map[string]any              `json:"format,omitempty"`
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
	if prepared.OutputLimitMode == ExactPreparedOutputLimitExplicit {
		request.Options.NumPredict = prepared.MaxOutputTokens
	}
	request.Options.Temperature = prepared.Temperature
	if prepared.Protocol == ExactPreparedProtocolStructuredV1 {
		request.Format = prepared.ResponseSchema
	}

	switch profile.transport {
	case exactPreparedTransportRaw:
		return profile.rawPreparedRequest(prepared, request)
	case exactPreparedTransportNativeThinking:
		return profile.thinkingPreparedRequest(prepared, request)
	case exactPreparedTransportNativeSystem:
		request.Prompt = prepared.PromptHint
		request.System = prepared.Prompt
		return request, nil
	case exactPreparedTransportNativeSystemThinking:
		request.Prompt = prepared.PromptHint
		request.System = prepared.Prompt
		think := true
		request.Think = &think
		return request, nil
	case exactPreparedTransportNativePrompt:
		prompt, err := ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
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
	prompt, err := ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
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

func (profile exactProviderModelProfile) thinkingPreparedRequest(
	prepared PreparedModel,
	request exactPreparedRequest,
) (exactPreparedRequest, error) {
	prompt, err := ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
	if err != nil {
		return exactPreparedRequest{}, err
	}
	request.Prompt = prompt
	think := true
	request.Think = &think
	return request, nil
}
