package llm

import (
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
)

// MaxExactPreparedProviderRequestBytes allows the bounded semantic prompt,
// its JSON escaping and wrapper, and the retained native token IDs.
const MaxExactPreparedProviderRequestBytes = 1024*1024 +
	maxExactPreparedJSONBytesPerContextID*MaxInferenceContextTokens

type exactPreparedRequestOptions struct {
	NumCtx      int                       `json:"num_ctx"`
	NumPredict  int                       `json:"num_predict"`
	Stop        []string                  `json:"stop,omitempty"`
	Temperature *ExactPreparedTemperature `json:"temperature,omitempty"`
}

type exactPreparedRequest struct {
	Context  []int                       `json:"context,omitempty"`
	Model    string                      `json:"model"`
	Options  exactPreparedRequestOptions `json:"options"`
	Prompt   string                      `json:"prompt"`
	Raw      bool                        `json:"raw"`
	Shift    bool                        `json:"shift"`
	Stream   bool                        `json:"stream"`
	Think    *bool                       `json:"think,omitempty"`
	Truncate bool                        `json:"truncate"`
}

// ExactPreparedRequestBytes renders the provider adapter's one consumed
// /api/generate request. Runtime admission does not depend on provider model
// identity, tokenizer metadata, installed tags, or backend version probes.
func ExactPreparedRequestBytes(prepared PreparedModel) ([]byte, error) {
	if err := validateExactPreparedRequest(prepared); err != nil {
		return nil, err
	}
	prompt, err := ExactPreparedRequestModelInput(prepared)
	if err != nil {
		return nil, err
	}
	think := false
	request := exactPreparedRequest{
		Context: prepared.RetainedContext,
		Model:   prepared.ContextModel,
		Options: exactPreparedRequestOptions{
			NumCtx:      prepared.ContextTokens,
			Temperature: prepared.Temperature,
		},
		Prompt: prompt, Raw: false, Shift: false, Stream: false,
		Think: &think, Truncate: false,
	}
	request.Options.NumPredict = prepared.MaxOutputTokens
	if prepared.RawTextStopSequence != "" {
		request.Options.Stop = []string{prepared.RawTextStopSequence}
	}
	raw, err := exactjson.Canonical(request)
	if err != nil {
		return nil, err
	}
	if len(raw) > MaxExactPreparedProviderRequestBytes {
		return nil, fmt.Errorf("exact provider request exceeds its byte ceiling")
	}
	return raw, nil
}
