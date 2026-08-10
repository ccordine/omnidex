package cognitionstate

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestMaterializeObligationProposalBindsAcceptedSnapshotAndExactGraph(t *testing.T) {
	t.Parallel()
	evidence := mappingTestObservation(t, "").EvidenceRef()
	snapshot := mappingTestSnapshot(t, evidence)
	graph := mappingActiveGraph(t, snapshot)
	desired := cognition.GoalExpression{All: []cognition.Predicate{{
		Name: "condition.prerequisite", Args: []string{"target-41"},
	}}}
	decision := cognition.CognitionDecision{
		ObligationID: snapshot.CurrentObligation().ID,
		Action: cognition.ActionRequest{
			Kind: "inspect", Arguments: []cognition.ActionArgument{{Name: "target", Value: "entity-1"}},
		},
		EvidenceRefs: []cognition.EvidenceRef{evidence}, ExpectedEffect: "Expose the prerequisite state.",
		Proposals: []cognition.LedgerProposal{{
			Kind: cognition.ProposalObligation,
			Obligation: &cognition.ObligationProposal{
				Desired: desired, EvidenceRefs: []cognition.EvidenceRef{evidence},
			},
		}},
	}
	authority, err := cognition.NewCompletionAuthority(
		snapshot.CurrentObligation().CompletionCheck,
		[]cognition.PredicateName{"condition.prerequisite", "goal.condition"},
	)
	if err != nil {
		t.Fatal(err)
	}
	materialization, err := MaterializeObligationProposal(ObligationProposalInput{
		Graph: graph, Snapshot: snapshot, Decision: decision,
		ActionSchema: mappingTestSchema(t), ProposalIndex: 0,
		CompletionAuthority: authority,
	})
	if err != nil {
		t.Fatalf("materialize proposal: %v", err)
	}
	if materialization.SourceSnapshotSHA256 != snapshot.SHA256() ||
		materialization.Spec.ParentID != snapshot.CurrentObligation().ID {
		t.Fatalf("materialization = %#v", materialization)
	}
	after, err := materialization.Apply(graph)
	if err != nil {
		t.Fatalf("apply exact materialization: %v", err)
	}
	if after.SHA256 != materialization.ResultGraphSHA256 {
		t.Fatalf("after hash = %q, want %q", after.SHA256, materialization.ResultGraphSHA256)
	}
}

func TestMaterializeObligationProposalRejectsUnboundEvidenceAndGraphState(t *testing.T) {
	t.Parallel()
	evidence := mappingTestObservation(t, "").EvidenceRef()
	snapshot := mappingTestSnapshot(t, evidence)
	graph := mappingActiveGraph(t, snapshot)
	decision := cognition.CognitionDecision{
		ObligationID: snapshot.CurrentObligation().ID,
		Action: cognition.ActionRequest{
			Kind: "inspect", Arguments: []cognition.ActionArgument{{Name: "target", Value: "entity-1"}},
		},
		EvidenceRefs: []cognition.EvidenceRef{evidence}, ExpectedEffect: "Expose the prerequisite state.",
		Proposals: []cognition.LedgerProposal{{
			Kind: cognition.ProposalObligation,
			Obligation: &cognition.ObligationProposal{
				Desired:      cognition.GoalExpression{All: []cognition.Predicate{{Name: "condition.prerequisite", Args: []string{"target-41"}}}},
				EvidenceRefs: []cognition.EvidenceRef{evidence},
			},
		}},
	}
	authority, err := cognition.NewCompletionAuthority(
		snapshot.CurrentObligation().CompletionCheck,
		[]cognition.PredicateName{"condition.prerequisite", "goal.condition"},
	)
	if err != nil {
		t.Fatal(err)
	}
	base := ObligationProposalInput{
		Graph: graph, Snapshot: snapshot, Decision: decision,
		ActionSchema: mappingTestSchema(t), ProposalIndex: 0,
		CompletionAuthority: authority,
	}

	missing := base
	missing.Decision = base.Decision.Clone()
	missing.Decision.EvidenceRefs[0].ObservationID = "observation-not-projected"
	missing.Decision.Proposals[0].Obligation.EvidenceRefs[0] = missing.Decision.EvidenceRefs[0]
	if _, err := MaterializeObligationProposal(missing); !errors.Is(err, ErrInvalidMapping) {
		t.Fatalf("unbound evidence error = %v, want ErrInvalidMapping", err)
	}

	wrongGraph := base
	wrongGraph.Graph.Obligations[0].Desired = cognition.GoalExpression{All: []cognition.Predicate{{Name: "goal.changed", Args: []string{"target-41"}}}}
	if _, err := MaterializeObligationProposal(wrongGraph); !errors.Is(err, ErrInvalidMapping) {
		t.Fatalf("graph mismatch error = %v, want ErrInvalidMapping", err)
	}

	wrongIndex := base
	wrongIndex.ProposalIndex = 1
	if _, err := MaterializeObligationProposal(wrongIndex); !errors.Is(err, ErrInvalidMapping) {
		t.Fatalf("proposal index error = %v, want ErrInvalidMapping", err)
	}
}

func mappingActiveGraph(t *testing.T, snapshot cognition.RuntimeSnapshot) cognition.ObligationGraphSnapshot {
	t.Helper()
	current := snapshot.CurrentObligation()
	graph, err := cognition.NewObligationGraph(
		current.CreatedGeneration, current.ID,
		[]cognition.ObligationSpec{{
			ID: current.ID, ParentID: current.ParentID, Desired: current.Desired,
			DependsOn: current.DependsOn, SupportingRefs: current.SupportingRefs,
			CompletionCheck: current.CompletionCheck,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := graph.RefreshReadiness(current.CreatedGeneration); err != nil {
		t.Fatal(err)
	}
	if err := graph.Transition(current.ID, current.CreatedGeneration, cognition.ObligationActive); err != nil {
		t.Fatal(err)
	}
	return graph.Snapshot()
}
