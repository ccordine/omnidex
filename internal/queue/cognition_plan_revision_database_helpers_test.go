package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func prepareCognitionPlanRevisionAction(
	t *testing.T,
	fixture cognitionDatabaseFixture,
) (CognitionActionRecord, cognitionProposalStep) {
	t.Helper()
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		t.Context(), CognitionRuntimeSnapshotCommand{
			Authority: fixture.Authority, EpisodeID: fixture.EpisodeID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := prepared.Prepared.Snapshot
	schema := fixture.Catalog.Schemas[0]
	request, err := cognition.NewActionRequest(schema.Kind, []cognition.ActionArgument{{
		Name: "target", Value: "artifact-1",
	}})
	if err != nil {
		t.Fatal(err)
	}
	next, err := cognition.NewGoalExpression([]cognition.Predicate{{
		Name: "prerequisite", Args: []string{"revised-artifact"},
	}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	decision := cognition.CognitionDecision{
		ObligationID: snapshot.CurrentObligation().ID, Action: request,
		EvidenceRefs:   []cognition.EvidenceRef{fixture.Evidence},
		ExpectedEffect: "Expose evidence for the revised bounded plan.",
		Proposals: []cognition.LedgerProposal{{
			Kind: cognition.ProposalPlanRevision,
			PlanRevision: &cognition.PlanRevisionProposal{
				Next: next, EvidenceRefs: []cognition.EvidenceRef{fixture.Evidence},
			},
		}},
	}
	response, _, err := cognitionJSON(decision)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := cognitionpolicy.New(
		cognitionGuardPolicyClient{response: string(response)}, cognitionTestBrain(),
		cognitionGuardActivationAuthorityFor(
			t, t.Context(), fixture.Repository, fixture.EpisodeID,
			fixture.Authority, fixture.Start.BrainBootstrap.AttestedBrain,
		),
		cognitionGuardProjectionLoader{repository: fixture.Repository},
		CognitionPolicyCallJournal{Repository: fixture.Repository},
	)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := cognition.NewCompletionResult(
		fixture.Start.Root.ID, fixture.Check, fixture.Start.Transition.Current,
		cognition.CompletionUnsatisfied, []cognition.EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := cognition.NewCoordinator(policy)
	if err != nil {
		t.Fatal(err)
	}
	step, err := coordinator.Step(t.Context(), snapshot, completion, snapshot.EvidenceRefs())
	if err != nil {
		t.Fatal(err)
	}
	command := cognitionruntime.ReconciliationCommand{
		Binding: cognitionruntime.Binding{
			Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: snapshot.Attempt(),
		},
		SnapshotSHA256: snapshot.SHA256(), Projection: snapshot.ContextProjection(),
		ActionSchema: schema, Decision: step.Decision.Clone(),
	}
	receipt, err := fixture.Repository.ReconcileCognitionRuntimeDecision(t.Context(), command)
	if err != nil {
		t.Fatal(err)
	}
	action, err := fixture.Repository.PrepareCognitionAction(
		t.Context(), cognitionruntime.PrepareActionCommand{
			Binding: command.Binding, Coordinator: step, Reconciliation: receipt,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	dispatched, err := fixture.Repository.DispatchCognitionAction(
		t.Context(), fixture.Authority, action.Action.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return dispatched, cognitionProposalStep{Prepared: prepared, Step: step, Command: command}
}

func planRevisionObligations(
	t *testing.T,
	graph cognition.ObligationGraphSnapshot,
) (cognition.Obligation, cognition.Obligation, cognition.Obligation) {
	t.Helper()
	var old, root, next cognition.Obligation
	for _, obligation := range graph.Obligations {
		switch {
		case obligation.CreatedGeneration < graph.Generation:
			old = obligation
		case obligation.ID == graph.RootID:
			root = obligation
		default:
			next = obligation
		}
	}
	if old.ID == "" || root.ID == "" || next.ID == "" || len(graph.Obligations) != 3 {
		t.Fatalf("revised obligations = %+v", graph.Obligations)
	}
	return old, root, next
}
