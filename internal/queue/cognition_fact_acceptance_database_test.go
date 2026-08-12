package queue

import (
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/cognitionstate"
)

func TestPostgresCognitionStartAtomicallyAcceptsRegisteredFactsAndReplays(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	facts := cognitionFactAuthorityForTest(t, planFirstCognitionObservation, cognitionTestDigest("e"))
	episode, err := repository.StartCognitionEpisode(ctx, fixture.Start, facts)
	if err != nil {
		t.Fatal(err)
	}
	if episode.FactAuthority.SHA256 != facts.Reference().SHA256 {
		t.Fatalf("episode fact authority=%+v", episode.FactAuthority)
	}
	state, err := repository.TaskLedger(ctx, fixture.Authority.JobID)
	if err != nil {
		t.Fatal(err)
	}
	factsFound := 0
	for _, entry := range state.Entries {
		if entry.Kind == "fact" && entry.Content == "Accepted fact: A bounded public fact is visible." {
			factsFound++
			if len(entry.Refs) != 1 || entry.Refs[0].Hash != fixture.Evidence.SHA256 {
				t.Fatalf("accepted fact lineage=%+v", entry.Refs)
			}
		}
	}
	var normalized, evidence, policies int
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM cognition_accepted_facts WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_accepted_fact_evidence evidence
		        JOIN cognition_accepted_facts facts ON facts.fact_id=evidence.fact_id
		        WHERE facts.episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_episode_fact_policies WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&normalized, &evidence, &policies); err != nil {
		t.Fatal(err)
	}
	if factsFound != 1 || normalized != 1 || evidence != 1 || policies != 1 {
		t.Fatalf("fact ledger/normalized/evidence/policies=%d/%d/%d/%d", factsFound, normalized, evidence, policies)
	}
	replay := fixture.Start
	replay.BrainBootstrap = freshReplayBrainBootstrap(t, fixture.Start.BrainBootstrap)
	replay.ProviderProcessActivation = cognitionGuardProviderProcessActivationFor(
		t, ctx, fixture.EpisodeID, fixture.Authority,
		replay.BrainBootstrap.AttestedBrain,
	)
	if _, err := repository.StartCognitionEpisode(ctx, replay, facts); err != nil {
		t.Fatalf("exact fact authority replay: %v", err)
	}
	changed := cognitionFactAuthorityForTest(t, planFirstCognitionObservation, cognitionTestDigest("f"))
	if _, err := repository.StartCognitionEpisode(ctx, fixture.Start, changed); !errors.Is(err, ErrCognitionConflict) {
		t.Fatalf("changed fact authority replay error=%v", err)
	}
}

