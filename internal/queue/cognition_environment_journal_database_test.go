package queue

import (
	"errors"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPostgresCognitionEnvironmentJournalExactReplayAndTerminalFence(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "environment-terminal")
	episode := cognition.EpisodeRef{ID: fixture.EpisodeID}
	if _, err := fixture.Repository.StartCognitionEnvironment(
		fixture.Context, episode, fixture.Start.Scenario, fixture.Start.Transition,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Repository.StartCognitionEnvironment(
		fixture.Context, episode, fixture.Start.Scenario, fixture.Start.Transition,
	); err != nil {
		t.Fatalf("exact start replay: %v", err)
	}
	changed := fixture.Start.Transition.Clone()
	changed.PublicOutcome = "Changed exact start authority."
	if _, err := fixture.Repository.StartCognitionEnvironment(
		fixture.Context, episode, fixture.Start.Scenario, changed,
	); !errors.Is(err, cognition.ErrEnvironmentJournalConflict) {
		t.Fatalf("changed start error=%v, want conflict", err)
	}
	action := prepareCognitionGuardAction(t, fixture, "environment-terminal")
	receipt := environmentTransitionReceipt(t, episode, action, true)
	stored, err := fixture.Repository.CommitCognitionEnvironmentAction(
		fixture.Context, episode, fixture.Start.Scenario, receipt,
	)
	if err != nil || stored.Transition == nil || !stored.Transition.Terminal {
		t.Fatalf("terminal commit=%+v error=%v", stored, err)
	}
	replayed, err := fixture.Repository.CommitCognitionEnvironmentAction(
		fixture.Context, episode, fixture.Start.Scenario, receipt,
	)
	if err != nil || replayed.Transition == nil || replayed.Transition.Current != receipt.Transition.Current {
		t.Fatalf("terminal replay=%+v error=%v", replayed, err)
	}
	changedReceipt := receipt.Clone()
	changedReceipt.Action.Actor.WorkerID = "changed-worker"
	if _, err := fixture.Repository.CommitCognitionEnvironmentAction(
		fixture.Context, episode, fixture.Start.Scenario, changedReceipt,
	); !errors.Is(err, cognition.ErrEnvironmentJournalConflict) {
		t.Fatalf("changed action replay error=%v, want conflict", err)
	}
	newAction := receipt.Action.Clone()
	newAction.ID = "after-terminal"
	if _, _, err := fixture.Repository.ReviewCognitionEnvironmentAction(
		fixture.Context, episode, fixture.Start.Scenario, receipt.Transition.Current, newAction,
	); !errors.Is(err, cognition.ErrEnvironmentJournalTerminal) {
		t.Fatalf("post-terminal action error=%v, want terminal fence", err)
	}
}

func TestPostgresCognitionEnvironmentJournalConcurrentReplayCommitsOneReceipt(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "environment-concurrent")
	episode := cognition.EpisodeRef{ID: fixture.EpisodeID}
	if _, err := fixture.Repository.StartCognitionEnvironment(
		fixture.Context, episode, fixture.Start.Scenario, fixture.Start.Transition,
	); err != nil {
		t.Fatal(err)
	}
	action := prepareCognitionGuardAction(t, fixture, "environment-concurrent")
	receipt := environmentTransitionReceipt(t, episode, action, true)
	errorsByIndex := make([]error, 2)
	var wait sync.WaitGroup
	for index := range errorsByIndex {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			_, errorsByIndex[index] = fixture.Repository.CommitCognitionEnvironmentAction(
				fixture.Context, episode, fixture.Start.Scenario, receipt,
			)
		}(index)
	}
	wait.Wait()
	for _, err := range errorsByIndex {
		if err != nil {
			t.Fatalf("concurrent exact replay failed: %v", err)
		}
	}
	var receiptCount, sequence int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT (SELECT COUNT(*) FROM cognition_environment_receipts WHERE episode_id=$1),
		       commit_sequence FROM cognition_environment_journals WHERE episode_id=$1
	`, fixture.EpisodeID).Scan(&receiptCount, &sequence); err != nil {
		t.Fatal(err)
	}
	state, err := fixture.Repository.CognitionEnvironmentState(
		fixture.Context, episode, fixture.Start.Scenario,
	)
	if err != nil || receiptCount != 1 || sequence != 1 || !state.Terminal || state.TerminalReceipt == nil {
		t.Fatalf("receipt/sequence/state=%d/%d/%+v error=%v", receiptCount, sequence, state, err)
	}
}

func TestPostgresCognitionEnvironmentCommitFencesReplacedAttempt(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	fixture := startTaskGenerationRetirementFixtureIn(
		t, repository, pool, t.Context(), "environment-stale-commit",
	)
	episode := cognition.EpisodeRef{ID: fixture.EpisodeID}
	if _, err := fixture.Repository.StartCognitionEnvironment(
		fixture.Context, episode, fixture.Start.Scenario, fixture.Start.Transition,
	); err != nil {
		t.Fatal(err)
	}
	action := prepareCognitionGuardAction(t, fixture, "environment-stale-commit")
	receipt := environmentTransitionReceipt(t, episode, action, true)
	replacement := replaceCognitionAttemptForTest(t, fixture.Pool, fixture.Authority)
	if _, err := fixture.Repository.CommitCognitionEnvironmentAction(
		fixture.Context, episode, fixture.Start.Scenario, receipt,
	); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("stale environment commit error=%v, want ErrStaleStepAttempt", err)
	}
	var receiptCount, sequence int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT (SELECT COUNT(*) FROM cognition_environment_receipts WHERE episode_id=$1),
		       commit_sequence FROM cognition_environment_journals WHERE episode_id=$1
	`, fixture.EpisodeID).Scan(&receiptCount, &sequence); err != nil {
		t.Fatal(err)
	}
	if receiptCount != 0 || sequence != 0 {
		t.Fatalf("stale actor advanced environment journal receipts=%d sequence=%d", receiptCount, sequence)
	}
	reauthorized := action
	reauthorizedAction, err := action.ActionFor(replacement)
	if err != nil {
		t.Fatal(err)
	}
	reauthorized.Action = reauthorizedAction
	replacementReceipt := environmentTransitionReceipt(t, episode, reauthorized, true)
	if _, err := fixture.Repository.CommitCognitionEnvironmentAction(
		fixture.Context, episode, fixture.Start.Scenario, replacementReceipt,
	); err != nil {
		t.Fatalf("replacement environment commit: %v", err)
	}
}

