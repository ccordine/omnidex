package queue

import (
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPostgresCognitionStartMakesInitialObservationAvailableBeforeFirstPolicyCall(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(
		t, repository, pool, ctx, "initial-observation-continuity",
	)
	observation, err := cognition.NewObservation(
		"initial-visible-state", fixture.Start.Transition.Current,
		"public_state", "The exact initial public state and visible actions.",
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Start.Transition.Observations = []cognition.Observation{observation}
	if _, err := repository.StartCognitionEpisode(ctx, fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	assertCognitionObservationResident(t, fixture, observation)
	prepared, err := repository.PrepareCognitionRuntimeSnapshot(
		ctx, CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertCognitionSnapshotHasEvidence(t, prepared, observation.EvidenceRef())
}

func TestPostgresCognitionTransitionMakesNewObservationAvailableToNextPolicyCall(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "transition-observation-continuity")
	action := prepareCognitionGuardAction(t, fixture, "transition-observation-continuity")
	next, err := cognition.NewWorldRevision(fixture.EpisodeID, 2, cognitionTestDigest("7"))
	if err != nil {
		t.Fatal(err)
	}
	observation, err := cognition.NewActionObservation(
		"newly-acquired-evidence", action.Action.ID, next,
		"search_result", "The bounded search result needed by the next decision.",
	)
	if err != nil {
		t.Fatal(err)
	}
	transition := cognition.Transition{
		ActionID: action.Action.ID, Previous: cognitionRevisionPointer(action.ExpectedRevision),
		Current: next, Observations: []cognition.Observation{observation},
		Effects: []cognition.Effect{}, Cost: 1,
	}
	if _, err := fixture.Repository.IngestCognitionTransition(
		fixture.Context, fixture.Authority, action.Action.ID, transition, cognitionTestFactAuthority(),
	); err != nil {
		t.Fatal(err)
	}
	assertCognitionObservationResident(t, fixture, observation)
	prepared, err := fixture.Repository.PrepareCognitionRuntimeSnapshot(
		fixture.Context,
		CognitionRuntimeSnapshotCommand{Authority: fixture.Authority, EpisodeID: fixture.EpisodeID},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertCognitionSnapshotHasEvidence(t, prepared, observation.EvidenceRef())
}

func assertCognitionObservationResident(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	observation cognition.Observation,
) {
	t.Helper()
	var state, role, refHash, scopeKind, scopeID, retention string
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT items.state,items.role,items.ref_sha256,
		       memberships.scope_kind,memberships.scope_id,memberships.retention
		FROM working_set_items items
		JOIN working_set_memberships memberships
		  ON memberships.working_set_id=items.working_set_id
		 AND memberships.item_id=items.item_id
		WHERE items.working_set_id=$1 AND items.ref_sha256=$2
	`, fixture.WorkingSet, observation.ContentSHA256).Scan(
		&state, &role, &refHash, &scopeKind, &scopeID, &retention,
	); err != nil {
		t.Fatal(err)
	}
	if state != "resident" || role != "evidence" || refHash != observation.ContentSHA256 ||
		scopeKind != "call" || scopeID != "cognition-decision-"+observation.Revision.SHA256 ||
		retention != "call" {
		t.Fatalf("observation attention=%q/%q/%q scope=%q/%q retention=%q",
			state, role, refHash, scopeKind, scopeID, retention)
	}
}

func assertCognitionSnapshotHasEvidence(
	t *testing.T,
	prepared CognitionRuntimeSnapshotRecord,
	want cognition.EvidenceRef,
) {
	t.Helper()
	for _, ref := range prepared.Prepared.Snapshot.EvidenceRefs() {
		if ref == want {
			return
		}
	}
	t.Fatalf("snapshot evidence=%+v, want %+v", prepared.Prepared.Snapshot.EvidenceRefs(), want)
}
