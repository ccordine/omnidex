package queue

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
)

func TestPostgresCognitionAttentionAdmissionPersistsExactAcceptedScope(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(t, repository, pool, ctx, "attention-admission")
	observation, err := cognition.NewObservation(
		"attention-admission-evidence", fixture.Start.Transition.Current,
		"public_state", "Exact evidence selected for bounded obligation retention.",
	)
	if err != nil {
		t.Fatal(err)
	}
	fixture.Start.Transition.Observations = []cognition.Observation{observation}
	if _, err := repository.StartCognitionEpisode(ctx, fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	schema := fixture.Start.ActionCatalog.Schemas[0]
	action, err := cognition.NewActionRequest(schema.Kind, []cognition.ActionArgument{})
	if err != nil {
		t.Fatal(err)
	}
	decision := cognition.CognitionDecision{
		ObligationID: fixture.Start.Root.ID, Action: action,
		EvidenceRefs:   []cognition.EvidenceRef{observation.EvidenceRef()},
		ExpectedEffect: "Expose bounded public state.",
		Attention: []cognition.AttentionRequest{{
			Operation: cognition.AttentionRetain, TargetRef: observation.EvidenceRef(),
			Scope:  cognition.AttentionScopeObligation,
			Reason: "Keep the exact evidence for this active obligation.",
		}},
	}
	prepared := prepareCognitionGuardActionWithDecision(t, fixture, decision)
	var operation, scope, disposition, targetID string
	if err := pool.QueryRow(ctx, `
		SELECT operation,scope,disposition,target_observation_id
		FROM cognition_attention_outcomes WHERE reconciliation_id=$1
	`, prepared.ReconciliationID).Scan(&operation, &scope, &disposition, &targetID); err != nil {
		t.Fatal(err)
	}
	if operation != "retain" || scope != "obligation" || disposition != "accepted" ||
		targetID != string(observation.ID) {
		t.Fatalf("attention outcome=%q/%q/%q target=%q", operation, scope, disposition, targetID)
	}
	var taskMemberships int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM working_set_memberships memberships
		JOIN working_set_items items
		  ON items.working_set_id=memberships.working_set_id AND items.item_id=memberships.item_id
		WHERE items.working_set_id=$1 AND items.ref_sha256=$2
		  AND memberships.scope_kind='task' AND memberships.scope_id=$3
		  AND memberships.retention='task'
	`, fixture.WorkingSet, observation.ContentSHA256,
		"obligation-"+string(fixture.Start.Root.ID)).Scan(&taskMemberships); err != nil {
		t.Fatal(err)
	}
	if taskMemberships != 1 {
		t.Fatalf("accepted obligation memberships=%d, want 1", taskMemberships)
	}
}

func TestPostgresCognitionAttentionAdmissionPersistsCapacityRejection(t *testing.T) {
	repository, pool, ctx := replanTestRepository(t)
	fixture := prepareTaskGenerationRetirementFixture(t, repository, pool, ctx, "attention-capacity")
	initial := make([]cognition.Observation, 0, 8)
	for index := 0; index < 8; index++ {
		observation, err := cognition.NewObservation(
			cognition.ObservationID(fmt.Sprintf("retained-%d", index)), fixture.Start.Transition.Current,
			"record", fmt.Sprintf("Exact retained evidence %d.", index),
		)
		if err != nil {
			t.Fatal(err)
		}
		initial = append(initial, observation)
	}
	fixture.Start.Transition.Observations = initial
	if _, err := repository.StartCognitionEpisode(ctx, fixture.Start, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	firstDecision := cognitionAttentionTestDecision(t, fixture, nil)
	for _, observation := range initial {
		firstDecision.EvidenceRefs = append(firstDecision.EvidenceRefs, observation.EvidenceRef())
		firstDecision.Attention = append(firstDecision.Attention, cognition.AttentionRequest{
			Operation: cognition.AttentionRetain, TargetRef: observation.EvidenceRef(),
			Scope:  cognition.AttentionScopeObligation,
			Reason: "Retain one exact bounded prerequisite for this obligation.",
		})
	}
	first := prepareCognitionGuardActionWithDecision(t, fixture, firstDecision)
	next, err := cognition.NewWorldRevision(fixture.EpisodeID, 2, strings.Repeat("7", 64))
	if err != nil {
		t.Fatal(err)
	}
	ninth, err := cognition.NewActionObservation(
		"retained-ninth", first.Action.ID, next, "record", "Ninth exact bounded prerequisite.",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.IngestCognitionTransition(ctx, fixture.Authority, first.Action.ID, cognition.Transition{
		ActionID: first.Action.ID, Previous: cognitionRevisionPointer(first.ExpectedRevision), Current: next,
		Observations: []cognition.Observation{ninth}, Effects: []cognition.Effect{}, Cost: 1,
	}, cognitionTestFactAuthority()); err != nil {
		t.Fatal(err)
	}
	secondDecision := cognitionAttentionTestDecision(t, fixture, []cognition.EvidenceRef{ninth.EvidenceRef()})
	secondDecision.Attention = []cognition.AttentionRequest{{
		Operation: cognition.AttentionRetain, TargetRef: ninth.EvidenceRef(),
		Scope:  cognition.AttentionScopeObligation,
		Reason: "Attempt to exceed the fixed scoped retention cap.",
	}}
	second := prepareCognitionGuardActionWithDecision(t, fixture, secondDecision)
	var disposition string
	if err := pool.QueryRow(ctx, `
		SELECT disposition FROM cognition_attention_outcomes WHERE reconciliation_id=$1
	`, second.ReconciliationID).Scan(&disposition); err != nil {
		t.Fatal(err)
	}
	if disposition != "rejected_capacity" {
		t.Fatalf("ninth retention disposition=%q", disposition)
	}
	var taskMemberships int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM working_set_memberships memberships
		JOIN working_set_items items
		  ON items.working_set_id=memberships.working_set_id AND items.item_id=memberships.item_id
		WHERE items.working_set_id=$1 AND items.ref_sha256=$2 AND memberships.scope_kind='task'
	`, fixture.WorkingSet, ninth.ContentSHA256).Scan(&taskMemberships); err != nil {
		t.Fatal(err)
	}
	if taskMemberships != 0 {
		t.Fatalf("capacity-rejected retention created %d task memberships", taskMemberships)
	}
}

func cognitionAttentionTestDecision(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	evidence []cognition.EvidenceRef,
) cognition.CognitionDecision {
	t.Helper()
	action, err := cognition.NewActionRequest(
		fixture.Start.ActionCatalog.Schemas[0].Kind, []cognition.ActionArgument{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if evidence == nil {
		evidence = []cognition.EvidenceRef{}
	}
	return cognition.CognitionDecision{
		ObligationID: fixture.Start.Root.ID, Action: action, EvidenceRefs: evidence,
		ExpectedEffect: "Expose bounded public state.",
		Attention:      []cognition.AttentionRequest{}, Proposals: []cognition.LedgerProposal{},
	}
}
