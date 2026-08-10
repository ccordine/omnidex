package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/workingset"
)

func TestPostgresSatisfiedChildHandsCompletionEvidenceOnlyToParent(t *testing.T) {
	_, repository, _ := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	proposalAction, _ := prepareCognitionProposalAction(t, fixture)
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, proposalAction.Action.ID,
		cognitionProposalTransition(t, fixture, proposalAction), cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	graph, err := repository.CognitionObligationGraph(t.Context(), fixture.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	parent, child := materializedParentAndChild(t, graph.Graph, fixture.Start.Root.ID)
	if parent.Status != cognition.ObligationBlocked || child.Status != cognition.ObligationActive {
		t.Fatalf("materialized graph parent=%+v child=%+v", parent, child)
	}

	childAction := prepareCurrentCognitionAction(t, fixture)
	next, err := cognition.NewWorldRevision(
		fixture.EpisodeID, childAction.ExpectedRevision.Number+1, cognitionTestDigest("f"),
	)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := cognition.NewActionObservation(
		"child-completion-proof", childAction.Action.ID, next,
		"public_state", "The exact clue required by the dependent parent.",
	)
	if err != nil {
		t.Fatal(err)
	}
	unrelated, err := cognition.NewActionObservation(
		"child-unrelated-observation", childAction.Action.ID, next,
		"public_state", "A child-local detail unrelated to the dependent parent.",
	)
	if err != nil {
		t.Fatal(err)
	}
	transition := cognition.Transition{
		ActionID: childAction.Action.ID, Previous: cognitionRevisionPointer(childAction.ExpectedRevision),
		Current: next, Observations: []cognition.Observation{proof, unrelated},
		Effects: []cognition.Effect{}, Cost: 1,
	}
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, childAction.Action.ID, transition, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	prepared, err := repository.PrepareCognitionRuntimeSnapshot(
		t.Context(), CognitionRuntimeSnapshotCommand{
			Authority: fixture.Authority, EpisodeID: fixture.EpisodeID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Prepared.Snapshot.CurrentObligation().ID != child.ID {
		t.Fatalf("prepared obligation=%q want child %q", prepared.Prepared.Snapshot.CurrentObligation().ID, child.ID)
	}
	completion, err := cognition.NewCompletionResult(
		child.ID, child.CompletionCheck, next, cognition.CompletionSatisfied,
		[]cognition.EvidenceRef{proof.EvidenceRef()},
	)
	if err != nil {
		t.Fatal(err)
	}
	progress, err := repository.AdvanceCognitionRuntimeSatisfied(
		t.Context(), cognitionruntime.CompletionCommand{
			Binding: cognitionruntime.Binding{
				Episode: cognition.EpisodeRef{ID: fixture.EpisodeID},
				Attempt: prepared.Prepared.Snapshot.Attempt(),
			},
			SnapshotSHA256: prepared.Prepared.Snapshot.SHA256(), GraphVersion: prepared.Prepared.GraphVersion,
			ObligationGraph: prepared.Prepared.ObligationGraph,
			CompletionEvidenceRefs: append(
				[]cognition.EvidenceRef{}, prepared.Prepared.CompletionEvidenceRefs...,
			),
			Result: completion,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if progress.State != cognitionruntime.ProgressActive {
		t.Fatalf("progress=%+v", progress)
	}
	parent = obligationFromSnapshot(t, progress.ObligationGraph, parent.ID)
	if parent.Status != cognition.ObligationActive || !hasCognitionEvidence(parent.SupportingRefs, proof.EvidenceRef()) {
		t.Fatalf("parent did not receive exact completion proof: %+v", parent)
	}

	parentPrepared, err := repository.PrepareCognitionRuntimeSnapshot(
		t.Context(), CognitionRuntimeSnapshotCommand{
			Authority: fixture.Authority, EpisodeID: fixture.EpisodeID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !hasCognitionEvidence(parentPrepared.Prepared.Snapshot.EvidenceRefs(), proof.EvidenceRef()) ||
		hasCognitionEvidence(parentPrepared.Prepared.Snapshot.EvidenceRefs(), unrelated.EvidenceRef()) {
		t.Fatalf("parent clean desk evidence=%+v", parentPrepared.Prepared.Snapshot.EvidenceRefs())
	}
	assertCognitionHandoffMemberships(t, fixture, child.ID, parent.ID, proof, unrelated)
}

func prepareCurrentCognitionAction(
	t *testing.T,
	fixture cognitionDatabaseFixture,
) CognitionActionRecord {
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
	decision := cognition.CognitionDecision{
		ObligationID: snapshot.CurrentObligation().ID, Action: request,
		EvidenceRefs:   []cognition.EvidenceRef{fixture.Evidence},
		ExpectedEffect: "Expose the bounded child completion state.",
	}
	raw, _, err := cognitionJSON(decision)
	if err != nil {
		t.Fatal(err)
	}
	policy, err := cognitionpolicy.New(
		cognitionGuardPolicyClient{response: string(raw)}, cognitionTestBrain(),
		cognitionGuardProjectionLoader{repository: fixture.Repository},
		CognitionPolicyCallJournal{Repository: fixture.Repository},
	)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := cognition.NewCompletionResult(
		snapshot.CurrentObligation().ID, snapshot.CurrentObligation().CompletionCheck,
		snapshot.CurrentRevision(), cognition.CompletionUnsatisfied, []cognition.EvidenceRef{},
	)
	if err != nil {
		t.Fatal(err)
	}
	coordinator, err := cognition.NewCoordinator(policy)
	if err != nil {
		t.Fatal(err)
	}
	step, err := coordinator.Step(
		t.Context(), snapshot, completion, prepared.Prepared.CompletionEvidenceRefs,
	)
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
	return dispatched
}

func assertCognitionHandoffMemberships(
	t *testing.T,
	fixture cognitionDatabaseFixture,
	child, parent cognition.ObligationID,
	proof, unrelated cognition.Observation,
) {
	t.Helper()
	set, err := fixture.Repository.CurrentWorkingSet(t.Context(), fixture.Authority.JobID)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range set.Items {
		if item.Ref.Hash == proof.ContentSHA256 {
			if item.State != workingset.ItemResident || !hasWorkingSetScope(item, "obligation-"+string(parent)) ||
				hasWorkingSetScope(item, "obligation-"+string(child)) {
				t.Fatalf("proof membership=%+v", item)
			}
		}
		if item.Ref.Hash == unrelated.ContentSHA256 && item.State != workingset.ItemReleased {
			t.Fatalf("unrelated evidence remained resident: %+v", item)
		}
	}
}

func hasWorkingSetScope(item workingset.Item, scope string) bool {
	for _, membership := range item.Memberships {
		if string(membership.Scope.ID) == scope {
			return true
		}
	}
	return false
}

func hasCognitionEvidence(refs []cognition.EvidenceRef, want cognition.EvidenceRef) bool {
	for _, ref := range refs {
		if ref == want {
			return true
		}
	}
	return false
}

func obligationFromSnapshot(
	t *testing.T,
	graph cognition.ObligationGraphSnapshot,
	id cognition.ObligationID,
) cognition.Obligation {
	t.Helper()
	for _, obligation := range graph.Obligations {
		if obligation.ID == id {
			return obligation
		}
	}
	t.Fatalf("obligation %q is absent", id)
	return cognition.Obligation{}
}
