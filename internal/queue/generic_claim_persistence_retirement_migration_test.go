package queue

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

const genericClaimPersistenceRetirementMigration = "172_generic_claim_persistence_retirement.sql"

func TestGenericClaimPersistenceSourceSurfaceIsAbsent(t *testing.T) {
	t.Parallel()
	checks := map[string][]string{
		"../model/model.go": {"type ClaimRecord struct", "type ClaimSupportRecord struct"},
		"job_history_types.go": {
			"JobHistoryClaims", "type HistoricalClaim struct", "`json:\"claims,omitempty\"`",
		},
		"repository_job_history.go":           {"JobHistoryClaims", "readHistoricalClaimPage"},
		"repository_record_history.go":        {"readHistoricalClaimPage", "FROM claims AS claim"},
		"../api/job_history.go":               {"JobHistoryClaims"},
		"../../cmd/cli/job_query_commands.go": {"evidence|claims", "claims|llm_calls"},
		"../../cmd/cli/cli_help.go":           {"evidence|claims", "claims|llm_calls"},
	}
	for path, forbidden := range checks {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Errorf("retired claim persistence token %q remains in %s", token, path)
			}
		}
	}
	for _, path := range []string{
		"repository_claim_writes.go",
		"repository_claim_writes_test.go",
		"repository_claim_writes_database_test.go",
		"repository_claims.go",
	} {
		if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
			t.Errorf("retired claim persistence file %s remains or could not be inspected: %v", path, err)
		}
	}
}

func TestGenericClaimPersistenceRetirementMigrationIsFailLoudlyAndOrdered(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + genericClaimPersistenceRetirementMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE claim_support, claims IN ACCESS EXCLUSIVE MODE",
		"EXISTS (SELECT 1 FROM claim_support)",
		"EXISTS (SELECT 1 FROM claims)",
		"generic claim persistence retirement requires a fresh reset",
		"generic claim persistence retirement postcondition failed",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("claim persistence retirement omitted %q", required)
		}
	}
	supportDrop := strings.Index(source, "DROP TABLE claim_support;")
	claimDrop := strings.Index(source, "DROP TABLE claims;")
	if supportDrop < 0 || claimDrop < 0 || supportDrop >= claimDrop {
		t.Fatalf("claim table retirement order support=%d claims=%d", supportDrop, claimDrop)
	}
	for _, forbidden := range []string{
		"DROP TABLE IF EXISTS", "CASCADE", "DELETE FROM", "TRUNCATE",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("claim persistence retirement contains forbidden fallback %q", forbidden)
		}
	}
}

func TestPostgresGenericClaimPersistenceRetirementRequiresFreshState(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "171")); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(ctx, "retained generic claim state", model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	var stepID int64
	if err := pool.QueryRow(ctx, `
		SELECT id FROM job_steps WHERE job_id=$1 ORDER BY id ASC LIMIT 1
	`, job.ID).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO claims (job_id,step_id,text,normalized_text)
		VALUES ($1,$2,'retained claim','retained claim')
	`, job.ID, stepID); err != nil {
		t.Fatal(err)
	}

	err = repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(err.Error(), "requires a fresh reset") {
		t.Fatalf("claim persistence retirement error=%v, want fresh-reset failure", err)
	}
	assertAppliedMigrationCount(t, pool, genericClaimPersistenceRetirementMigration, 0)
	assertMigrationRelationExists(t, pool, "claims", true)
	assertMigrationRelationExists(t, pool, "claim_support", true)
}

func TestPostgresFreshSchemaHasNoGenericClaimPersistence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	if err := New(pool).EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, genericClaimPersistenceRetirementMigration, 1)
	assertMigrationRelationExists(t, pool, "claim_support", false)
	assertMigrationRelationExists(t, pool, "claims", false)
}
