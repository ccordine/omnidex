package cognitionruntime

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func configureChildObligation(t *testing.T, harness *runtimeHarness) cognition.ObligationID {
	t.Helper()
	generation := harness.fixture.graph.Generation
	root := harness.fixture.graph.Obligations[0]
	predicate, err := cognition.NewPredicate("prerequisite.ready", []string{"target"})
	requireNoError(t, err)
	goal, err := cognition.NewGoalExpression([]cognition.Predicate{predicate}, nil, nil)
	requireNoError(t, err)
	childID := cognition.ObligationID("obligation-child")
	child := cognition.ObligationSpec{
		ID: childID, ParentID: root.ID, Desired: goal,
		SupportingRefs: []cognition.EvidenceRef{harness.fixture.evidence},
		CompletionCheck: cognition.CompletionCheckRef{
			ID: "completion-child", Version: "1.0.0", SHA256: runtimeDigest("completion-child"),
		},
	}
	rootSpec := cognition.ObligationSpec{
		ID: root.ID, Desired: root.Desired, DependsOn: []cognition.ObligationID{childID},
		SupportingRefs: root.SupportingRefs, CompletionCheck: root.CompletionCheck,
	}
	graph, err := cognition.NewObligationGraph(
		generation, root.ID, []cognition.ObligationSpec{rootSpec, child},
	)
	requireNoError(t, err)
	requireNoError(t, graph.RefreshReadiness(generation))
	requireNoError(t, graph.Transition(childID, generation, cognition.ObligationActive))
	harness.graph, harness.version = graph.Snapshot(), 1
	return childID
}
