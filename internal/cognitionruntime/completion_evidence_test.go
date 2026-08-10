package cognitionruntime

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestCompletionMayUseCodeOnlyEvidenceAbsentFromModelSnapshot(t *testing.T) {
	fixture := newRuntimeFixture(t)
	root := fixture.graph.Obligations[0]
	graph, err := cognition.NewObligationGraph(
		fixture.graph.Generation, root.ID,
		[]cognition.ObligationSpec{{
			ID: root.ID, Desired: root.Desired, DependsOn: []cognition.ObligationID{},
			SupportingRefs: []cognition.EvidenceRef{}, CompletionCheck: root.CompletionCheck,
		}},
	)
	requireNoError(t, err)
	requireNoError(t, graph.RefreshReadiness(fixture.graph.Generation))
	requireNoError(t, graph.Transition(root.ID, fixture.graph.Generation, cognition.ObligationActive))
	current, exists := graph.Obligation(root.ID)
	if !exists {
		t.Fatal("active obligation is missing")
	}
	projection := cognition.ContextProjectionRef{
		ID: "projection-completion-packet", SHA256: runtimeDigest("completion-packet"),
		WorkingSetID: "working-set-runtime", WorkingSetVersion: 7, RendererVersion: "1.0.0",
	}
	snapshot, err := cognition.NewRuntimeSnapshot(
		fixture.goal, fixture.revision, current, fixture.catalog, fixture.binding.Attempt,
		projection, cognition.RuntimeBudget{
			RemainingPolicyCalls: 1, MaxInputBytes: 4096, MaxInputTokens: 1024,
			MaxOutputBytes: 2048, MaxOutputTokens: 512, MaxEvidenceRefs: 4,
			MaxActionArguments: 4, MaxLedgerProposals: 4, MaxAttentionRequests: 4,
			MaxExpectedEffectBytes: 256,
		}, []cognition.EvidenceRef{},
	)
	requireNoError(t, err)
	result, err := cognition.NewCompletionResult(
		root.ID, root.CompletionCheck, fixture.revision,
		cognition.CompletionSatisfied, []cognition.EvidenceRef{fixture.evidence},
	)
	requireNoError(t, err)
	prepared := PreparedSnapshot{
		Snapshot: snapshot, ObligationGraph: graph.Snapshot(), GraphVersion: 1,
		CompletionEvidenceRefs: []cognition.EvidenceRef{fixture.evidence},
	}
	if err := prepared.ValidateFor(fixture.binding); err != nil {
		t.Fatalf("code-only completion packet invalid: %v", err)
	}
	if len(prepared.Snapshot.EvidenceRefs()) != 0 ||
		len(completionRequest(prepared, fixture.binding).EvidenceRefs) != 1 {
		t.Fatalf("model and completion evidence were not separated")
	}
	if err := validateCompletionResult(prepared, result); err != nil {
		t.Fatalf("prepared packet evidence rejected: %v", err)
	}
	command := completionCommand(prepared, fixture.binding, result)
	if len(command.CompletionEvidenceRefs) != 1 || command.CompletionEvidenceRefs[0] != fixture.evidence {
		t.Fatalf("progress command omitted code-owned completion evidence: %#v", command)
	}
}

func TestStepSealsWithCodeOnlyCompletionEvidenceAndNoPolicyCall(t *testing.T) {
	harness := newRuntimeHarness(t)
	root := harness.graph.Obligations[0]
	graph, err := cognition.NewObligationGraph(
		harness.graph.Generation, root.ID,
		[]cognition.ObligationSpec{{
			ID: root.ID, Desired: root.Desired, SupportingRefs: []cognition.EvidenceRef{},
			CompletionCheck: root.CompletionCheck,
		}},
	)
	requireNoError(t, err)
	requireNoError(t, graph.RefreshReadiness(harness.graph.Generation))
	requireNoError(t, graph.Transition(root.ID, harness.graph.Generation, cognition.ObligationActive))
	harness.graph = graph.Snapshot()
	harness.useModelEvidenceOverride = true
	harness.modelEvidenceOverride = []cognition.EvidenceRef{}
	harness.completionResultEvidence = []cognition.EvidenceRef{harness.fixture.evidence}
	harness.forceSatisfied, harness.terminal = true, true
	harness.public = "The code-owned completion predicate is satisfied."
	runtime, err := New(harness.dependencies())
	requireNoError(t, err)

	result, err := runtime.Step(context.Background(), harness.fixture.binding)
	requireNoError(t, err)
	if result.State != StepEpisodeCompleted || result.Seal == nil || result.PolicyCalled ||
		harness.policyCalls != 0 || harness.completionCalls != 1 {
		t.Fatalf(
			"result=%#v policy calls=%d completion calls=%d",
			result, harness.policyCalls, harness.completionCalls,
		)
	}
}