func TestPostgresCognitionFactPlannerFailureRollsBackStartAndTransition(t *testing.T) {
	_, repository, pool := openWorkingSetDatabase(t)
	fixture := newCognitionDatabaseFixture(t, repository)
	failingStart := cognitionFactAuthorityForTest(t, func(cognition.Transition) ([]cognitionstate.FactPlan, error) {
		return nil, errors.New("registered start fact planner failed")
	}, cognitionTestDigest("7"))
	if _, err := repository.StartCognitionEpisode(t.Context(), fixture.Start, failingStart); err == nil {
		t.Fatal("failed fact planner committed episode start")
	}
	var episodes, transitions int
	if err := pool.QueryRow(t.Context(), `
		SELECT (SELECT COUNT(*) FROM cognition_episodes WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM cognition_transitions WHERE episode_id=$1)
	`, fixture.EpisodeID).Scan(&episodes, &transitions); err != nil {
		t.Fatal(err)
	}
	if episodes != 0 || transitions != 0 {
		t.Fatalf("failed start committed episodes/transitions=%d/%d", episodes, transitions)
	}

	repository, pool, ctx := replanTestRepository(t)
	actionFixture := prepareTaskGenerationRetirementFixture(t, repository, pool, ctx, "fact-rollback")
	failingApply := cognitionFactAuthorityForTest(t, func(transition cognition.Transition) ([]cognitionstate.FactPlan, error) {
		if transition.Current.Number == 1 {
			return []cognitionstate.FactPlan{}, nil
		}
		return nil, errors.New("registered transition fact planner failed")
	}, cognitionTestDigest("8"))
	if _, err := repository.StartCognitionEpisode(ctx, actionFixture.Start, failingApply); err != nil {
		t.Fatal(err)
	}
	action := prepareCognitionGuardAction(t, actionFixture, "fact-rollback")
	next, err := cognition.NewWorldRevision(actionFixture.EpisodeID, 2, cognitionTestDigest("9"))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := cognition.NewActionObservation(
		"fact-rollback-observation", action.Action.ID, next, "public_state", "Must roll back.",
	)
	if err != nil {
		t.Fatal(err)
	}
	transition := cognition.Transition{
		ActionID: action.Action.ID, Previous: cognitionRevisionPointer(action.ExpectedRevision),
		Current: next, Observations: []cognition.Observation{observation}, Effects: []cognition.Effect{},
	}
	if _, err := repository.IngestCognitionTransition(
		ctx, actionFixture.Authority, action.Action.ID, transition, failingApply,
	); err == nil {
		t.Fatal("failed transition fact planner committed mutation")
	}
	var status string
	var revision, transitionCount, factCount int
	if err := pool.QueryRow(ctx, `
		SELECT actions.status,episodes.current_revision,
		       (SELECT COUNT(*) FROM cognition_transitions WHERE episode_id=episodes.episode_id),
		       (SELECT COUNT(*) FROM cognition_accepted_facts WHERE episode_id=episodes.episode_id)
		FROM cognition_actions actions JOIN cognition_episodes episodes USING (episode_id)
		WHERE actions.action_id=$1
	`, action.Action.ID).Scan(&status, &revision, &transitionCount, &factCount); err != nil {
		t.Fatal(err)
	}
	if status != "dispatched" || revision != 1 || transitionCount != 1 || factCount != 0 {
		t.Fatalf("failed apply state=%s revision=%d transitions=%d facts=%d", status, revision, transitionCount, factCount)
	}
}

