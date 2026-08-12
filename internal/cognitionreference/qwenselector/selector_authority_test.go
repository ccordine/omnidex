package qwenselector

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionreference"
	"github.com/gryph/omnidex/internal/llm"
)

func TestSelectorPropagatesExactProviderFailureWithoutFallback(t *testing.T) {
	t.Parallel()
	want := errors.New("provider failed")
	client, selector := testSelector(t, `{"candidate_id":"C17"}`, Limits{3000, 64})
	client.err = want
	selected, err := selector.Select(t.Context(), testGap())
	if !errors.Is(err, want) || selected != "" || client.calls != 1 {
		t.Fatalf("selection=%q error=%v calls=%d, want exact failure after one call", selected, err, client.calls)
	}
}

func TestSelectorRejectsEveryCorruptHeldGenerationAuthority(t *testing.T) {
	t.Parallel()
	for name, mutate := range map[string]func(*llm.PreparedGeneration){
		"request SHA": func(generation *llm.PreparedGeneration) {
			generation.ProviderRequestSHA256 = strings.Repeat("b", 64)
		},
		"response model": func(generation *llm.PreparedGeneration) {
			generation.ProviderResponseModel = "other-model"
		},
		"length stop": func(generation *llm.PreparedGeneration) {
			generation.ProviderDoneReason = "length"
		},
		"observation challenge": func(generation *llm.PreparedGeneration) {
			generation.ProviderObservation.ChallengeSHA256 = strings.Repeat("b", 64)
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client, selector := testSelector(t, `{"candidate_id":"C17"}`, Limits{3000, 64})
			client.mutateResult = mutate
			selected, err := selector.Select(t.Context(), testGap())
			if !errors.Is(err, ErrSelection) || selected != "" || client.calls != 1 {
				t.Fatalf("selection=%q error=%v calls=%d, want corrupt authority rejection", selected, err, client.calls)
			}
		})
	}
}

func TestSelectorDiscardsValidGenerationWhenContextCancelsDuringCall(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	client, selector := testSelector(t, `{"candidate_id":"C17"}`, Limits{3000, 64})
	client.beforeReturn = cancel
	selected, err := selector.Select(ctx, testGap())
	if !errors.Is(err, context.Canceled) || selected != "" || client.calls != 1 {
		t.Fatalf("selection=%q error=%v calls=%d, want canceled valid generation discarded", selected, err, client.calls)
	}
}

func TestSelectorRejectsCallerOutputCeilingAboveStationMaximum(t *testing.T) {
	t.Parallel()
	provider := testProvider(t)
	client := &recordingExactClient{provider: provider, content: `{"candidate_id":"C17"}`}
	if _, err := New(client, provider, Limits{3000, maxSelectionOutputTokens + 1}); !errors.Is(err, ErrSelection) {
		t.Fatalf("New() error=%v, want fixed station output ceiling rejection", err)
	}
	if client.calls != 0 {
		t.Fatalf("invalid output ceiling generated %d times", client.calls)
	}
}

func TestSelectorRejectsOversizedContentBeforeDecoding(t *testing.T) {
	t.Parallel()
	content := `{"candidate_id":"C17","padding":"` + strings.Repeat("x", maxSelectionResponseBytes) + `"}`
	client, selector := testSelector(t, content, Limits{3000, 64})
	selected, err := selector.Select(t.Context(), testGap())
	if !errors.Is(err, ErrSelection) || selected != "" || client.calls != 1 {
		t.Fatalf("selection=%q error=%v calls=%d, want bounded content rejection", selected, err, client.calls)
	}
}

func TestSelectorRejectsSchemaCandidateThatCannotFitOutputAuthorityBeforeDispatch(t *testing.T) {
	t.Parallel()
	gap := testGap()
	gap.Candidates[0].ID = cognitionreference.CandidateID("C" + strings.Repeat(`"`, 60))
	client, selector := testSelector(t, `{"candidate_id":"C23"}`, Limits{3000, 64})
	selected, err := selector.Select(t.Context(), gap)
	if !errors.Is(err, ErrSelection) || selected != "" || client.calls != 0 {
		t.Fatalf("selection=%q error=%v calls=%d, want predispatch expressibility rejection", selected, err, client.calls)
	}
}

