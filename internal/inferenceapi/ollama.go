package inferenceapi

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
)

type ollamaProvider struct {
	client *ollama.Client
	model  string
}

func NewOllamaProvider(client *ollama.Client, model string) Provider {
	return &ollamaProvider{client: client, model: model}
}

func (p *ollamaProvider) Check(ctx context.Context) error {
	if p.client == nil {
		return fmt.Errorf("missing Ollama client")
	}
	installed, err := p.client.HasModel(ctx, p.model)
	if err != nil || !installed {
		return fmt.Errorf("configured local inference model is unavailable")
	}
	return nil
}

func (p *ollamaProvider) Predict(ctx context.Context, input Request) (string, error) {
	if p.client == nil {
		return "", fmt.Errorf("missing Ollama client")
	}
	temperature := llm.ExactPreparedTemperature(0)
	prepared := llm.PreparedModel{
		Protocol:  llm.ExactPreparedProtocolPlainCompletionV4,
		BaseModel: p.model, ContextModel: p.model, ContextTokens: 8192,
		Prompt:          "Question:\n" + input.Question + "\n\nContext:\n" + input.Context + "\n\nReturn only the requested semantic result.",
		MaxOutputTokens: input.MaxOutputTokens, OutputLimitMode: llm.ExactPreparedOutputLimitExplicit,
		Temperature: &temperature,
	}
	result, err := p.client.GeneratePreparedExact(ctx, prepared)
	if err == nil {
		err = llm.ValidateExactPreparedGenerationForRequest(prepared, result)
	}
	return result.Content, err
}
