package cognitionstate

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestMaterializePlanRevisionUsesIndependentPlanGeneration(t *testing.T) {
	t.Parallel()
	evidence := mappingTestObservation(t, "").EvidenceRef()
	snapshot := mappingPlanRevisionSnapshot(t, evidence, 7)
	graph := mappingActiveGraph(t, snapshot)
	next := cognition.GoalExpression{All: []cognition.Predicate{{
		Name: "condition.alternate", Args: []string{"target-41"},
	}}}
	decision := cognition.CognitionDecision{
		ObligationID: snapshot.CurrentObligation().ID,
		Action: cognition.ActionRequest{
			Kind: "inspect", Arguments: []cognition.ActionArgument{{Name: "target", Value: "entity-1"}},
		},
		EvidenceRefs: []cognition.EvidenceRef{evidence}, ExpectedEffect: "Inspect the alternate path.",
		Proposals: []cognition.LedgerProposal{{
			Kind: cognition.ProposalPlanRevision,
			PlanRevision: &cognition.PlanRevisionProposal{
				Next: next, EvidenceRefs: []cognition.EvidenceRef{evidence},
			},
		}},
	}
	authority, err := cognition.NewCompletionAuthority(
		snapshot.CurrentObligation().CompletionCheck,
		[]cognition.PredicateName{"condition.alternate", "goal.condition"},
	)
	if err != nil {
		t.Fatal(err)
	}
	value, err := MaterializePlanRevisionProposal(PlanRevisionProposalInput{
		Graph: graph, Snapshot: snapshot, Decision: decision,
		ActionSchema: mappingTestSchema(t), ProposalIndex: 0, CompletionAuthority: authority,
	})
	if err != nil {
		t.Fatalf("materialize plan revision: %v", err)
	}
	if snapshot.Attempt().Generation != 2 || value.PreviousGeneration != 7 || value.NextGeneration != 8 {
		t.Fatalf("job/plan generations = %d/%d→%d", snapshot.Attempt().Generation, value.PreviousGeneration, value.NextGeneration)
	}
	after, err := value.Apply(graph)
	if err != nil || after.Generation != 8 {
		t.Fatalf("apply plan revision: generation=%d error=%v", after.Generation, err)
	}
	mutations, err := MapModelProposals(ModelProposalInput{
		Ledger: mappingTestLedger(t), Snapshot: snapshot, Decision: decision,
		ActionSchema: mappingTestSchema(t),
	})
	if err != nil || len(mutations) != 1 || mutations[0].Descriptor().SourceKind != SourceModelPlanRevision {
		t.Fatalf("plan revision candidate = %#v error=%v", mutations, err)
	}
}

func TestMaterializePlanRevisionRejectsChangedRootAndMixedProposal(t *testing.T) {
	t.Parallel()
	evidence := mappingTestObservation(t, "").EvidenceRef()
	snapshot := mappingPlanRevisionSnapshot(t, evidence, 7)
	graph := mappingActiveGraph(t, snapshot)
	proposal := cognition.LedgerProposal{
		Kind: cognition.ProposalPlanRevision,
		PlanRevision: &cognition.PlanRevisionProposal{
			Next:         cognition.GoalExpression{All: []cognition.Predicate{{Name: "condition.alternate", Args: []string{"target-41"}}}},
			EvidenceRefs: []cognition.EvidenceRef{evidence},
		},
	}
	decision := cognition.CognitionDecision{
		ObligationID: snapshot.CurrentObligation().ID,
		Action:       cognition.ActionRequest{Kind: "inspect", Arguments: []cognition.ActionArgument{{Name: "target", Value: "entity-1"}}},
		EvidenceRefs: []cognition.EvidenceRef{evidence}, ExpectedEffect: "Inspect the alternate path.",
		Proposals: []cognition.LedgerProposal{proposal},
	}
	authority, err := cognition.NewCompletionAuthority(
		snapshot.CurrentObligation().CompletionCheck,
		[]cognition.PredicateName{"condition.alternate", "goal.changed", "goal.condition"},
	)
	if err != nil {
		t.Fatal(err)
	}
	base := PlanRevisionProposalInput{
		Graph: graph, Snapshot: snapshot, Decision: decision,
		ActionSchema: mappingTestSchema(t), CompletionAuthority: authority,
	}
	changed := base
	changed.Graph = base.Graph.Clone()
	changed.Graph.Obligations[0].Desired = cognition.GoalExpression{All: []cognition.Predicate{{Name: "goal.changed", Args: []string{"target-41"}}}}
	if _, err := MaterializePlanRevisionProposal(changed); !errors.Is(err, ErrInvalidMapping) {
		t.Fatalf("changed root error = %v", err)
	}
	mixed := base
	mixed.Decision = base.Decision.Clone()
	mixed.Decision.Proposals = append(mixed.Decision.Proposals, cognition.LedgerProposal{
		Kind: cognition.ProposalQuestion, Content: "What remains?",
	})
	if _, err := MaterializePlanRevisionProposal(mixed); !errors.Is(err, ErrInvalidMapping) {
		t.Fatalf("mixed proposal error = %v", err)
	}
}

func mappingPlanRevisionSnapshot(
	t *testing.T,
	evidence cognition.EvidenceRef,
	planGeneration uint64,
) cognition.RuntimeSnapshot {
	t.Helper()
	base := mappingTestSnapshot(t, evidence)
	current := base.CurrentObligation()
	current.CreatedGeneration = planGeneration
	snapshot, err := cognition.NewRuntimeSnapshot(
		base.Goal(), base.CurrentRevision(), current, base.ActionCatalog(), base.Attempt(),
		base.ContextProjection(), base.Budget(), base.EvidenceRefs(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