func TestPostgresCognitionEnvironmentJournalTerminalStartFencesApply(t *testing.T) {
	_, repository, _ := openWorkingSetDatabase(t)
	episode, _ := cognition.NewEpisodeRef("terminal-start-episode")
	scenario, _ := cognition.NewScenarioRef("terminal-start-scenario", cognitionTestDigest("7"))
	revision, _ := cognition.NewWorldRevision(episode.ID, 1, cognitionTestDigest("8"))
	start := cognition.Transition{
		Current: revision, Observations: []cognition.Observation{}, Effects: []cognition.Effect{},
		Terminal: true, PublicOutcome: "The environment began in its terminal state.",
	}
	if _, err := repository.StartCognitionEnvironment(t.Context(), episode, scenario, start); err != nil {
		t.Fatal(err)
	}
	state, err := repository.CognitionEnvironmentState(t.Context(), episode, scenario)
	if err != nil || !state.Terminal || state.TerminalReceipt != nil {
		t.Fatalf("terminal start state=%+v error=%v", state, err)
	}
	schema, _ := cognition.NewActionSchema(
		"terminal-start.inspect", "1.0.0", "inspect", []cognition.ActionParameterSpec{}, cognition.EvidenceOptional,
	)
	request, _ := cognition.NewActionRequest(schema.Kind, []cognition.ActionArgument{})
	action, _ := cognition.NewRegisteredAction(
		"terminal-start-action", cognition.AttemptRef{
			JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "terminal-start-worker",
		}, schema, request, []cognition.EvidenceRef{},
	)
	if _, _, err := repository.ReviewCognitionEnvironmentAction(
		t.Context(), episode, scenario, revision, action,
	); !errors.Is(err, cognition.ErrEnvironmentJournalTerminal) {
		t.Fatalf("terminal-start action error=%v, want terminal fence", err)
	}
}

func TestPostgresCognitionEnvironmentRejectsForgedStartProjection(t *testing.T) {
	_, _, pool := openWorkingSetDatabase(t)
	realEpisode, _ := cognition.NewEpisodeRef("real-start-episode")
	revision, _ := cognition.NewWorldRevision(realEpisode.ID, 1, cognitionTestDigest("4"))
	start := cognition.Transition{
		Current: revision, Observations: []cognition.Observation{}, Effects: []cognition.Effect{},
	}
	raw, sha, err := cognitionJSON(start)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO cognition_environment_journals (
			episode_id,scenario_id,scenario_sha256,start_json,start_sha256,
			current_revision,current_revision_sha256,terminal
		) VALUES ('forged-start-episode','forged-scenario',$1,$2,$3,1,$4,FALSE)
	`, cognitionTestDigest("5"), string(raw), sha, revision.SHA256); err == nil {
		t.Fatal("forged immutable start projection was accepted")
	}
}
