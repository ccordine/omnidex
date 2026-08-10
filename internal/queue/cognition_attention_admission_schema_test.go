package queue

import (
	"context"
	"strings"
	"testing"
)

func TestCognitionAttentionAdmissionMigrationOwnsExactOutcomes(t *testing.T) {
	t.Parallel()
	schema := readCognitionMigration(t, "054_cognition_attention_admission.sql")
	for _, required := range []string{
		"migration 054 requires cognition_reconciliations empty",
		"CREATE TABLE cognition_attention_outcomes",
		"rejected_capacity",
		"target_revision_sha256",
		"require_cognition_attention_outcomes",
		"expected->outcomes.request_index",
		"cognition_attention_outcomes_immutable",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("attention admission migration omitted %q", required)
		}
	}
}

func TestPostgresCognitionAttentionAdmissionRejectsLegacyReconciliation(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE cognition_reconciliations (reconciliation_id TEXT PRIMARY KEY);
		INSERT INTO cognition_reconciliations VALUES ('legacy');
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, readCognitionMigration(t, "054_cognition_attention_admission.sql")); err == nil ||
		!strings.Contains(err.Error(), "exact attention outcomes cannot be backfilled") {
		t.Fatalf("migration error=%v, want explicit legacy reconciliation rejection", err)
	}
	assertMigrationRelationExists(t, pool, "cognition_attention_outcomes", false)
}
