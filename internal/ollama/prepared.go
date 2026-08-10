package ollama

import (
	"context"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/llm"
)

func (c *Client) Generate(ctx context.Context, model, prompt string) (string, error) {
	prepared, err := c.PrepareContextModel(ctx, model, prompt)
	if err != nil {
		return "", err
	}
	defer c.CleanupPreparedModel(prepared)
	return c.GeneratePrepared(ctx, prepared)
}

func (c *Client) PrepareContextModel(_ context.Context, model, prompt string) (llm.PreparedModel, error) {
	if strings.TrimSpace(model) == "" {
		model = c.defaultModel
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return llm.PreparedModel{}, fmt.Errorf("model is required")
	}
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		prompt = "(empty prompt)"
	}
	return llm.PreparedModel{
		BaseModel: model, ContextModel: model,
		PromptHint: llm.DerivePreparedModelPromptHint(prompt),
		Prompt:     prompt, ContextTokens: c.contextTokens,
	}, nil
}

func (c *Client) GeneratePrepared(ctx context.Context, prepared llm.PreparedModel) (string, error) {
	model := strings.TrimSpace(prepared.ContextModel)
	if model == "" {
		model = strings.TrimSpace(prepared.BaseModel)
	}
	if model == "" {
		model = c.defaultModel
	}
	if strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("model is required")
	}
	system := strings.TrimSpace(prepared.Prompt)
	if system == "" {
		system = "(empty prompt)"
	}
	promptHint := strings.TrimSpace(prepared.PromptHint)
	if promptHint == "" {
		promptHint = llm.MinimalGeneratePrompt
	}
	contextTokens := prepared.ContextTokens
	if contextTokens == 0 {
		contextTokens = c.contextTokens
	}
	return c.chat(
		ctx, model, system, promptHint, prepared.MaxOutputTokens, contextTokens,
		prepared.ResponseFormat, prepared.ResponseSchema,
		prepared.ThinkingEnabled, prepared.Temperature,
	)
}

func (c *Client) CleanupPreparedModel(llm.PreparedModel) {}

func (c *Client) RequireExactPreparedContract() error {
	if c == nil || c.contextTokens <= 0 {
		return fmt.Errorf("ollama exact prepared contract requires an initialized client")
	}
	return nil
}

func (c *Client) ValidateExactPreparedContract(prepared llm.PreparedModel) error {
	if err := c.RequireExactPreparedContract(); err != nil {
		return err
	}
	if prepared.BaseModel == "" || prepared.ContextModel != prepared.BaseModel ||
		prepared.Prompt == "" || prepared.PromptHint == "" ||
		prepared.MaxOutputTokens <= 0 || prepared.ContextTokens <= 0 ||
		prepared.ResponseFormat != llm.ResponseFormatJSON || len(prepared.ResponseSchema) == 0 ||
		prepared.ThinkingEnabled || prepared.Temperature == nil || *prepared.Temperature != 0 {
		return fmt.Errorf("ollama prepared request does not satisfy the exact structured cognition contract")
	}
	if err := llm.ValidateResponseContract(prepared); err != nil {
		return err
	}
	return llm.ValidateInferenceBudget(
		prepared.ContextTokens, prepared.MaxOutputTokens, prepared.Prompt, prepared.PromptHint,
	)
}
