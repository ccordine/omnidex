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

func (c *Client) GeneratePreparedExact(
	ctx context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	if err := c.ValidateExactPreparedContract(prepared); err != nil {
		return llm.PreparedGeneration{}, err
	}
	observed, err := c.ObserveProviderIdentity(ctx, llm.ProviderIdentityObservationRequest{
		Expectation:     *prepared.ProviderIdentityExpectation,
		ChallengeSHA256: prepared.ProviderObservationChallenge,
	})
	if err != nil {
		return llm.PreparedGeneration{
			Schema:                   llm.PreparedGenerationSchemaV1,
			ProviderObservation:      observed.Observation,
			ProviderIdentityEvidence: observed.Evidence,
		}, fmt.Errorf("observe exact cognition provider identity: %w", err)
	}
	result, generationErr := c.generatePreparedRaw(ctx, prepared, observed)
	if generationErr != nil {
		return result, generationErr
	}
	return result, result.Validate()
}

func (c *Client) CleanupPreparedModel(llm.PreparedModel) {}

func (c *Client) RequireExactPreparedContract() error {
	if c == nil || c.contextTokens <= 0 {
		return fmt.Errorf("ollama exact prepared contract requires an initialized client")
	}
	return nil
}

func (c *Client) ValidateExactPreparedProvider(expected llm.ProviderIdentityExpectation) error {
	return llm.ValidateExactPreparedProviderExpectation(expected)
}

func (c *Client) ValidateExactPreparedContract(prepared llm.PreparedModel) error {
	if err := c.RequireExactPreparedContract(); err != nil {
		return err
	}
	if prepared.BaseModel == "" || prepared.ContextModel != prepared.BaseModel ||
		prepared.Prompt == "" || prepared.PromptHint != llm.MinimalGeneratePrompt ||
		prepared.MaxOutputTokens <= 0 || prepared.ContextTokens <= 0 ||
		prepared.ResponseFormat != llm.ResponseFormatJSON || len(prepared.ResponseSchema) == 0 ||
		prepared.ThinkingEnabled || prepared.Temperature == nil || *prepared.Temperature != 0 ||
		prepared.ProviderIdentityExpectation == nil || prepared.ProviderObservationChallenge == "" {
		return fmt.Errorf("ollama prepared request does not satisfy the exact structured cognition contract")
	}
	if err := prepared.ProviderIdentityExpectation.Validate(); err != nil ||
		prepared.ProviderIdentityExpectation.Backend != ollamaProviderBackend ||
		prepared.ProviderIdentityExpectation.Model != prepared.BaseModel ||
		prepared.ProviderIdentityExpectation.NativeContextLimit != prepared.ContextTokens {
		return fmt.Errorf("ollama prepared request changed its frozen provider identity")
	}
	if err := c.ValidateExactPreparedProvider(*prepared.ProviderIdentityExpectation); err != nil {
		return err
	}
	if err := (llm.ProviderIdentityObservationRequest{
		Expectation:     *prepared.ProviderIdentityExpectation,
		ChallengeSHA256: prepared.ProviderObservationChallenge,
	}).Validate(); err != nil {
		return fmt.Errorf("ollama prepared request has an invalid provider observation challenge")
	}
	if err := llm.ValidateResponseContract(prepared); err != nil {
		return err
	}
	rawInput, err := llm.ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
	if err != nil {
		return err
	}
	return llm.ValidateExactPreparedInputBudget(
		prepared.ContextTokens,
		prepared.ContextTokens-prepared.MaxOutputTokens,
		prepared.MaxOutputTokens,
		rawInput,
		llm.MaxRawInputSpecialTokenReserve,
	)
}
