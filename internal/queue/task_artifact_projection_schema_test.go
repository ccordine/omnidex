package queue

import (
	"os"
	"strings"
	"testing"
)

func TestAcceptedArtifactProjectionMigrationBindsImmutableIntentAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/034_task_artifact_projections.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE TABLE task_artifact_projections",
		"CREATE TABLE task_artifact_projection_items",
		"artifact_kind = 'intent'",
		"projection_schema = 'omnidex.accepted-intent-projection.v1'",
		"FOREIGN KEY (artifact_id, job_id, step_id)",
		"REFERENCES artifacts(id, job_id, step_id)",
		"FOREIGN KEY (job_id, job_generation)",
		"REFERENCES job_generations(job_id, generation)",
		"FOREIGN KEY (ledger_id, job_id)",
		"REFERENCES task_ledgers(id, job_id)",
		"FOREIGN KEY (ledger_id, objective_node_id)",
		"REFERENCES task_nodes(ledger_id, id)",
		"item_kind IN ('objective', 'constraint', 'ambiguity')",
		"source_uri = 'artifact://job/'",
		"prevent_task_artifact_projection_mutation",
		"task artifact projections are immutable",
		"prevent_projected_artifact_mutation",
		"accepted task artifact is immutable",
		"IF TG_OP = 'UPDATE' THEN",
		"RETURN NEW",
		"BEFORE UPDATE OR DELETE ON artifacts",
		"require_intent_artifact_projection",
		"intent artifact requires an accepted task projection",
		"DEFERRABLE INITIALLY DEFERRED",
		"BEFORE TRUNCATE ON task_artifact_projections",
		"BEFORE TRUNCATE ON task_artifact_projection_items",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("accepted artifact projection migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"ON DELETE CASCADE",
		"ON CONFLICT",
		"plan_artifact",
		"assigned_step_id BIGINT NOT NULL",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Fatalf("accepted artifact projection migration introduced forbidden %q", forbidden)
		}
	}
}
