package queue

import (
	"strconv"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/taskstate"
)

type cognitionRevisionFixture struct {
	Database cognitionDatabaseFixture
	Target   cognition.EpistemicRef
	Evidence cognition.EvidenceRef
}

type cognitionBoundDecision struct {
	Prepared CognitionRuntimeSnapshotRecord
	Step     cognition.CoordinatorStep
	Command  cognitionruntime.ReconciliationCommand
}

func startCognitionRevisionFixture(t *testing.T) cognitionRevisionFixture {
	t.Helper()
	_, repository, _ := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	first := cognition.CognitionDecision{
		ObligationID: fixture.Start.Root.ID,
		Action: cognition.ActionRequest{
			Kind:      fixture.Catalog.Schemas[0].Kind,
			Arguments: []cognition.ActionArgument{{Name: "target", Value: "artifact-1"}},
		},
		EvidenceRefs:   []cognition.EvidenceRef{fixture.Evidence},
		ExpectedEffect: "Inspect the exact current mechanism.",
		Proposals: []cognition.LedgerProposal{{
			Kind: cognition.ProposalHypothesis, Content: "The first mechanism remains available.",
			EvidenceRefs: []cognition.EvidenceRef{fixture.Evidence},
		}},
		Attention: []cognition.AttentionRequest{},
	}
	action := prepareCognitionDecisionAction(t, fixture, first)
	next, err := cognition.NewWorldRevision(
		fixture.EpisodeID, 2, cognitionTestDigest("7"),
	)
	if err != nil {
		t.Fatal(err)
	}
	observation, err := cognition.NewActionObservation(
		"observation-contradiction", action.Action.ID, next, "public_state",
		"The current state proves that the first mechanism is unavailable.",
	)
	if err != nil {
		t.Fatal(err)
	}
	transition := cognition.Transition{
		ActionID: action.Action.ID, Previous: cognitionRevisionPointer(action.ExpectedRevision),
		Current: next, Observations: []cognition.Observation{observation},
		Effects: []cognition.Effect{}, Cost: 1,
	}
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, action.Action.ID, transition, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	state, err := repository.TaskLedger(t.Context(), fixture.Authority.JobID)
	if err != nil {
		t.Fatal(err)
	}
	var hypothesis taskstate.Entry
	for _, entry := range state.Entries {
		if entry.Kind == taskstate.EntryHypothesis && entry.Authority == taskstate.AuthorityModelProposal {
			hypothesis = entry
		}
	}
	if hypothesis.ID == "" {
		t.Fatal("initial policy decision did not persist its hypothesis candidate")
	}
	return cognitionRevisionFixture{
		Database: fixture, Evidence: observation.EvidenceRef(),
		Target: cognition.EpistemicRef{
			URI:     "task:ledger/" + string(state.ID) + "/entry/" + string(hypothesis.ID),
			Version: strconv.FormatUint(hypothesis.UpdatedVersion, 10), SHA256: hypothesis.ContentSHA256,
		},
	}
}

func prepareCognitionDecisionAction(
	t *testing.T,
	fixture cognitionDatabaseFixture,
	decision cognition.CognitionDecision,
) CognitionActionRecord {
	t.Helper()
	bound := buildCognitionDecisionStep(t, fixture, decision)
	receipt, err := fixture.Repository.ReconcileCognitionRuntimeDecision(t.Context(), bound.Command)
	if err != nil {
		t.Fatal(err)
	}
	action, err := fixture.Repository.PrepareCognitionAction(
		t.Context(), cognitionruntime.PrepareActionCommand{
			Binding: bound.Command.Binding, Coordinator: bound.Step, Reconciliation: receipt,
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
	return dispatched
}

func buildCognitionDecisionStep(
	t *testing.T,
	fixture cognitionDatabaseFixture,
	decision cognition.CognitionDecision,
) cognitionBoundDecision {
	t.Helper()
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		t.Context(), CognitionRuntimeSnapshotCommand{
			Authority: fixture.Authority, EpisodeID: fixture.EpisodeID,
		},
	)
	if err != nil {
		t.Fatal(err)
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
		fixture.Start.Root.ID, fixture.Check, prepared.Prepared.Snapshot.CurrentRevision(),
		cognition.CompletionUnsatisfied, []cognition.EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := cognition.NewCoordinator(policy)
	if err != nil {
		t.Fatal(err)
	}
	step, err := coordinator.Step(
		t.Context(), prepared.Prepared.Snapshot, completion,
		prepared.Prepared.CompletionEvidenceRefs,
	)
	if err != nil || step.Decision == nil {
		t.Fatalf("policy decision step=%#v error=%v", step, err)
	}
	command := cognitionruntime.ReconciliationCommand{
		Binding: cognitionruntime.Binding{
			Episode: cognition.EpisodeRef{ID: fixture.EpisodeID},
			Attempt: prepared.Prepared.Snapshot.Attempt(),
		},
		SnapshotSHA256: prepared.Prepared.Snapshot.SHA256(),
		Projection:     prepared.Prepared.Snapshot.ContextProjection(),
		ActionSchema:   fixture.Catalog.Schemas[0], Decision: step.Decision.Clone(),
	}
	return cognitionBoundDecision{Prepared: prepared, Step: step, Command: command}
}

func cognitionRevisionDecision(fixture cognitionRevisionFixture) cognition.CognitionDecision {
	return cognition.CognitionDecision{
		ObligationID: fixture.Database.Start.Root.ID,
		Action: cognition.ActionRequest{
			Kind:      fixture.Database.Catalog.Schemas[0].Kind,
			Arguments: []cognition.ActionArgument{{Name: "target", Value: "artifact-1"}},
		},
		EvidenceRefs:   []cognition.EvidenceRef{fixture.Evidence},
		ExpectedEffect: "Inspect the revised mechanism state.",
		Proposals: []cognition.LedgerProposal{{
			Kind: cognition.ProposalRevision,
			Revision: &cognition.BeliefRevisionProposal{
				TargetRef: fixture.Target, EvidenceRefs: []cognition.EvidenceRef{fixture.Evidence},
			},
		}},
		Attention: []cognition.AttentionRequest{},
	}
}
