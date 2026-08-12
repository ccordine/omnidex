package qwenselector

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionreference"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

const providerChallengeScope = "cognition-reference-qwen-selector.v1:"

const (
	maxSelectionOutputTokens  = 64
	maxSelectionResponseBytes = 256
)

var ErrSelection = errors.New("Qwen semantic candidate selection failed")

type Limits struct {
	MaxInputTokens  int
	MaxOutputTokens int
}

type Selector struct {
	client   llm.ExactPreparedContractClient
	provider llm.ObservedProviderIdentity
	expected llm.ProviderIdentityExpectation
	limits   Limits
}

func New(
	client llm.ExactPreparedContractClient,
	provider llm.ObservedProviderIdentity,
	limits Limits,
) (*Selector, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: exact client is required", ErrSelection)
	}
	provider.Evidence = provider.Evidence.Clone()
	selection := llm.ProviderIdentitySelection{
		Model: provider.Attestation.Model, NativeContextLimit: provider.Attestation.NativeContextLimit,
	}
	expected, err := llm.DeriveExactProviderIdentityExpectation(provider.Evidence, selection)
	if err != nil {
		return nil, fmt.Errorf("%w: derive held provider identity: %v", ErrSelection, err)
	}
	request := llm.ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: provider.Observation.ChallengeSHA256,
	}
	if err := provider.ValidateFor(request); err != nil {
		return nil, fmt.Errorf("%w: held provider identity: %v", ErrSelection, err)
	}
	if limits.MaxInputTokens <= 0 || limits.MaxOutputTokens <= 0 ||
		limits.MaxOutputTokens > maxSelectionOutputTokens ||
		limits.MaxInputTokens+limits.MaxOutputTokens > expected.NativeContextLimit {
		return nil, fmt.Errorf("%w: invalid exact token limits", ErrSelection)
	}
	if err := client.RequireExactPreparedContract(); err != nil {
		return nil, fmt.Errorf("%w: exact provider contract: %v", ErrSelection, err)
	}
	if err := client.ValidateExactPreparedProvider(expected); err != nil {
		return nil, fmt.Errorf("%w: exact provider identity: %v", ErrSelection, err)
	}
	return &Selector{client: client, provider: provider, expected: expected, limits: limits}, nil
}

func (selector *Selector) Select(
	ctx context.Context,
	gap cognitionreference.SemanticGap,
) (cognitionreference.CandidateID, error) {
	if ctx == nil || selector == nil || selector.client == nil {
		return "", fmt.Errorf("%w: initialized selector and context are required", ErrSelection)
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	gap = gap.Clone()
	prompt, err := renderEnvelope(gap)
	if err != nil {
		return "", err
	}
	prepared, requestBytes, requestSHA, err := selector.prepare(gap, prompt)
	if err != nil {
		return "", err
	}
	generation, generateErr := selector.client.GeneratePreparedExact(ctx, prepared)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(generation.Content) > maxSelectionResponseBytes {
		return "", fmt.Errorf("%w: response exceeds %d bytes", ErrSelection, maxSelectionResponseBytes)
	}
	owned, ownershipErr := llm.OwnBoundedPreparedGeneration(generation)
	if ownershipErr != nil {
		return "", fmt.Errorf("%w: own exact generation: %v", ErrSelection, ownershipErr)
	}
	afterBytes, renderErr := llm.ExactPreparedRequestBytes(prepared)
	if renderErr != nil || !bytes.Equal(afterBytes, requestBytes) {
		return "", fmt.Errorf("%w: client mutated the prepared request", ErrSelection)
	}
	if generateErr != nil {
		return "", fmt.Errorf("%w: exact generation: %w", ErrSelection, generateErr)
	}
	if err := selector.validateGeneration(owned, prepared, requestSHA); err != nil {
		return "", err
	}
	selected, err := decodeSelection(owned.Content)
	if err != nil {
		return "", err
	}
	if err := gap.ValidateSelection(selected); err != nil {
		return "", err
	}
	return selected, nil
}

func (selector *Selector) prepare(
	gap cognitionreference.SemanticGap,
	prompt string,
) (llm.PreparedModel, []byte, string, error) {
	if err := validateExpressibleSelections(gap, selector.limits.MaxOutputTokens); err != nil {
		return llm.PreparedModel{}, nil, "", err
	}
	zero := 0.0
	expected := selector.expected
	prepared := llm.PreparedModel{
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: prompt, PromptHint: llm.MinimalGeneratePrompt,
		MaxOutputTokens: selector.limits.MaxOutputTokens, ContextTokens: expected.NativeContextLimit,
		ResponseFormat: llm.ResponseFormatJSON, ResponseSchema: responseSchema(gap),
		ThinkingEnabled: false, Temperature: &zero,
		ProviderIdentityExpectation: &expected,
	}
	challenge, err := selector.observationChallenge(gap, prepared)
	if err != nil {
		return llm.PreparedModel{}, nil, "", err
	}
	prepared.ProviderObservationChallenge = challenge
	input, err := llm.ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
	if err != nil {
		return llm.PreparedModel{}, nil, "", err
	}
	if err := llm.ValidateExactPreparedInputBudget(
		prepared.ContextTokens, selector.limits.MaxInputTokens, prepared.MaxOutputTokens,
		input, llm.MaxRawInputSpecialTokenReserve,
	); err != nil {
		return llm.PreparedModel{}, nil, "", fmt.Errorf("%w: %v", ErrSelection, err)
	}
	intendedRaw, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		return llm.PreparedModel{}, nil, "", err
	}
	if err := selector.client.ValidateExactPreparedContract(prepared); err != nil {
		return llm.PreparedModel{}, nil, "", fmt.Errorf("%w: provider rejected exact request: %v", ErrSelection, err)
	}
	afterValidation, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil || !bytes.Equal(afterValidation, intendedRaw) ||
		prepared.ProviderIdentityExpectation == nil ||
		*prepared.ProviderIdentityExpectation != selector.expected ||
		prepared.ProviderObservationChallenge != challenge {
		return llm.PreparedModel{}, nil, "", fmt.Errorf(
			"%w: provider validation mutated the exact request", ErrSelection,
		)
	}
	requestDigest := sha256.Sum256(intendedRaw)
	return prepared, intendedRaw, hex.EncodeToString(requestDigest[:]), nil
}

