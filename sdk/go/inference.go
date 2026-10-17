package omnidex

import (
	"context"
	"fmt"
	"net/http"
	"strings"
)

// InferenceInput contains one bounded semantic input. Question is the primary
// input; Context is supporting data. An explicitly configured native provider
// may define a model-specific input contract. No channel or history is created.
type InferenceInput struct {
	Question        string `json:"question"`
	Context         string `json:"context"`
	MaxOutputTokens int    `json:"max_output_tokens"`
}

type InferenceResult struct {
	Text       string `json:"text"`
	Model      string `json:"model"`
	DurationMS int64  `json:"duration_ms"`
}

func (client *Client) Infer(ctx context.Context, input InferenceInput) (InferenceResult, error) {
	if strings.TrimSpace(input.Question) == "" || len(input.Question) > 4096 || len(input.Context) > 12000 || input.MaxOutputTokens < 1 || input.MaxOutputTokens > 1024 {
		return InferenceResult{}, fmt.Errorf("Omnidex inference request is outside its bounds")
	}
	var result InferenceResult
	if err := client.request(ctx, http.MethodPost, "/v1/integrations/infer", nil, input, http.StatusOK, &result); err != nil {
		return result, err
	}
	if strings.TrimSpace(result.Text) == "" || len(result.Text) > 16384 || result.Model == "" || result.DurationMS < 0 {
		return InferenceResult{}, fmt.Errorf("Omnidex returned an invalid inference result")
	}
	return result, nil
}
