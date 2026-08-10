package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPostgresCognitionCompletesOnlyAfterTerminalTransitionAndSatisfiedGraph(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "terminal-completion")
	action := prepareCognitionGuardAction(t, fixture, "terminal-completion")
	next, err := cognition.NewWorldRevision(fixture.EpisodeID, 2, cognitionTestDigest("9"))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := cognition.NewActionObservation(
		"observation-terminal-completion", action.Action.ID, next,
		"public_state", "The registered completion predicate is satisfied.",
	)
	if err != nil {
		t.Fatal(err)
	}
	effect, err := cognition.NewEffect(
		action.Action.ID, next, cognition.EffectStateChanged,
		"The registered public state changed.",
	)
	if err != nil {
		t.Fatal(err)
	}
	transition := cognition.Transition{
		ActionID: action.Action.ID, Previous: cognitionRevisionPointer(action.ExpectedRevision),
		Current: next, Observations: []cognition.Observation{observation},
		Effects: []cognition.Effect{effect}, Cost: 1, Terminal: true,
		PublicOutcome: "The registered goal predicate is satisfied.",
	}
	succeeded, err := fixture.Repository.IngestCognitionTransition(
		fixture.Context, fixture.Authority, action.Action.ID, transition, cognitionTestFactAuthority(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if succeeded.Status != CognitionActionSucceeded || succeeded.ResultRevision == nil ||
		*succeeded.ResultRevision != next {
		t.Fatalf("succeeded cognition action=%+v", succeeded)
	}
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		fixture.Context, CognitionRuntimeSnapshotCommand{
			Authority: fixture.Authority, EpisodeID: fixture.EpisodeID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	completion, err := cognition.NewCompletionResult(
		fixture.Start.Root.ID, fixture.Start.Root.CompletionCheck, next,
		cognition.CompletionSatisfied, []cognition.EvidenceRef{observation.EvidenceRef()},
	)
	if err != nil {
		t.Fatal(err)
	}
	progress, err := fixture.Repository.AdvanceCognitionRuntimeSatisfied(
		fixture.Context, cognitionruntime.CompletionCommand{
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
			Result: completion, EnvironmentTerminal: true,
			PublicOutcome: transition.PublicOutcome,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if progress.State != cognitionruntime.ProgressCompleted || progress.Completion == nil {
		t.Fatalf("terminal progress=%+v", progress)
	}
	seal, err := fixture.Repository.SealCognitionEpisode(fixture.Context, CognitionTerminalCommand{
		Authority: fixture.Authority, EpisodeID: fixture.EpisodeID,
		Outcome: CognitionEpisodeCompleted, GraphVersion: progress.GraphVersion,
		Completion: *progress.Completion, ObligationGraph: progress.ObligationGraph,
		PublicOutcome: transition.PublicOutcome, ExpectedRevision: next,
	})
	if err != nil {
		t.Fatal(err)
	}
	if seal.Outcome != CognitionEpisodeCompleted || seal.FinalRevision != next || seal.TraceSHA256 == "" {
		t.Fatalf("completed cognition seal=%+v", seal)
	}
	var episodeStatus string
	var actionCount, transitionCount, sealCount int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT episodes.status,
		       (SELECT COUNT(*) FROM cognition_actions WHERE episode_id=episodes.episode_id),
		       (SELECT COUNT(*) FROM cognition_transitions WHERE episode_id=episodes.episode_id),
		       (SELECT COUNT(*) FROM cognition_terminal_seals WHERE episode_id=episodes.episode_id)
		FROM cognition_episodes episodes WHERE episodes.episode_id=$1
	`, fixture.EpisodeID).Scan(&episodeStatus, &actionCount, &transitionCount, &sealCount); err != nil {
		t.Fatal(err)
	}
	if episodeStatus != string(CognitionEpisodeCompleted) || actionCount != 1 ||
		transitionCount != 2 || sealCount != 1 {
		t.Fatalf("terminal projection=%q actions=%d transitions=%d seals=%d",
			episodeStatus, actionCount, transitionCount, sealCount)
	}
	assertSealedWorkingSetTrace(t, fixture, seal)
}