func (selector *Selector) observationChallenge(
	gap cognitionreference.SemanticGap,
	prepared llm.PreparedModel,
) (string, error) {
	binding, err := exactjson.Canonical(struct {
		GapID           cognitionreference.GapID       `json:"gap_id"`
		GapKind         cognitionreference.GapKind     `json:"gap_kind"`
		ObjectiveID     cognitionreference.ObjectiveID `json:"objective_id"`
		Prompt          string                         `json:"prompt"`
		ResponseSchema  map[string]any                 `json:"response_schema"`
		Model           string                         `json:"model"`
		ContextTokens   int                            `json:"context_tokens"`
		MaxInputTokens  int                            `json:"max_input_tokens"`
		MaxOutputTokens int                            `json:"max_output_tokens"`
	}{
		GapID: gap.ID, GapKind: gap.Kind, ObjectiveID: gap.ObjectiveID,
		Prompt: prepared.Prompt, ResponseSchema: prepared.ResponseSchema,
		Model: prepared.ContextModel, ContextTokens: prepared.ContextTokens,
		MaxInputTokens: selector.limits.MaxInputTokens, MaxOutputTokens: prepared.MaxOutputTokens,
	})
	if err != nil {
		return "", fmt.Errorf("%w: bind exact selector request: %v", ErrSelection, err)
	}
	digest := sha256.Sum256(binding)
	return llm.DeriveProviderIdentityObservationChallenge(
		providerChallengeScope+hex.EncodeToString(digest[:]), selector.expected,
	)
}

func (selector *Selector) validateGeneration(
	generation llm.PreparedGeneration,
	prepared llm.PreparedModel,
	requestSHA string,
) error {
	if err := generation.Validate(); err != nil {
		return fmt.Errorf("%w: invalid exact generation: %v", ErrSelection, err)
	}
	if generation.ProviderRequestSHA256 != requestSHA ||
		generation.ProviderResponseModel != selector.expected.Model ||
		generation.ProviderDoneReason != "stop" ||
		generation.Usage.PromptEvalCount > selector.limits.MaxInputTokens ||
		generation.Usage.EvalCount > selector.limits.MaxOutputTokens {
		return fmt.Errorf("%w: provider result differs from the held request", ErrSelection)
	}
	observed := llm.ObservedProviderIdentity{
		Attestation: selector.provider.Attestation,
		Observation: generation.ProviderObservation,
		Evidence:    generation.ProviderIdentityEvidence,
	}
	request := llm.ProviderIdentityObservationRequest{
		Expectation: selector.expected, ChallengeSHA256: prepared.ProviderObservationChallenge,
	}
	if err := observed.ValidateFor(request); err != nil {
		return fmt.Errorf("%w: generation provider identity: %v", ErrSelection, err)
	}
	return nil
}

type selectionResponse struct {
	CandidateID cognitionreference.CandidateID `json:"candidate_id"`
}

func validateExpressibleSelections(
	gap cognitionreference.SemanticGap,
	maxOutputTokens int,
) error {
	for _, candidate := range gap.Candidates {
		raw, err := exactjson.Canonical(selectionResponse{CandidateID: candidate.ID})
		if err != nil {
			return fmt.Errorf("%w: render candidate %q: %v", ErrSelection, candidate.ID, err)
		}
		if len(raw) > maxSelectionResponseBytes || len(raw) > maxOutputTokens {
			return fmt.Errorf(
				"%w: candidate %q cannot fit exact output authority", ErrSelection, candidate.ID,
			)
		}
	}
	return nil
}

func decodeSelection(content string) (cognitionreference.CandidateID, error) {
	raw := []byte(content)
	if err := exactjson.ValidateObject(raw, selectionResponse{}, "semantic candidate response"); err != nil {
		return "", fmt.Errorf("%w: response shape: %v", ErrSelection, err)
	}
	var response selectionResponse
	if err := json.Unmarshal(raw, &response); err != nil || response.CandidateID == "" {
		return "", fmt.Errorf("%w: response candidate ID is empty", ErrSelection)
	}
	return response.CandidateID, nil
}