func TestSelectorRejectsPreparedRequestAliasMutation(t *testing.T) {
	t.Parallel()
	client, selector := testSelector(t, `{"candidate_id":"C17"}`, Limits{3000, 64})
	client.mutate = func(prepared *llm.PreparedModel) {
		prepared.ResponseSchema["type"] = "array"
	}
	selected, err := selector.Select(t.Context(), testGap())
	if !errors.Is(err, ErrSelection) || selected != "" || client.calls != 1 {
		t.Fatalf("selection=%q error=%v calls=%d, want mutated-request rejection", selected, err, client.calls)
	}
}

func TestSelectorRejectsValidationTimeSchemaAliasMutationBeforeDispatch(t *testing.T) {
	t.Parallel()
	client, selector := testSelector(t, `{"candidate_id":"C17"}`, Limits{3000, 64})
	client.validateMutate = func(prepared *llm.PreparedModel) {
		prepared.ResponseSchema["additionalProperties"] = true
	}
	selected, err := selector.Select(t.Context(), testGap())
	if !errors.Is(err, ErrSelection) || selected != "" || client.calls != 0 {
		t.Fatalf("selection=%q error=%v calls=%d, want predispatch alias rejection", selected, err, client.calls)
	}
}

func TestSelectorRejectsNativeUsageOutsideBothFrozenCeilings(t *testing.T) {
	t.Parallel()
	for name, configure := range map[string]func(*recordingExactClient){
		"input":  func(client *recordingExactClient) { client.promptTokens = 3001 },
		"output": func(client *recordingExactClient) { client.outputTokens = 65 },
	} {
		name, configure := name, configure
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			client, selector := testSelector(t, `{"candidate_id":"C17"}`, Limits{3000, 64})
			configure(client)
			selected, err := selector.Select(t.Context(), testGap())
			if !errors.Is(err, ErrSelection) || selected != "" || client.calls != 1 {
				t.Fatalf("selection=%q error=%v calls=%d, want native usage rejection", selected, err, client.calls)
			}
		})
	}
}

func TestObservationChallengeBindsEveryExactCallAuthority(t *testing.T) {
	t.Parallel()
	client, selector := testSelector(t, `{"candidate_id":"C17"}`, Limits{3000, 64})
	if _, err := selector.Select(t.Context(), testGap()); err != nil {
		t.Fatal(err)
	}
	base := client.prepared
	baseGap := testGap()
	baseChallenge := base.ProviderObservationChallenge
	mutations := map[string]func(*Selector, *cognitionreference.SemanticGap, *llm.PreparedModel){
		"gap ID": func(_ *Selector, gap *cognitionreference.SemanticGap, _ *llm.PreparedModel) {
			gap.ID = "gap.other"
		},
		"gap kind": func(_ *Selector, gap *cognitionreference.SemanticGap, _ *llm.PreparedModel) {
			gap.Kind = "other"
		},
		"objective ID": func(_ *Selector, gap *cognitionreference.SemanticGap, _ *llm.PreparedModel) {
			gap.ObjectiveID = "objective.other"
		},
		"prompt": func(_ *Selector, _ *cognitionreference.SemanticGap, value *llm.PreparedModel) { value.Prompt += "x" },
		"schema": func(_ *Selector, _ *cognitionreference.SemanticGap, value *llm.PreparedModel) {
			value.ResponseSchema = responseSchema(testGap())
			value.ResponseSchema["additionalProperties"] = true
		},
		"model": func(_ *Selector, _ *cognitionreference.SemanticGap, value *llm.PreparedModel) {
			value.ContextModel += "x"
		},
		"context": func(_ *Selector, _ *cognitionreference.SemanticGap, value *llm.PreparedModel) { value.ContextTokens++ },
		"input limit": func(value *Selector, _ *cognitionreference.SemanticGap, _ *llm.PreparedModel) {
			value.limits.MaxInputTokens++
		},
		"output limit": func(_ *Selector, _ *cognitionreference.SemanticGap, value *llm.PreparedModel) {
			value.MaxOutputTokens++
		},
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			copySelector, copyGap, copyPrepared := *selector, baseGap.Clone(), base
			mutate(&copySelector, &copyGap, &copyPrepared)
			if (name == "gap ID" || name == "gap kind" || name == "objective ID") &&
				copyPrepared.Prompt != base.Prompt {
				t.Fatalf("code-held %s leaked into model-visible prompt", name)
			}
			challenge, err := copySelector.observationChallenge(copyGap, copyPrepared)
			if err != nil {
				t.Fatal(err)
			}
			if challenge == baseChallenge {
				t.Fatalf("%s did not change provider observation challenge", name)
			}
		})
	}
}
