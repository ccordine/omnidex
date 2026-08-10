package cognition

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func testRuntimeBudget() RuntimeBudget {
	return RuntimeBudget{
		RemainingPolicyCalls:   1,
		MaxInputBytes:          64 * 1024,
		MaxInputTokens:         16 * 1024,
		MaxOutputBytes:         16 * 1024,
		MaxOutputTokens:        4 * 1024,
		MaxEvidenceRefs:        4,
		MaxActionArguments:     4,
		MaxLedgerProposals:     4,
		MaxAttentionRequests:   4,
		MaxExpectedEffectBytes: 512,
	}
}

func testContextProjectionRef() ContextProjectionRef {
	return ContextProjectionRef{
		ID:     "context_projection_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		SHA256: testDigest, WorkingSetID: "working-set-1", WorkingSetVersion: 3,
		RendererVersion: "omnidex.context-material-json.v1",
	}
}

func testRuntimeSnapshot(t *testing.T) (RuntimeSnapshot, ActionSchema, EvidenceRef) {
	t.Helper()
	evidence := testEvidenceRef(t)
	root := testObligationSpec(t, "obligation-root", "")
	root.SupportingRefs = []EvidenceRef{evidence}
	graph, err := NewObligationGraph(1, root.ID, []ObligationSpec{root})
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(1); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(root.ID, 1, ObligationActive); err != nil {
		t.Fatal(err)
	}
	schema := testActionSchema(t, EvidenceRequired)
	catalog, err := NewActionCatalog("catalog.runtime", "1.0.0", []ActionSchema{schema})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := NewRuntimeSnapshot(
		testGoalExpression(t, "goal.complete"),
		evidence.Revision,
		requireObligation(t, graph, root.ID),
		catalog,
		testAttemptRef(),
		testContextProjectionRef(),
		testRuntimeBudget(),
		[]EvidenceRef{evidence},
	)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, schema, evidence
}

func testPolicyDecision(snapshot RuntimeSnapshot, evidence EvidenceRef) CognitionDecision {
	return CognitionDecision{
		ObligationID: snapshot.CurrentObligation().ID,
		Action: ActionRequest{
			Kind: "inspect", Arguments: []ActionArgument{{Name: "target", Value: "entity-1"}},
		},
		EvidenceRefs:   []EvidenceRef{evidence},
		ExpectedEffect: "Expose the target's current public properties.",
	}
}

func TestCoordinatorProducesOneValidatedActionWithoutEnvironmentMutation(t *testing.T) {
	t.Parallel()
	snapshot, schema, evidence := testRuntimeSnapshot(t)
	policyCalls := 0
	policy := PolicyFunc(func(_ context.Context, received RuntimeSnapshot) (PolicyOutcome, error) {
		policyCalls++
		goal := received.Goal()
		goal.All[0].Args[0] = "policy-mutation"
		catalog := received.ActionCatalog()
		catalog.Schemas[0].Kind = "policy-mutation"
		refs := received.EvidenceRefs()
		refs[0].SHA256 = strings.Repeat("b", 64)
		return PolicyOutcome{
			Decision: testPolicyDecision(received, evidence), InferenceExecuted: true,
		}, nil
	})
	coordinator, err := NewCoordinator(policy)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := NewCompletionResult(
		snapshot.CurrentObligation().ID,
		snapshot.CurrentObligation().CompletionCheck,
		snapshot.CurrentRevision(),
		CompletionUnsatisfied,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	step, err := coordinator.Step(context.Background(), snapshot, completion, snapshot.EvidenceRefs())
	if err != nil {
		t.Fatalf("coordinator step: %v", err)
	}
	if policyCalls != 1 || !step.PolicyCalled || step.State != CoordinatorActionReady || step.Decision == nil {
		t.Fatalf("step = %#v, policy calls=%d", step, policyCalls)
	}
	if step.ActionSchema != schema.Ref() || step.RemainingBudget.RemainingPolicyCalls != 0 {
		t.Fatalf("schema/budget not bound: %#v", step)
	}
	if step.Actor != testAttemptRef() || step.ContextProjection != testContextProjectionRef() {
		t.Fatalf("step lost attempt/projection binding: %#v", step)
	}
	if got := snapshot.Goal().All[0].Args[0]; got == "policy-mutation" {
		t.Fatal("policy mutated immutable snapshot goal")
	}
	if got := snapshot.ActionCatalog().Schemas[0].Kind; got == "policy-mutation" {
		t.Fatal("policy mutated immutable snapshot catalog")
	}
	if got := snapshot.EvidenceRefs()[0].SHA256; got != evidence.SHA256 {
		t.Fatal("policy mutated immutable snapshot evidence")
	}
}

func TestCoordinatorRejectsPolicyAuthorityAndEvidenceViolations(t *testing.T) {
	t.Parallel()
	snapshot, _, evidence := testRuntimeSnapshot(t)
	completion, err := NewCompletionResult(
		snapshot.CurrentObligation().ID,
		snapshot.CurrentObligation().CompletionCheck,
		snapshot.CurrentRevision(),
		CompletionUnsatisfied,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}

	for name, mutate := range map[string]func(*CognitionDecision){
		"wrong obligation":    func(value *CognitionDecision) { value.ObligationID = "obligation-other" },
		"unknown evidence":    func(value *CognitionDecision) { value.EvidenceRefs[0].ObservationID = "observation-other" },
		"unregistered action": func(value *CognitionDecision) { value.Action.Kind = "unregistered" },
		"proposal budget": func(value *CognitionDecision) {
			for index := 0; index < testRuntimeBudget().MaxLedgerProposals+1; index++ {
				value.Proposals = append(value.Proposals, LedgerProposal{
					Kind: ProposalQuestion, Content: "Question " + string(rune('a'+index)) + "?",
				})
			}
		},
	} {
		mutate := mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			coordinator, err := NewCoordinator(PolicyFunc(func(_ context.Context, received RuntimeSnapshot) (PolicyOutcome, error) {
				decision := testPolicyDecision(received, evidence)
				mutate(&decision)
				return PolicyOutcome{Decision: decision, InferenceExecuted: true}, nil
			}))
			if err != nil {
				t.Fatal(err)
			}
			step, err := coordinator.Step(
				context.Background(), snapshot, completion, snapshot.EvidenceRefs(),
			)
			if !errors.Is(err, ErrInvalidDecision) {
				t.Fatalf("error = %v, want ErrInvalidDecision", err)
			}
			if !step.PolicyCalled {
				t.Fatalf("invalid post-inference decision lost its executed-call evidence: %+v", step)
			}
		})
	}
}

