package qwenselector_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognitionreference"
	"github.com/gryph/omnidex/internal/cognitionreference/qwenselector"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/ollama"
)

const (
	liveQwenModel   = "qwen3.5:9b-q4_K_M"
	liveQwenContext = 32768
)

type liveExactRecorder struct {
	*ollama.Client
	mu            sync.Mutex
	calls         int
	dispatched    int
	prepared      llm.PreparedModel
	generation    llm.PreparedGeneration
	request       []byte
	generationErr error
}

func (recorder *liveExactRecorder) GeneratePreparedExact(
	ctx context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	raw, renderErr := llm.ExactPreparedRequestBytes(prepared)
	if renderErr != nil {
		return llm.PreparedGeneration{}, renderErr
	}
	generation, err := recorder.Client.GeneratePreparedExact(ctx, prepared)
	recorder.mu.Lock()
	recorder.calls++
	if generation.ProviderRequestDisposition.MayHaveReachedProvider() {
		recorder.dispatched++
	}
	recorder.prepared = prepared
	recorder.generation = generation
	recorder.request = append([]byte{}, raw...)
	recorder.generationErr = err
	recorder.mu.Unlock()
	return generation, err
}

func TestLiveContaminatedQwenSemanticGapVertical(t *testing.T) {
	if os.Getenv("OMNIDEX_COGNITION_GAP_SMOKE") != "1" {
		t.Skip("set OMNIDEX_COGNITION_GAP_SMOKE=1 to run the real-Qwen semantic-gap smoke")
	}
	t.Log("COGNITION GAP SMOKE — CONTAMINATED — NON-PROMOTABLE — IN-MEMORY DEVELOPMENT EVIDENCE ONLY")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	endpoint := strings.TrimSpace(os.Getenv("OMNIDEX_TEST_OLLAMA_URL"))
	if endpoint == "" {
		endpoint = "http://127.0.0.1:11434"
	}
	recorder := &liveExactRecorder{Client: ollama.New(
		endpoint, liveQwenModel, "", 15*time.Minute, liveQwenContext,
	)}
	selection := llm.ProviderIdentitySelection{
		Model: liveQwenModel, NativeContextLimit: liveQwenContext,
	}
	provider, err := llm.RequireDiscoveredProviderIdentityEvidence(
		ctx, recorder, selection, "contaminated-cognition-gap-smoke.v1",
	)
	if err != nil {
		t.Fatalf("discover exact installed Qwen identity: %v", err)
	}
	selector, err := qwenselector.New(
		recorder, provider, qwenselector.Limits{MaxInputTokens: 4096, MaxOutputTokens: 64},
	)
	if err != nil {
		t.Fatal(err)
	}
	machine, initial := liveSemanticMachine(t, selector)

	result, err := machine.Run(ctx, initial)
	if err != nil {
		logLiveExactEvidence(t, recorder)
		t.Fatal(err)
	}
	if recorder.calls != 1 || recorder.dispatched != 1 ||
		result.SelectorCalls != 1 || result.InferenceCalls != 1 {
		t.Fatalf(
			"provider/selector/inference calls=%d/%d/%d/%d, want exactly 1/1/1/1",
			recorder.calls, recorder.dispatched, result.SelectorCalls, result.InferenceCalls,
		)
	}
	if !result.Complete || !result.Final.HasPredicate("destination.reached") ||
		len(result.SemanticResolutions) != 1 || len(result.Trace) != 1 {
		t.Fatalf("live semantic gap did not complete exact vertical: %#v", result)
	}
	resolution := result.SemanticResolutions[0]
	if resolution.CandidateID != "C17" && resolution.CandidateID != "C23" {
		t.Fatalf("accepted nonmember candidate %q", resolution.CandidateID)
	}
	accepted, exists := result.Final.Fact("route.interpretation")
	if !exists || accepted != resolution.Fact {
		t.Fatalf("accepted semantic fact=%#v exists=%t resolution=%#v", accepted, exists, resolution)
	}
	wantArguments := []cognitionreference.Argument{{Name: "meaning", Value: accepted.Text}}
	if result.Trace[0].Operation != "follow.interpretation" ||
		!reflect.DeepEqual(result.Trace[0].Arguments, wantArguments) {
		t.Fatalf("code-owned transition=%#v, want unique operation with %#v", result.Trace[0], wantArguments)
	}
	logLiveExactEvidence(t, recorder)
}

