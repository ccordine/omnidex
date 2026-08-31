package llm

import (
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
		Model: prepared.ContextModel,
		Options: exactPreparedRequestOptions{
			NumCtx: prepared.ContextTokens, NumPredict: prepared.MaxOutputTokens,
			Temperature: prepared.Temperature,
		},
		Prompt: prompt, Raw: false, Shift: false, Stream: false,
		Think: &think, Truncate: false,
	}
	if prepared.RawTextStopSequence != "" {
		request.Options.Stop = []string{prepared.RawTextStopSequence}
	}
	return exactjson.Canonical(request)
}
