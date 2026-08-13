package queue

import (
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/db"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresLegacyPublicCutoverPreservesRowsSequencesOIDsAndRetriesExactly(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	seedLegacyPublicRows(t, fixture.Pool)
	var publicSchemaOID, projectsOID uint32
	if err := fixture.Pool.QueryRow(t.Context(), `
		SELECT (SELECT oid FROM pg_namespace WHERE nspname='public'),
		       'public.projects'::regclass::oid
	`).Scan(&publicSchemaOID, &projectsOID); err != nil {
		t.Fatal(err)
	}
	fixture.Pool.Close()

	cutoverPool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	receipt, err := PreserveLegacyPublic(
		t.Context(), cutoverPool, db.DefaultRuntimeSchema, fixture.Bundle,
	)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.RuntimeSchemaOID != publicSchemaOID ||
		receipt.MigrationManifestSHA256 != legacyExpectedMigrationManifestSHA256 ||
		receipt.RuntimeCatalogSHA256 != legacyExpectedRuntimeCatalogSHA256 {
		t.Fatalf("cutover receipt=%+v", receipt)
	}
	assertPreservedLegacyData(t, cutoverPool, projectsOID, fixture.Bundle)

	retried, err := PreserveLegacyPublic(
		t.Context(), cutoverPool, db.DefaultRuntimeSchema, fixture.Bundle,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(retried, receipt) {
		t.Fatalf("retry receipt=%+v want %+v", retried, receipt)
	}
	firstJSON, err := receipt.JSON()
	if err != nil {
		t.Fatal(err)
	}
	retryJSON, err := retried.JSON()
	if err != nil {
		t.Fatal(err)
	}
	if retryJSON != firstJSON {
		t.Fatalf("retry JSON=%q want exact %q", retryJSON, firstJSON)
	}
	cutoverPool.Close()

	runtime, err := db.ConnectRuntime(
		t.Context(), fixture.DatabaseURL, db.DefaultRuntimeSchema,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer runtime.Close()
	if err := New(runtime).EnsureSchema(t.Context(), fixture.Bundle); err != nil {
		t.Fatalf("normal runtime rejected completed cutover: %v", err)
	}
}

func assertPreservedLegacyData(
	t *testing.T,
	pool *pgxpool.Pool,
	projectsOID uint32,
	bundle MigrationBundle,
) {
	t.Helper()
	var projectCount, jobCount, stepCount int
	var projectSequence, jobSequence, stepSequence int64
	var currentProjectsOID uint32
	var retiredPlanningTables bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
		 (SELECT COUNT(*) FROM omnidex_runtime.projects WHERE id=41 AND
		   location='/tmp/preserved-project'),
		 (SELECT COUNT(*) FROM omnidex_runtime.jobs WHERE id=51 AND project_id=41),
		 (SELECT COUNT(*) FROM omnidex_runtime.job_steps WHERE id=61 AND job_id=51),
		 (SELECT last_value FROM omnidex_runtime.projects_id_seq),
		 (SELECT last_value FROM omnidex_runtime.jobs_id_seq),
		 (SELECT last_value FROM omnidex_runtime.job_steps_id_seq),
		 'omnidex_runtime.projects'::regclass::oid,
		 to_regclass('omnidex_runtime.project_planning_configs') IS NULL AND
		 to_regclass('omnidex_runtime.project_planning_messages') IS NULL AND
		 to_regclass('omnidex_runtime.project_planning_drafts') IS NULL
	`).Scan(
		&projectCount, &jobCount, &stepCount, &projectSequence, &jobSequence,
		&stepSequence, &currentProjectsOID, &retiredPlanningTables,
	); err != nil {
		t.Fatal(err)
	}
	if projectCount != 1 || jobCount != 1 || stepCount != 1 ||
		projectSequence != 91 || jobSequence != 101 || stepSequence != 111 ||
		currentProjectsOID != projectsOID || !retiredPlanningTables {
		t.Fatalf(
			"preserved state rows=%d/%d/%d sequences=%d/%d/%d relation_oid=%d/%d",
			projectCount, jobCount, stepCount, projectSequence, jobSequence,
			stepSequence, currentProjectsOID, projectsOID,
		)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE omnidex_runtime.llm_call_evidence SET scope='mutated'
	`); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("preserved immutable trigger error=%v", err)
	}
	var migrationCount int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM omnidex_runtime.schema_migrations
		WHERE manifest_sha256=$1
	`, bundle.ManifestSHA256()).Scan(&migrationCount); err != nil {
		t.Fatal(err)
	}
	if migrationCount != len(bundle.entries) {
		t.Fatalf("migration count=%d want %d", migrationCount, len(bundle.entries))
	}
}