func liveSemanticMachine(
	t *testing.T,
	selector cognitionreference.Selector,
) (cognitionreference.Machine, cognitionreference.State) {
	t.Helper()
	clue := cognitionreference.FactDefinition{ID: "route.clue", Kind: cognitionreference.FactText, MaxBytes: 128}
	parity := cognitionreference.FactDefinition{ID: "route.parity", Kind: cognitionreference.FactText, MaxBytes: 128}
	meaning := cognitionreference.FactDefinition{ID: "route.interpretation", Kind: cognitionreference.FactText, MaxBytes: 32}
	goal := cognitionreference.PredicateDefinition{ID: "destination.reached"}
	follow := cognitionreference.Operation{
		ID: "follow.interpretation", Requires: []cognitionreference.FactID{meaning.ID},
		Achieves: []cognitionreference.PredicateID{goal.ID},
		Bindings: []cognitionreference.Binding{{Argument: "meaning", Fact: meaning.ID}},
		Execute: func(_ context.Context, input cognitionreference.OperationInput) (cognitionreference.Transition, error) {
			value, exists := input.Argument("meaning")
			if !exists || (value != "sheltered" && value != "exposed") {
				return cognitionreference.Transition{}, cognitionreference.ErrInvalidTransition
			}
			return input.Transition(nil, []cognitionreference.PredicateID{goal.ID}, value+" route traversed")
		},
	}
	catalog, err := cognitionreference.NewCatalog(
		[]cognitionreference.FactDefinition{clue, parity, meaning},
		[]cognitionreference.PredicateDefinition{goal}, []cognitionreference.Operation{follow},
	)
	if err != nil {
		t.Fatal(err)
	}
	objective := cognitionreference.Objective{ID: "objective.reach-destination", Desired: goal.ID}
	gap := cognitionreference.SemanticGap{
		ID: "gap.route-meaning", Kind: cognitionreference.GapCandidateSelection,
		ObjectiveID: objective.ID,
		Question:    "Which equally supported route interpretation should this objective retain?",
		Evidence: []cognitionreference.SemanticEvidence{
			{ID: "E10", Content: "The authoritative clue permits either a sheltered or exposed route."},
			{ID: "E20", Content: "Both routes are legal, equal-cost, and equally supported; no registered fact breaks the tie."},
		},
		Candidates: []cognitionreference.SemanticCandidate{
			{ID: "C17", Summary: "Retain the sheltered interpretation.", EvidenceIDs: []cognitionreference.EvidenceID{"E10", "E20"}},
			{ID: "C23", Summary: "Retain the exposed interpretation.", EvidenceIDs: []cognitionreference.EvidenceID{"E10", "E20"}},
		},
	}
	producer := cognitionreference.SemanticFactProducer{
		FactID: meaning.ID, Gap: gap,
		EvidenceBindings: []cognitionreference.SemanticEvidenceBinding{
			{EvidenceID: "E10", FactID: clue.ID}, {EvidenceID: "E20", FactID: parity.ID},
		},
		Values: []cognitionreference.SemanticCandidateValue{
			{CandidateID: "C17", Fact: cognitionreference.Fact{ID: meaning.ID, Text: "sheltered"}},
			{CandidateID: "C23", Fact: cognitionreference.Fact{ID: meaning.ID, Text: "exposed"}},
		},
	}
	initial, err := catalog.NewState([]cognitionreference.Fact{
		{ID: clue.ID, Text: gap.Evidence[0].Content},
		{ID: parity.ID, Text: gap.Evidence[1].Content},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	machine, err := cognitionreference.NewMachineWithSemanticFacts(
		catalog, objective, cognitionreference.Limits{MaxSteps: 4, MaxDepth: 4},
		selector, []cognitionreference.SemanticFactProducer{producer},
	)
	if err != nil {
		t.Fatal(err)
	}
	return machine, initial
}

func logLiveExactEvidence(t *testing.T, recorder *liveExactRecorder) {
	t.Helper()
	schema, err := json.Marshal(recorder.prepared.ResponseSchema)
	if err != nil {
		t.Fatalf("marshal exact live response schema: %v", err)
	}
	requestSHA, err := llm.ExactPreparedRequestSHA256(recorder.prepared)
	if err != nil {
		t.Fatalf("hash exact live provider request: %v", err)
	}
	rendered, err := llm.ExactPreparedRequestBytes(recorder.prepared)
	if err != nil {
		t.Fatalf("render exact live provider request: %v", err)
	}
	if !bytes.Equal(rendered, recorder.request) {
		t.Fatal("recorded live provider request differs from exact prepared bytes")
	}
	if recorder.prepared.ProviderIdentityExpectation == nil {
		t.Fatal("live prepared request omitted provider identity expectation")
	}
	expected := recorder.prepared.ProviderIdentityExpectation
	t.Logf("prompt=%s", recorder.prepared.Prompt)
	t.Logf("schema=%s", schema)
	t.Logf("request_sha256=%s request_bytes=%d request=%s", requestSHA, len(recorder.request), recorder.request)
	t.Logf("provider_model=%s provider_digest=%s", expected.Model, expected.Digest)
	t.Logf(
		"native_prompt_tokens=%d native_output_tokens=%d output=%s generation_error=%v",
		recorder.generation.Usage.PromptEvalCount, recorder.generation.Usage.EvalCount,
		recorder.generation.Content, recorder.generationErr,
	)
}
