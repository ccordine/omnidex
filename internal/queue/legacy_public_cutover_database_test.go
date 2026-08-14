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
	var publicSchemaOID, projectsOID, jobsProjectFKOID, jobStepsJobFKOID uint32
	if err := fixture.Pool.QueryRow(t.Context(), `
		SELECT (SELECT oid FROM pg_namespace WHERE nspname='public'),
		       'public.projects'::regclass::oid,
		       (SELECT oid FROM pg_constraint
		        WHERE conrelid='public.jobs'::regclass AND
		              conname='jobs_project_id_fkey'),
		       (SELECT oid FROM pg_constraint
		        WHERE conrelid='public.job_steps'::regclass AND
		              conname='job_steps_job_id_fkey')
	`).Scan(
		&publicSchemaOID, &projectsOID, &jobsProjectFKOID, &jobStepsJobFKOID,
	); err != nil {
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
	assertPreservedLegacyData(
		t, cutoverPool, projectsOID, jobsProjectFKOID, jobStepsJobFKOID, fixture.Bundle,
	)

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
	jobsProjectFKOID uint32,
	jobStepsJobFKOID uint32,
	bundle MigrationBundle,
) {
	t.Helper()
	var projectCount, jobCount, stepCount, retiredHistoryCount int
	var projectSequence, jobSequence, stepSequence int64
	var currentProjectsOID, currentJobsProjectFKOID, currentJobStepsJobFKOID uint32
	var retiredPlanningTables bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
		 (SELECT COUNT(*) FROM omnidex_runtime.projects WHERE id=41 AND
		   location='/tmp/preserved-project'),
		 (SELECT COUNT(*) FROM omnidex_runtime.jobs WHERE id=51 AND project_id=41),
		 (SELECT COUNT(*) FROM omnidex_runtime.job_steps WHERE id=61 AND job_id=51),
		 (SELECT COUNT(*) FROM omnidex_runtime.jobs WHERE
		  (id,pipeline,status) IN (
		    (52,'assistant','completed'),
		    (53,'story','failed'),
		    (54,'agent','canceled')
		  )),
		 (SELECT last_value FROM omnidex_runtime.projects_id_seq),
		 (SELECT last_value FROM omnidex_runtime.jobs_id_seq),
		 (SELECT last_value FROM omnidex_runtime.job_steps_id_seq),
		 'omnidex_runtime.projects'::regclass::oid,
		 (SELECT oid FROM pg_constraint
		  WHERE conrelid='omnidex_runtime.jobs'::regclass AND
		        conname='jobs_project_id_fkey'),
		 (SELECT oid FROM pg_constraint
		  WHERE conrelid='omnidex_runtime.job_steps'::regclass AND
		        conname='job_steps_job_id_fkey'),
		 to_regclass('omnidex_runtime.project_planning_configs') IS NULL AND
		 to_regclass('omnidex_runtime.project_planning_messages') IS NULL AND
		 to_regclass('omnidex_runtime.project_planning_drafts') IS NULL
	`).Scan(
		&projectCount, &jobCount, &stepCount, &retiredHistoryCount,
		&projectSequence, &jobSequence, &stepSequence, &currentProjectsOID,
		&currentJobsProjectFKOID,
		&currentJobStepsJobFKOID, &retiredPlanningTables,
	); err != nil {
		t.Fatal(err)
	}
	if projectCount != 1 || jobCount != 1 || stepCount != 1 || retiredHistoryCount != 3 ||
		projectSequence != 91 || jobSequence != 101 || stepSequence != 111 ||
		currentProjectsOID != projectsOID || currentJobsProjectFKOID != jobsProjectFKOID ||
		currentJobStepsJobFKOID != jobStepsJobFKOID || !retiredPlanningTables {
		t.Fatalf(
			"preserved state rows=%d/%d/%d retired_history=%d sequences=%d/%d/%d relation_oid=%d/%d fk_oids=%d/%d,%d/%d",
			projectCount, jobCount, stepCount, retiredHistoryCount,
			projectSequence, jobSequence,
			stepSequence, currentProjectsOID, projectsOID, currentJobsProjectFKOID,
			jobsProjectFKOID, currentJobStepsJobFKOID, jobStepsJobFKOID,
		)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO omnidex_runtime.job_steps(job_id,generation,action,sort_index)
		VALUES (9223372036854775807,1,'must-not-exist',0)
	`); err == nil || !strings.Contains(err.Error(), "foreign key") {
		t.Fatalf("preserved foreign-key enforcement error=%v", err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE omnidex_runtime.jobs SET result='must-not-change' WHERE id=52
	`); err == nil || !strings.Contains(err.Error(), "historical retired job is immutable") {
		t.Fatalf("preserved retired history mutation error=%v", err)
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
