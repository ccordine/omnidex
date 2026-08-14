package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
)

type objectiveAdvisoryProviderTestClient struct {
	discoveries int
	generations int
	selection   llm.ProviderIdentitySelection
	prepared    llm.PreparedModel
	callError   error
}

func (*objectiveAdvisoryProviderTestClient) RequireExactPreparedContract() error { return nil }

func (*objectiveAdvisoryProviderTestClient) ValidateExactPreparedProvider(
	expected llm.ProviderIdentityExpectation,
) error {
	return llm.ValidateExactPreparedProviderExpectation(expected)
}

func (*objectiveAdvisoryProviderTestClient) ValidateExactPreparedContract(
	prepared llm.PreparedModel,
) error {
	_, err := llm.ExactPreparedRequestBytes(prepared)
	return err
}

func (client *objectiveAdvisoryProviderTestClient) DiscoverProviderIdentityEvidence(
	_ context.Context,
	selection llm.ProviderIdentitySelection,
	challenge string,
) (llm.ObservedProviderIdentity, error) {
	client.discoveries++
	client.selection = selection
	return desiredStateProductObservedIdentity(selection, challenge)
}

func (client *objectiveAdvisoryProviderTestClient) GeneratePreparedExact(
	_ context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	client.generations++
	client.prepared = prepared
	if client.callError != nil {
		return llm.PreparedGeneration{}, client.callError
	}
	return desiredStateProductGeneration(prepared, "Check the boundary condition before accepting the answer.")
}

func TestExactObjectiveAdvisoryProviderUsesRawTextAndOwnsIdentityUsage(t *testing.T) {
	client := &objectiveAdvisoryProviderTestClient{}
	provider := exactObjectiveAdvisoryProvider{
		client: client, contextTokens: 32768,
	}
	request := validObjectiveAdvisoryGenerateRequest(t)

	generation, err := provider.Generate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	wantPrompt, err := objectiveadvisory.BuildPrompt(request)
	if err != nil {
		t.Fatal(err)
	}
	if client.discoveries != 1 || client.generations != 1 {
		t.Fatalf("provider calls discovery=%d generation=%d", client.discoveries, client.generations)
	}
	if client.selection.Model != request.Source.Model || client.selection.NativeContextLimit != 32768 {
		t.Fatalf("identity selection=%+v", client.selection)
	}
	prepared := client.prepared
	if prepared.Protocol != llm.ExactPreparedProtocolRawTextV1 ||
		prepared.Prompt != wantPrompt || prepared.ResponseFormat != "" ||
		prepared.ResponseSchema != nil || prepared.ThinkingEnabled ||
		prepared.RawTextStopSequence != llm.ExactPreparedObjectiveAdvisoryStopV1 ||
		prepared.Temperature == nil || *prepared.Temperature != 0 ||
		prepared.MaxOutputTokens != request.Source.Budget.MaxOutputTokens {
		t.Fatalf("prepared advisory request violated raw protocol: %+v", prepared)
	}
	if generation.FinalText == "" || generation.EffectiveProvider != llm.ExactPreparedProviderBackend ||
		generation.EffectiveModel != request.Source.Model ||
		generation.ModelDigest != strings.Repeat("7", 64) || generation.Quantization != "Q4_K_M" ||
		generation.PromptTokens != 1 || generation.OutputTokens != 1 ||
		generation.Duration != 10 || generation.FinishReason != "stop" {
		t.Fatalf("owned advisory generation=%+v", generation)
	}
}

func TestExactObjectiveAdvisoryProviderStopMatchesPromptTerminator(t *testing.T) {
	request := validObjectiveAdvisoryGenerateRequest(t)
	prompt, err := objectiveadvisory.BuildPrompt(request)
	if err != nil {
		t.Fatal(err)
	}
	marker := strings.TrimPrefix(llm.ExactPreparedObjectiveAdvisoryStopV1, "\n")
	if marker == llm.ExactPreparedObjectiveAdvisoryStopV1 ||
		strings.Count(prompt, marker) != 1 {
		t.Fatalf("prompt does not request the exact provider terminator: %q", prompt)
	}
}

func TestExactObjectiveAdvisoryProviderRejectsNonExactSamplingBeforeTransport(t *testing.T) {
	for _, mutate := range []func(*objectiveadvisory.GenerateRequest){
		func(request *objectiveadvisory.GenerateRequest) { request.Source.Sampling.Temperature = 0.1 },
		func(request *objectiveadvisory.GenerateRequest) {
			value := 0.9
			request.Source.Sampling.TopP = &value
		},
		func(request *objectiveadvisory.GenerateRequest) {
			value := int64(7)
			request.Source.Sampling.Seed = &value
		},
		func(request *objectiveadvisory.GenerateRequest) { request.Source.Provider = "openai" },
	} {
		client := &objectiveAdvisoryProviderTestClient{}
		request := validObjectiveAdvisoryGenerateRequest(t)
		mutate(&request)
		_, err := (exactObjectiveAdvisoryProvider{
			client: client, contextTokens: 32768,
		}).Generate(context.Background(), request)
		if err == nil {
			t.Fatal("non-exact advisory request was accepted")
		}
		if client.discoveries != 0 || client.generations != 0 {
			t.Fatalf("invalid request reached transport: %+v", client)
		}
	}
}

func TestExactObjectiveAdvisoryProviderPreservesProviderFailure(t *testing.T) {
	client := &objectiveAdvisoryProviderTestClient{callError: errors.New("provider unavailable")}
	_, err := (exactObjectiveAdvisoryProvider{
		client: client, contextTokens: 32768,
	}).Generate(context.Background(), validObjectiveAdvisoryGenerateRequest(t))
	if err == nil || !strings.Contains(err.Error(), "provider unavailable") {
		t.Fatalf("provider failure=%v", err)
	}
}

func validObjectiveAdvisoryGenerateRequest(t *testing.T) objectiveadvisory.GenerateRequest {
	t.Helper()
	summary := "internal/worker/objective_turn_workflow.go calls the grounded closure."
	digest := sha256.Sum256([]byte(summary))
	projection, err := objectiveadvisory.BuildProjection(objectiveadvisory.ProjectionInput{
		ObjectiveID: "objective:test", Generation: 1,
		Objective: "Review the grounded answer before it is returned.",
		UserAuthorities: []objectiveadvisory.TextAuthority{
			{ID: "current-user-objective", Content: "Review the grounded answer before it is returned."},
		},
		Constraints: []string{}, Decisions: []string{}, Invariants: []string{},
		UnresolvedQuestions: []string{},
		GroundedEvidence: []objectiveadvisory.EvidenceSummary{
			{ID: "evidence-1", Summary: summary, SHA256: hex.EncodeToString(digest[:])},
		},
		UsefulAdvice: "Identify risks and verification ideas relevant to the candidate.",
	})
	if err != nil {
		t.Fatal(err)
	}
	return objectiveadvisory.GenerateRequest{
		TriggerID:      objectiveadvisory.TriggerPostGroundingObjective,
		TriggerVersion: objectiveadvisory.TriggerVersionV1,
		Projection:     projection,
		Source: objectiveadvisory.SourceConfig{
			ID: "objective-advisory-primary", Provider: llm.ExactPreparedProviderBackend,
			Model:    "qwen3.5:9b-q4_K_M",
			Sampling: objectiveadvisory.SamplingConfig{Temperature: 0},
			Budget: objectiveadvisory.Budget{
				MaxInputBytes: 28 * 1024, MaxOutputBytes: 8 * 1024,
				MaxOutputTokens: 2048,
			},
		},
	}
}
