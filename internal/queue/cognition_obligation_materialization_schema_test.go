package queue

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestPostgresCognitionMaterializationMigrationRejectsLegacyEpisodes(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `
		CREATE TABLE cognition_episodes (episode_id TEXT PRIMARY KEY);
		INSERT INTO cognition_episodes VALUES ('legacy-episode');
	`); err != nil {
		t.Fatal(err)
	}
	migration := readCognitionMigration(t, "050_cognition_obligation_materialization.sql")
	if _, err := pool.Exec(ctx, migration); err == nil ||
		!strings.Contains(err.Error(), "completion and prepared-evidence authority cannot be backfilled") {
		t.Fatalf("migration error=%v, want explicit legacy-episode rejection", err)
	}
	var count int
	if err := pool.QueryRow(ctx, `SELECT COUNT(*) FROM cognition_episodes`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("legacy episode count=%d, want rollback-preserved row", count)
	}
}

func TestCognitionMaterializationMigrationBindsOneAtomicActionGraph(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/050_cognition_obligation_materialization.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"completion_authority_json", "completion_evidence_refs_json",
		"completion_evidence_refs_sha256", "CREATE TABLE cognition_obligation_materializations",
		"CREATE TABLE cognition_obligation_materialization_applications",
		"successful cognition action omitted its obligation materialization",
		"actions.status='succeeded'", "graphs.command_kind='materialize'",
		"cognition_obligation_materializations_immutable",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("obligation materialization migration omitted %q", required)
		}
	}
}

func TestCognitionMaterializationDoesNotUseLegacyThreeCommandPath(t *testing.T) {
	t.Parallel()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"AddCognitionObligationCommand", "AddCognitionObligationDependencyCommand",
			"ActivateCognitionObligationCommand", "ApplyCognitionObligationCommand",
			"AddCognitionObligationEvidenceCommand", "CognitionObligationAddDependency",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s contains forbidden legacy materialization path %q", name, forbidden)
			}
		}
	}
}
