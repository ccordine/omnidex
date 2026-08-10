package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

type cognitionProposalStep struct {
	Prepared CognitionRuntimeSnapshotRecord
	Step     cognition.CoordinatorStep
	Command  cognitionruntime.ReconciliationCommand
}

func buildCognitionProposalStep(
	t *testing.T,
	fixture cognitionDatabaseFixture,
	predicate cognition.PredicateName,
) cognitionProposalStep {
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
	desired, err := cognition.NewGoalExpression([]cognition.Predicate{{
		Name: predicate, Args: []string{"artifact-1"},
	}}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	decision := cognition.CognitionDecision{
		ObligationID: fixture.Start.Root.ID, Action: request,
		EvidenceRefs:   []cognition.EvidenceRef{fixture.Evidence},
		ExpectedEffect: "Expose the bounded prerequisite state.",
		Proposals: []cognition.LedgerProposal{{
			Kind: cognition.ProposalObligation,
			Obligation: &cognition.ObligationProposal{
				Desired: desired, EvidenceRefs: []cognition.EvidenceRef{fixture.Evidence},
			},
		}},
	}
	response, _, err := cognitionJSON(decision)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := cognitionpolicy.New(
		cognitionGuardPolicyClient{response: string(response)},
		cognitionTestBrain(),
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
	if step.Decision == nil {
		t.Fatal("proposal policy produced no decision")
	}
	command := cognitionruntime.ReconciliationCommand{
		Binding: cognitionruntime.Binding{
			Episode: cognition.EpisodeRef{ID: fixture.EpisodeID}, Attempt: snapshot.Attempt(),
		},
		SnapshotSHA256: snapshot.SHA256(), Projection: snapshot.ContextProjection(),
		ActionSchema: schema, Decision: step.Decision.Clone(),
	}
	return cognitionProposalStep{Prepared: prepared, Step: step, Command: command}
}

func prepareCognitionProposalAction(
	t *testing.T,
	fixture cognitionDatabaseFixture,
) (CognitionActionRecord, cognitionProposalStep) {
	t.Helper()
	proposal := buildCognitionProposalStep(t, fixture, "prerequisite")
	receipt, err := fixture.Repository.ReconcileCognitionRuntimeDecision(t.Context(), proposal.Command)
	if err != nil {
		t.Fatal(err)
	}
	action, err := fixture.Repository.PrepareCognitionAction(
		t.Context(), cognitionruntime.PrepareActionCommand{
			Binding: proposal.Command.Binding, Coordinator: proposal.Step, Reconciliation: receipt,
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
	return dispatched, proposal
}

func cognitionProposalTransition(
	t *testing.T,
	fixture cognitionDatabaseFixture,
	action CognitionActionRecord,
) cognition.Transition {
	t.Helper()
	next, err := cognition.NewWorldRevision(
		fixture.EpisodeID, action.ExpectedRevision.Number+1, cognitionTestDigest("e"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return cognition.Transition{
		ActionID: action.Action.ID, Previous: cognitionRevisionPointer(action.ExpectedRevision),
		Current: next, Observations: []cognition.Observation{}, Effects: []cognition.Effect{}, Cost: 1,
	}
}
