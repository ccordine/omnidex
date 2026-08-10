package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionruntime"
	"github.com/gryph/omnidex/internal/taskstate"
)

func TestPostgresCognitionReconciliationAtomicallyRejectsContradictedHypothesis(t *testing.T) {
	fixture := startCognitionRevisionFixture(t)
	decision := cognitionRevisionDecision(fixture)
	bound := buildCognitionDecisionStep(t, fixture.Database, decision)
	receipt, err := fixture.Database.Repository.ReconcileCognitionRuntimeDecision(
		t.Context(), bound.Command,
	)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := fixture.Database.Repository.ReconcileCognitionRuntimeDecision(
		t.Context(), bound.Command,
	)
	if err != nil || !reflect.DeepEqual(replayed, receipt) {
		t.Fatalf("revision replay=%#v want=%#v error=%v", replayed, receipt, err)
	}
	action, err := fixture.Database.Repository.PrepareCognitionAction(
		t.Context(), cognitionruntime.PrepareActionCommand{
			Binding: bound.Command.Binding, Coordinator: bound.Step, Reconciliation: receipt,
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	state, err := fixture.Database.Repository.TaskLedger(
		t.Context(), fixture.Database.Authority.JobID,
	)
	if err != nil {
		t.Fatal(err)
	}
	var revised taskstate.Entry
	for _, entry := range state.Entries {
		if fixture.Target.URI == "task:ledger/"+string(state.ID)+"/entry/"+string(entry.ID) {
			revised = entry
		}
	}
	if revised.Status != taskstate.EntryRejected || revised.DispositionBy != taskstate.AuthorityCode ||
		len(revised.Refs) == 0 || revised.Refs[len(revised.Refs)-1].Relation != taskstate.RefContradicts {
		t.Fatalf("revised hypothesis=%#v", revised)
	}
	var descriptors, actions int
	if err := fixture.Database.Repository.pool.QueryRow(t.Context(), `
		SELECT
		 (SELECT COUNT(*) FROM cognition_belief_revisions WHERE episode_id=$1),
		 (SELECT COUNT(*) FROM cognition_actions WHERE episode_id=$1)
	`, fixture.Database.EpisodeID).Scan(&descriptors, &actions); err != nil {
		t.Fatal(err)
	}
	if descriptors != 1 || actions != 2 || action.Decision.Proposals[0].Kind != "revision" {
		t.Fatalf("descriptors/actions=%d/%d action=%#v", descriptors, actions, action)
	}
}

func TestPostgresCognitionBeliefRevisionTraceRejectsForgeryAndMutation(t *testing.T) {
	fixture := startCognitionRevisionFixture(t)
	bound := buildCognitionDecisionStep(t, fixture.Database, cognitionRevisionDecision(fixture))
	if _, err := fixture.Database.Repository.ReconcileCognitionRuntimeDecision(
		t.Context(), bound.Command,
	); err != nil {
		t.Fatal(err)
	}
	var raw []byte
	var digest string
	if err := fixture.Database.Repository.pool.QueryRow(t.Context(), `
		SELECT descriptor_json,descriptor_json_sha256
		FROM cognition_belief_revisions WHERE episode_id=$1
	`, fixture.Database.EpisodeID).Scan(&raw, &digest); err != nil {
		t.Fatal(err)
	}
	if err := validateCognitionBeliefRevisionTracePayload(raw, digest); err != nil {
		t.Fatalf("valid belief revision trace: %v", err)
	}
	var forged map[string]any
	if err := json.Unmarshal(raw, &forged); err != nil {
		t.Fatal(err)
	}
	forged["target_ref"].(map[string]any)["content_sha256"] = cognitionTestDigest("8")
	changed, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	changedDigest := sha256.Sum256(changed)
	if err := validateCognitionBeliefRevisionTracePayload(
		changed, hex.EncodeToString(changedDigest[:]),
	); err == nil {
		t.Fatal("self-consistently rehashed forged belief revision entered the sealed trace")
	}
	if _, err := fixture.Database.Repository.pool.Exec(t.Context(), `
		UPDATE cognition_belief_revisions SET target_sha256=$2 WHERE episode_id=$1
	`, fixture.Database.EpisodeID, cognitionTestDigest("8")); err == nil {
		t.Fatal("durable belief revision authority was mutable")
	}
}

func TestPostgresCognitionBeliefRevisionRejectsChangedTargetAndRollsBack(t *testing.T) {
	fixture := startCognitionRevisionFixture(t)
	decision := cognitionRevisionDecision(fixture)
	decision.Proposals[0].Revision.TargetRef.SHA256 = cognitionTestDigest("9")

	bound := buildCognitionDecisionStep(t, fixture.Database, decision)
	if _, err := fixture.Database.Repository.ReconcileCognitionRuntimeDecision(
		t.Context(), bound.Command,
	); err == nil {
		t.Fatal("changed hypothesis target was reconciled")
	}
	state := mustTaskLedger(t, fixture.Database.Repository, fixture.Database.Authority.JobID)
	for _, entry := range state.Entries {
		if entry.Kind == taskstate.EntryHypothesis && entry.Status != taskstate.EntryActive {
			t.Fatalf("invalid revision changed hypothesis=%#v", entry)
		}
	}
	var descriptors int
	if err := fixture.Database.Repository.pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM cognition_belief_revisions WHERE episode_id=$1
	`, fixture.Database.EpisodeID).Scan(&descriptors); err != nil {
		t.Fatal(err)
	}
	if descriptors != 0 {
		t.Fatalf("invalid revision persisted %d descriptors", descriptors)
	}
}
