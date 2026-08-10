package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionruntime"
)

func TestPostgresAcceptedRecoveryRejectsForgedAuthorityProjection(t *testing.T) {
	fixture := startTaskGenerationRetirementFixture(t, "forged-accepted-recovery")
	prepared, step := reserveAcceptedDecisionWithoutAction(t, fixture)
	decisionSHA, err := cognitionruntime.DecisionSHA256(*step.Decision)
	if err != nil {
		t.Fatal(err)
	}
	var callID string
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT call_id FROM cognition_policy_calls WHERE episode_id=$1
	`, fixture.EpisodeID).Scan(&callID); err != nil {
		t.Fatal(err)
	}
	authorityJSON, authoritySHA, err := cognitionJSON(struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	digest := strings.Repeat("a", 64)
	revision := prepared.Prepared.Snapshot.CurrentRevision()
	projection := prepared.Prepared.Snapshot.ContextProjection()
	source := prepared.Prepared.Snapshot.Attempt()
	_, err = fixture.Pool.Exec(fixture.Context, `
		INSERT INTO cognition_accepted_decision_recoveries (
			recovery_id,recovery_sha256,episode_id,job_id,generation,step_id,
			source_policy_call_id,source_attempt,source_worker_id,recovery_attempt,recovery_worker_id,
			snapshot_sha256,expected_revision,expected_revision_sha256,graph_version,graph_sha256,
			projection_id,obligation_node_id,decision_sha256,action_schema_id,
			action_schema_version,action_schema_sha256,authority_json,authority_json_sha256
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
	`, "cognition_recovery_"+digest, digest, fixture.EpisodeID, source.JobID,
		source.Generation, source.StepID, callID, int64(source.Attempt), source.WorkerID,
		prepared.Prepared.Snapshot.SHA256(), int64(revision.Number), revision.SHA256,
		int64(prepared.Prepared.GraphVersion), prepared.Prepared.ObligationGraph.SHA256,
		projection.ID, step.Decision.ObligationID, decisionSHA, step.ActionSchema.ID,
		step.ActionSchema.Version, step.ActionSchema.SHA256, string(authorityJSON), authoritySHA)
	if err == nil {
		t.Fatal("direct SQL accepted a recovery with missing exact authority fields")
	}
	var count int
	if err := fixture.Pool.QueryRow(fixture.Context, `
		SELECT COUNT(*) FROM cognition_accepted_decision_recoveries WHERE source_policy_call_id=$1
	`, callID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("forged accepted recovery occupied durable authority: count=%d", count)
	}
}
