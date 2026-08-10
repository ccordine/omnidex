package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPostgresCognitionPlanRevisionCompletesAndSealsOnlyReplacementRoot(t *testing.T) {
	_, repository, _ := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	if _, err := repository.StartCognitionEpisode(
		t.Context(), fixture.Start, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	revisionAction, _ := prepareCognitionPlanRevisionAction(t, fixture)
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, revisionAction.Action.ID,
		cognitionProposalTransition(t, fixture, revisionAction), cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	revised, err := repository.CognitionObligationGraph(t.Context(), fixture.EpisodeID)
	if err != nil {
		t.Fatal(err)
	}
	old, root, next := planRevisionObligations(t, revised.Graph)

	terminalAction := prepareCurrentCognitionAction(t, fixture)
	terminalRevision, err := cognition.NewWorldRevision(
		fixture.EpisodeID, terminalAction.ExpectedRevision.Number+1, cognitionTestDigest("8"),
	)
	if err != nil {
		t.Fatal(err)
	}
	proof, err := cognition.NewActionObservation(
		"plan-revision-terminal-proof", terminalAction.Action.ID, terminalRevision,
		"public_state", "The code-owned completion predicate is satisfied.",
	)
	if err != nil {
		t.Fatal(err)
	}
	publicOutcome := "The revised cognition goal is satisfied."
	transition := cognition.Transition{
		ActionID: terminalAction.Action.ID,
		Previous: cognitionRevisionPointer(terminalAction.ExpectedRevision), Current: terminalRevision,
		Observations: []cognition.Observation{proof}, Effects: []cognition.Effect{}, Cost: 1,
		Terminal: true, PublicOutcome: publicOutcome,
	}
	if _, err := repository.IngestCognitionTransition(
		t.Context(), fixture.Authority, terminalAction.Action.ID, transition, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}

	nextProgress := satisfyPreparedPlanObligation(
		t, fixture, next.ID, next.CompletionCheck, proof.EvidenceRef(), publicOutcome,
	)
	if nextProgress.State != cognitionruntime.ProgressActive ||
		nextProgress.ObligationGraph.RootID != root.ID {
		t.Fatalf("replacement prerequisite progress=%+v", nextProgress)
	}
	current := obligationFromSnapshot(t, nextProgress.ObligationGraph, root.ID)
	if current.Status != cognition.ObligationActive {
		t.Fatalf("replacement root was not activated: %+v", current)
	}
	oldAfter := obligationFromSnapshot(t, nextProgress.ObligationGraph, old.ID)
	if oldAfter.Status != cognition.ObligationSuperseded {
		t.Fatalf("superseded generation-one root reopened: %+v", oldAfter)
	}

	rootProgress := satisfyPreparedPlanObligation(
		t, fixture, root.ID, root.CompletionCheck, proof.EvidenceRef(), publicOutcome,
	)
	if rootProgress.State != cognitionruntime.ProgressCompleted || rootProgress.Completion == nil ||
		rootProgress.ObligationGraph.RootID != root.ID {
		t.Fatalf("replacement root terminal progress=%+v", rootProgress)
	}
	seal, err := repository.SealCognitionEpisode(t.Context(), CognitionTerminalCommand{
		Authority: fixture.Authority, EpisodeID: fixture.EpisodeID,
		Outcome: CognitionEpisodeCompleted, GraphVersion: rootProgress.GraphVersion,
		Completion: *rootProgress.Completion, ObligationGraph: rootProgress.ObligationGraph,
		PublicOutcome: publicOutcome, ExpectedRevision: terminalRevision,
	})
	if err != nil {
		t.Fatal(err)
	}
	if seal.Outcome != CognitionEpisodeCompleted ||
		seal.ObligationGraphSHA256 != rootProgress.ObligationGraph.SHA256 {
		t.Fatalf("replacement root seal=%+v", seal)
	}
}

func satisfyPreparedPlanObligation(
	t *testing.T,
	fixture cognitionDatabaseFixture,
	obligationID cognition.ObligationID,
	check cognition.CompletionCheckRef,
	proof cognition.EvidenceRef,
	publicOutcome string,
) cognitionruntime.EpisodeProgress {
	t.Helper()
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		t.Context(), CognitionRuntimeSnapshotCommand{
			Authority: fixture.Authority, EpisodeID: fixture.EpisodeID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Prepared.Snapshot.CurrentObligation().ID != obligationID ||
		!prepared.Prepared.EnvironmentTerminal || prepared.Prepared.PublicOutcome != publicOutcome {
		t.Fatalf("prepared terminal obligation=%+v", prepared.Prepared)
	}
	completion, err := cognition.NewCompletionResult(
		obligationID, check, prepared.Prepared.Snapshot.CurrentRevision(),
		cognition.CompletionSatisfied, []cognition.EvidenceRef{proof},
	)
	if err != nil {
		t.Fatal(err)
	}
	progress, err := fixture.Repository.AdvanceCognitionRuntimeSatisfied(
		t.Context(), cognitionruntime.CompletionCommand{
			Binding: cognitionruntime.Binding{
				Episode: cognition.EpisodeRef{ID: fixture.EpisodeID},
				Attempt: prepared.Prepared.Snapshot.Attempt(),
			},
			SnapshotSHA256:  prepared.Prepared.Snapshot.SHA256(),
			GraphVersion:    prepared.Prepared.GraphVersion,
			ObligationGraph: prepared.Prepared.ObligationGraph,
			CompletionEvidenceRefs: append(
				[]cognition.EvidenceRef{}, prepared.Prepared.CompletionEvidenceRefs...,
			),
			Result: completion, EnvironmentTerminal: true, PublicOutcome: publicOutcome,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return progress
}
