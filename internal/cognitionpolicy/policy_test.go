package cognitionpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

func TestPolicyReservesCallBeforeInferenceAndPersistsAcceptedResultBeforeReturn(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, evidence := policyTestSnapshot(t, projection)
	client := &policyTestClient{response: policyTestResponse(t, snapshot, evidence)}
	journal := &policyTestCallJournal{}
	loader := newPolicyTestProjectionLoader(projection)
	policy, err := New(client, policyTestAttestedBrain(), loader, journal)
	if err != nil {
		t.Fatalf("new policy: %v", err)
	}
	outcome, err := policy.Decide(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	decision := outcome.Decision
	if !outcome.InferenceExecuted {
		t.Fatal("accepted provider generation was not reported")
	}
	if decision.ObligationID != snapshot.CurrentObligation().ID || decision.Action.Kind != "inspect" {
		t.Fatalf("decision = %#v", decision)
	}
	if client.generateCalls != 1 || client.plainGenerateCalls != 0 || client.prepareCalls != 1 ||
		client.cleanupCalls != 1 || client.otherCalls != 0 || client.model != policyTestBrain().Model {
		t.Fatalf("client calls prepared=%d plain=%d prepare=%d cleanup=%d other=%d model=%q",
			client.generateCalls, client.plainGenerateCalls, client.prepareCalls,
			client.cleanupCalls, client.otherCalls, client.model)
	}
	prepared := client.prepared[0]
	if prepared.PromptHint != llm.MinimalGeneratePrompt ||
		prepared.MaxOutputTokens != snapshot.Budget().MaxOutputTokens ||
		prepared.ContextTokens != policyTestBrain().NativeContextLimit ||
		prepared.ResponseFormat != llm.ResponseFormatJSON || len(prepared.ResponseSchema) == 0 ||
		prepared.ThinkingEnabled || prepared.Temperature == nil || *prepared.Temperature != 0 {
		t.Fatalf("prepared cognition contract=%+v", prepared)
	}
	if loader.calls != 1 {
		t.Fatalf("projection loads=%d want 1", loader.calls)
	}
	if len(journal.attempts) != 1 || len(journal.results) != 1 {
		t.Fatalf("attempts=%d results=%d, want 1/1", len(journal.attempts), len(journal.results))
	}
	attempt, result := journal.attempts[0], journal.results[0]
	if err := attempt.Validate(); err != nil {
		t.Fatalf("validate call attempt: %v", err)
	}
	if err := result.Validate(attempt); err != nil {
		t.Fatalf("validate call result: %v", err)
	}
	if attempt.Actor != snapshot.Attempt() || attempt.SnapshotSHA256 != snapshot.SHA256() ||
		attempt.ExpectedRevision != snapshot.CurrentRevision() ||
		attempt.ObligationID != snapshot.CurrentObligation().ID ||
		attempt.RuntimeBudget != snapshot.Budget() ||
		attempt.ContextProjection != snapshot.ContextProjection() || attempt.Brain != policyTestBrain() {
		t.Fatalf("authority identity was not persisted exactly: %#v", attempt)
	}
	if attempt.Envelope != client.prompt || result.Response != client.response ||
		attempt.EnvelopeSHA256 == "" || result.ResponseSHA256 == "" || result.Status != CallResultAccepted {
		t.Fatalf("model bytes/result were not persisted exactly: %#v / %#v", attempt, result)
	}
	if attempt.ProviderAttestation != policyTestAttestedBrain().Attestation {
		t.Fatal("policy call omitted exact live provider attestation")
	}
}

func TestPolicyRefusesDecisionWhenCallFinishFails(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, evidence := policyTestSnapshot(t, projection)
	client := &policyTestClient{response: policyTestResponse(t, snapshot, evidence)}
	journal := &policyTestCallJournal{finishErr: errors.New("storage unavailable")}
	policy, err := New(client, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), journal)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := policy.Decide(context.Background(), snapshot)
	if !errors.Is(err, ErrCallJournal) {
		t.Fatalf("error = %v, want ErrCallJournal", err)
	}
	decision := outcome.Decision
	if !outcome.InferenceExecuted {
		t.Fatal("failed finish hid the executed provider generation")
	}
	if decision.ObligationID != "" || decision.Action.Kind != "" || len(decision.EvidenceRefs) != 0 {
		t.Fatalf("decision escaped failed call finish: %#v", decision)
	}
	if client.generateCalls != 1 || len(journal.attempts) != 1 || len(journal.results) != 1 {
		t.Fatalf("calls=%d attempts=%d results=%d", client.generateCalls, len(journal.attempts), len(journal.results))
	}
}