func TestPostgresCognitionTransitionAtomicallyAcceptsRegisteredFactAndReplays(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(t, repository, pool, ctx, "fact-success")
	facts := cognitionFactAuthorityForTest(t, func(transition cognition.Transition) ([]cognitionstate.FactPlan, error) {
		if transition.Current.Number != 2 {
			return []cognitionstate.FactPlan{}, nil
		}
		return planFirstCognitionObservation(transition)
	}, cognitionTestDigest("6"))
	if _, err := repository.StartCognitionEpisode(ctx, fixture.Start, facts); err != nil {
		t.Fatal(err)
	}
	action := prepareCognitionGuardAction(t, fixture, "fact-success")
	next, err := cognition.NewWorldRevision(fixture.EpisodeID, 2, cognitionTestDigest("5"))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := cognition.NewActionObservation(
		"fact-success-observation", action.Action.ID, next,
		"public_state", "A second-revision fact is visible.",
	)
	if err != nil {
		t.Fatal(err)
	}
	transition := cognition.Transition{
		ActionID: action.Action.ID, Previous: cognitionRevisionPointer(action.ExpectedRevision),
		Current: next, Observations: []cognition.Observation{observation}, Effects: []cognition.Effect{},
	}
	if _, err := repository.IngestCognitionTransition(
		ctx, fixture.Authority, action.Action.ID, transition, facts,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.IngestCognitionTransition(
		ctx, fixture.Authority, action.Action.ID, transition, facts,
	); err != nil {
		t.Fatalf("exact transition fact replay: %v", err)
	}
	var normalized, entries int
	var factSHA string
	if err := pool.QueryRow(ctx, `
		SELECT (SELECT COUNT(*) FROM cognition_accepted_facts WHERE episode_id=$1),
		       (SELECT COUNT(*) FROM task_entries entries
		        JOIN cognition_episodes episodes ON episodes.ledger_id=entries.ledger_id
		        WHERE episodes.episode_id=$1 AND entries.kind='fact' AND entries.status='active')
	`, fixture.EpisodeID).Scan(&normalized, &entries); err != nil {
		t.Fatal(err)
	}
	if normalized != 1 || entries != 1 {
		t.Fatalf("transition accepted fact normalized/entries=%d/%d", normalized, entries)
	}
	if err := pool.QueryRow(ctx, `
		SELECT entries.content_sha256 FROM task_entries entries
		JOIN cognition_episodes episodes ON episodes.ledger_id=entries.ledger_id
		WHERE episodes.episode_id=$1 AND entries.kind='fact' AND entries.status='active'
	`, fixture.EpisodeID).Scan(&factSHA); err != nil {
		t.Fatal(err)
	}
	prepared, err := repository.PrepareCognitionRuntimeSnapshot(
		ctx, CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := repository.GetContextProjection(
		ctx, string(prepared.Prepared.Snapshot.ContextProjection().ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	visible, rawVisible := false, false
	for _, selected := range projection.Projection.Selected {
		if selected.Role == "fact" && selected.ContentSHA256 == factSHA {
			visible = true
		}
		if selected.Ref == cognitionEvidenceTaskRefs([]cognition.EvidenceRef{observation.EvidenceRef()})[0] {
			rawVisible = true
		}
	}
	if visible || !rawVisible {
		t.Fatalf("source revision clean desk fact/raw=%v/%v", visible, rawVisible)
	}

	second := prepareCognitionGuardAction(t, fixture, "fact-success-next-revision")
	thirdRevision, err := cognition.NewWorldRevision(fixture.EpisodeID, 3, cognitionTestDigest("4"))
	if err != nil {
		t.Fatal(err)
	}
	thirdObservation, err := cognition.NewActionObservation(
		"fact-success-third-observation", second.Action.ID, thirdRevision,
		"public_state", "The current state after the accepted fact.",
	)
	if err != nil {
		t.Fatal(err)
	}
	third := cognition.Transition{
		ActionID: second.Action.ID, Previous: cognitionRevisionPointer(second.ExpectedRevision),
		Current: thirdRevision, Observations: []cognition.Observation{thirdObservation}, Effects: []cognition.Effect{},
	}
	if _, err := repository.IngestCognitionTransition(
		ctx, fixture.Authority, second.Action.ID, third, facts,
	); err != nil {
		t.Fatal(err)
	}
	prepared, err = repository.PrepareCognitionRuntimeSnapshot(
		ctx, CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	projection, err = repository.GetContextProjection(
		ctx, string(prepared.Prepared.Snapshot.ContextProjection().ID),
	)
	if err != nil {
		t.Fatal(err)
	}
	visible, rawVisible, currentVisible := false, false, false
	for _, selected := range projection.Projection.Selected {
		if selected.Role == "fact" && selected.ContentSHA256 == factSHA {
			visible = len(selected.SourceRefs) == 1 &&
				selected.SourceRefs[0] == cognitionEvidenceTaskRefs([]cognition.EvidenceRef{observation.EvidenceRef()})[0]
		}
		rawVisible = rawVisible || selected.Ref == cognitionEvidenceTaskRefs([]cognition.EvidenceRef{observation.EvidenceRef()})[0]
		currentVisible = currentVisible || selected.Ref == cognitionEvidenceTaskRefs([]cognition.EvidenceRef{thirdObservation.EvidenceRef()})[0]
	}
	modelRefs := prepared.Prepared.Snapshot.EvidenceRefs()
	modelEvidence := make(map[cognition.EvidenceRef]struct{}, len(modelRefs))
	for _, ref := range modelRefs {
		modelEvidence[ref] = struct{}{}
	}
	_, sourceAdmitted := modelEvidence[observation.EvidenceRef()]
	_, currentAdmitted := modelEvidence[thirdObservation.EvidenceRef()]
	if !visible || rawVisible || !currentVisible || !sourceAdmitted || !currentAdmitted {
		t.Fatalf(
			"later clean desk fact/raw/current/source-authority/current-authority=%v/%v/%v/%v/%v",
			visible, rawVisible, currentVisible, sourceAdmitted, currentAdmitted,
		)
	}
}