func TestCoordinatorFailsLoudlyWhenPolicyIsUnavailableOrFails(t *testing.T) {
	t.Parallel()
	if _, err := NewCoordinator(nil); !errors.Is(err, ErrPolicyUnavailable) {
		t.Fatalf("nil policy error = %v, want ErrPolicyUnavailable", err)
	}
	var nilPolicy PolicyFunc
	if _, err := NewCoordinator(nilPolicy); !errors.Is(err, ErrPolicyUnavailable) {
		t.Fatalf("typed nil policy error = %v, want ErrPolicyUnavailable", err)
	}
	snapshot, _, _ := testRuntimeSnapshot(t)
	completion, err := NewCompletionResult(
		snapshot.CurrentObligation().ID,
		snapshot.CurrentObligation().CompletionCheck,
		snapshot.CurrentRevision(), CompletionUnsatisfied, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	policyCause := errors.New("policy unavailable")
	coordinator, err := NewCoordinator(PolicyFunc(func(context.Context, RuntimeSnapshot) (PolicyOutcome, error) {
		return PolicyOutcome{InferenceExecuted: true}, policyCause
	}))
	if err != nil {
		t.Fatal(err)
	}
	step, stepErr := coordinator.Step(
		context.Background(), snapshot, completion, snapshot.EvidenceRefs(),
	)
	if !errors.Is(stepErr, ErrPolicyFailed) || !errors.Is(stepErr, policyCause) || !step.PolicyCalled {
		t.Fatalf("policy failure step=%+v error=%v, want called wrapper and exact cause", step, stepErr)
	}
}

func TestCoordinatorRejectsExhaustedBudgetAndInvalidCompletion(t *testing.T) {
	t.Parallel()
	snapshot, _, evidence := testRuntimeSnapshot(t)
	exhausted, err := NewRuntimeSnapshot(
		snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
		snapshot.ActionCatalog(), snapshot.Attempt(), snapshot.ContextProjection(), RuntimeBudget{
			MaxInputBytes: 64 * 1024, MaxInputTokens: 16 * 1024,
			MaxOutputBytes: 16 * 1024, MaxOutputTokens: 4 * 1024,
			MaxEvidenceRefs: 1, MaxActionArguments: 1,
			MaxExpectedEffectBytes: 128,
		}, snapshot.EvidenceRefs(),
	)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := NewCoordinator(PolicyFunc(func(context.Context, RuntimeSnapshot) (PolicyOutcome, error) {
		return PolicyOutcome{}, nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	completion, err := NewCompletionResult(snapshot.CurrentObligation().ID, snapshot.CurrentObligation().CompletionCheck, snapshot.CurrentRevision(), CompletionUnsatisfied, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := coordinator.Step(
		context.Background(), exhausted, completion, exhausted.EvidenceRefs(),
	); !errors.Is(err, ErrCoordinatorBudgetExhausted) {
		t.Fatalf("budget error = %v, want ErrCoordinatorBudgetExhausted", err)
	}
	wrongCheck := completion
	wrongCheck.Check.ID = "check.other"
	if _, err := coordinator.Step(
		context.Background(), snapshot, wrongCheck, snapshot.EvidenceRefs(),
	); !errors.Is(err, ErrInvalidCompletionResult) {
		t.Fatalf("completion error = %v, want ErrInvalidCompletionResult", err)
	}
	_ = evidence
}

func TestRuntimeSnapshotRequiresExactAttemptAndContextProjection(t *testing.T) {
	t.Parallel()
	snapshot, _, _ := testRuntimeSnapshot(t)
	if snapshot.Attempt() != testAttemptRef() || snapshot.ContextProjection() != testContextProjectionRef() {
		t.Fatalf("snapshot authority refs are not exact")
	}
	invalidAttempt := snapshot.Attempt()
	invalidAttempt.Generation = 0
	if _, err := NewRuntimeSnapshot(
		snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
		snapshot.ActionCatalog(), invalidAttempt, snapshot.ContextProjection(),
		snapshot.Budget(), snapshot.EvidenceRefs(),
	); !errors.Is(err, ErrInvalidRuntimeSnapshot) {
		t.Fatalf("invalid attempt error = %v, want ErrInvalidRuntimeSnapshot", err)
	}
	invalidProjection := snapshot.ContextProjection()
	invalidProjection.WorkingSetVersion = 0
	if _, err := NewRuntimeSnapshot(
		snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
		snapshot.ActionCatalog(), snapshot.Attempt(), invalidProjection,
		snapshot.Budget(), snapshot.EvidenceRefs(),
	); !errors.Is(err, ErrInvalidRuntimeSnapshot) {
		t.Fatalf("invalid projection error = %v, want ErrInvalidRuntimeSnapshot", err)
	}
}