func TestPolicyRejectsProjectionIdentityMismatchBeforeModelCall(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, evidence := policyTestSnapshot(t, projection)
	for name, testCase := range map[string]struct {
		mutate   func(*cognition.ContextProjectionRef)
		expected error
	}{
		"ID": {func(ref *cognition.ContextProjectionRef) {
			ref.ID = "context-projection-other"
		}, ErrInvalidProjection},
		"hash": {func(ref *cognition.ContextProjectionRef) {
			ref.SHA256 = strings.Repeat("c", 64)
		}, ErrProjectionMismatch},
		"working set": {func(ref *cognition.ContextProjectionRef) {
			ref.WorkingSetID = "working-set-other"
		}, ErrProjectionMismatch},
		"version": {func(ref *cognition.ContextProjectionRef) {
			ref.WorkingSetVersion++
		}, ErrProjectionMismatch},
		"renderer": {func(ref *cognition.ContextProjectionRef) {
			ref.RendererVersion = "renderer.other"
		}, ErrProjectionMismatch},
	} {
		testCase := testCase
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			ref := snapshot.ContextProjection()
			testCase.mutate(&ref)
			mismatched, err := cognition.NewRuntimeSnapshot(
				snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
				snapshot.ActionCatalog(), snapshot.Attempt(), ref,
				snapshot.Budget(), snapshot.EvidenceRefs(),
			)
			if err != nil {
				t.Fatal(err)
			}
			client := &policyTestClient{response: policyTestResponse(t, snapshot, evidence)}
			policy, err := New(client, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), &policyTestCallJournal{})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := policy.Decide(context.Background(), mismatched); !errors.Is(err, testCase.expected) {
				t.Fatalf("error = %v, want %v", err, testCase.expected)
			}
			if client.generateCalls != 0 {
				t.Fatal("projection mismatch reached model")
			}
		})
	}
}

func TestPolicyEnforcesHardResponseLimit(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, _ := policyTestSnapshot(t, projection)
	client := &policyTestClient{response: strings.Repeat("x", MaxResponseBytes+1)}
	journal := &policyTestCallJournal{}
	policy, err := New(client, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), snapshot); !errors.Is(err, ErrResponseLimit) {
		t.Fatalf("error = %v, want ErrResponseLimit", err)
	}
	if client.generateCalls != 1 {
		t.Fatalf("generate calls = %d, want 1", client.generateCalls)
	}
	if len(journal.results) != 1 || journal.results[0].Status != CallResultRejected ||
		journal.results[0].ResponseStored {
		t.Fatalf("oversize result was not recorded as bounded rejection: %#v", journal.results)
	}
}

func TestPolicyEnforcesFreshPerCallInputAndOutputBudgets(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	base, evidence := policyTestSnapshot(t, projection)

	inputBudget := base.Budget()
	inputBudget.MaxInputBytes = 1
	inputBudget.MaxInputTokens = 1
	inputLimited := policySnapshotWithBudget(t, base, inputBudget)
	inputClient := &policyTestClient{response: policyTestResponse(t, base, evidence)}
	policy, err := New(inputClient, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), &policyTestCallJournal{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), inputLimited); !errors.Is(err, ErrEnvelopeLimit) {
		t.Fatalf("input budget error=%v", err)
	}
	if inputClient.generateCalls != 0 {
		t.Fatal("input-budget overflow reached the model")
	}

	outputBudget := base.Budget()
	outputBudget.MaxOutputBytes = 1
	outputBudget.MaxOutputTokens = 1
	outputLimited := policySnapshotWithBudget(t, base, outputBudget)
	outputClient := &policyTestClient{response: policyTestResponse(t, base, evidence)}
	outputJournal := &policyTestCallJournal{}
	policy, err = New(outputClient, policyTestAttestedBrain(), newPolicyTestProjectionLoader(projection), outputJournal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), outputLimited); !errors.Is(err, ErrResponseLimit) {
		t.Fatalf("output budget error=%v", err)
	}
	if outputClient.generateCalls != 1 {
		t.Fatalf("output-budget model calls=%d want 1", outputClient.generateCalls)
	}
	if len(outputJournal.results) != 1 || outputJournal.results[0].Status != CallResultRejected {
		t.Fatalf("output-budget rejection was not persisted: %#v", outputJournal.results)
	}
}

func TestPolicyLoadsOneDisposableProjectionPerCallWithoutHistory(t *testing.T) {
	t.Parallel()
	firstProjection := policyTestProjection(t, "first-clean-desk-authority")
	secondProjection := policyTestProjection(t, "second-clean-desk-authority")
	firstSnapshot, evidence := policyTestSnapshot(t, firstProjection)
	secondSnapshot, _ := policyTestSnapshot(t, secondProjection)
	loader := newPolicyTestProjectionLoader(firstProjection)
	loader.projections[cognition.ContextProjectionID(secondProjection.ID)] = cloneProjection(secondProjection)
	client := &policyTestClient{response: policyTestResponse(t, firstSnapshot, evidence)}
	journal := &policyTestCallJournal{}
	policy, err := New(client, policyTestAttestedBrain(), loader, journal)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), firstSnapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := policy.Decide(context.Background(), secondSnapshot); err != nil {
		t.Fatal(err)
	}
	if loader.calls != 2 || len(client.prompts) != 2 || len(journal.attempts) != 2 || len(journal.results) != 2 {
		t.Fatalf("loads=%d prompts=%d attempts=%d results=%d", loader.calls, len(client.prompts), len(journal.attempts), len(journal.results))
	}
	if strings.Contains(client.prompts[0], "second-clean-desk-authority") ||
		strings.Contains(client.prompts[1], "first-clean-desk-authority") {
		t.Fatal("a cognition call inherited another call's projected material")
	}
	for index, prompt := range client.prompts {
		var envelope map[string]json.RawMessage
		if err := json.Unmarshal([]byte(prompt), &envelope); err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"chat_history", "transcript", "messages", "previous_prompt"} {
			if _, exists := envelope[forbidden]; exists {
				t.Fatalf("prompt %d contains accumulated-context field %q", index, forbidden)
			}
		}
	}
}
