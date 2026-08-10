package queue

import (
	"os"
	"strings"
	"testing"
)

func TestAcceptedDecisionRecoveryMigrationFencesExactSourceAndReplayAttempts(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/049_cognition_accepted_decision_recovery.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"CREATE TABLE cognition_accepted_decision_recoveries",
		"recovery_attempt=source_attempt AND recovery_worker_id=source_worker_id",
		"calls.status='accepted'",
		"episodes.status='active'",
		"recovery_id='cognition_recovery_'||recovery_sha256",
		"steps.current_attempt=NEW.recovery_attempt",
		"recovery_attempt.status='active'",
		"authority_json::jsonb->>'ID' IS NOT DISTINCT FROM recovery_id",
		"NEW.authority_json::jsonb->'Projection'=calls.attempt_json::jsonb->'context_projection'",
		"NOT EXISTS (",
		"cognition_actions_policy_call_fk",
		"FOREIGN KEY (policy_call_id) REFERENCES cognition_policy_calls(call_id)",
		"cognition_accepted_decision_recoveries_immutable",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("accepted recovery migration omitted %q", required)
		}
	}
	if strings.Contains(schema,
		"UNIQUE (episode_id,job_id,generation,step_id,recovery_attempt,recovery_worker_id)") {
		t.Fatal("accepted recovery permanently reserves an actor after a resolved call")
	}
}
