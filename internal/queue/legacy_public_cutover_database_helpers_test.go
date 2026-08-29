package queue

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/db"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type legacyPublicFixture struct {
	DatabaseURL string
	Bundle      MigrationBundle
	Pool        *pgxpool.Pool
}

func openLegacyPublicFixture(t *testing.T) legacyPublicFixture {
	t.Helper()
	baseURL, configured := os.LookupEnv("OMNI_LEGACY_TEST_DATABASE_URL")
	if !configured || baseURL == "" {
		t.Skip("set OMNI_LEGACY_TEST_DATABASE_URL to run legacy public cutover tests against PostgreSQL with vector 0.8.2")
	}
	if baseURL != strings.TrimSpace(baseURL) {
		t.Fatal("OMNI_LEGACY_TEST_DATABASE_URL must not contain surrounding whitespace")
	}
	databaseURL := freshRuntimeDatabaseURL(t, baseURL)
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	bundle := loadCheckedMigrationBundle(t)
	installLegacyPublicFixture(t, pool, bundle)
	return legacyPublicFixture{DatabaseURL: databaseURL, Bundle: bundle, Pool: pool}
}

func installLegacyPublicFixture(t *testing.T, pool *pgxpool.Pool, bundle MigrationBundle) {
	t.Helper()
	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(t.Context(), `SET LOCAL search_path TO public,pg_catalog`); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(t.Context(), `
		CREATE TABLE schema_migrations (
		 filename TEXT PRIMARY KEY,
		 applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`); err != nil {
		t.Fatal(err)
	}
	legacy, err := legacyMigrationEntries(bundle)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range legacy {
		if _, err := tx.Exec(t.Context(), string(entry.body)); err != nil {
			t.Fatalf("install legacy fixture migration %s: %v", entry.name, err)
		}
		if _, err := tx.Exec(t.Context(),
			`INSERT INTO schema_migrations(filename) VALUES ($1)`, entry.name,
		); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func openLegacyCutoverPool(t *testing.T, databaseURL string) *pgxpool.Pool {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	t.Cleanup(cancel)
	pool, err := db.ConnectLegacyPublicCutover(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func seedLegacyPublicRows(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO public.projects(id,location,name) VALUES
		 (41,'/tmp/preserved-project','Preserved project');
		SELECT setval('public.projects_id_seq',91,true);
		INSERT INTO public.project_planning_configs(project_id,model,reasoning_mode)
		 VALUES (41,'','instant');
		INSERT INTO public.jobs(
			id,instruction,pipeline,project_id,status,result,completed_at
		) VALUES (
			51,'preserve exact row','coding',41,'completed',
			'preserved exact result',clock_timestamp()
		);
		INSERT INTO public.jobs(id,instruction,pipeline,project_id,status) VALUES
		 (52,'preserve assistant history','assistant',41,'completed'),
		 (53,'preserve story history','story',41,'failed'),
		 (54,'preserve agent history','agent',41,'canceled');
		SELECT setval('public.jobs_id_seq',101,true);
		INSERT INTO public.job_steps(
			id,job_id,action,sort_index,status,output,started_at,finished_at
		) VALUES (
			61,51,'implementation',0,'completed','preserved exact output',
			clock_timestamp(),clock_timestamp()
		);
		SELECT setval('public.job_steps_id_seq',111,true);
		INSERT INTO public.llm_call_evidence(
		 job_id,step_id,scope,requested_model,model,attempt,
		 system_prompt,user_prompt,request_sha256,response_format,
		 context_tokens,max_output_tokens,response,response_sha256,status,latency_ms
		) VALUES (
		 51,61,'preserved-trigger','fixture','fixture',1,
		 'system','user',repeat('a',64),'text',8192,128,
		 'response',repeat('b',64),'succeeded',1
		);
	`); err != nil {
		t.Fatalf("seed legacy public rows: %v", err)
	}
}

func assertLegacyRollbackState(t *testing.T, databaseURL string) {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var publicLedger, runtimeExists bool
	if err := pool.QueryRow(t.Context(), `
		SELECT EXISTS(
		 SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
		 WHERE n.nspname='public' AND c.relname='schema_migrations'
		),EXISTS(SELECT 1 FROM pg_namespace WHERE nspname=$1)
	`, db.DefaultRuntimeSchema).Scan(&publicLedger, &runtimeExists); err != nil {
		t.Fatal(err)
	}
	if !publicLedger || runtimeExists {
		t.Fatalf("failed cutover changed authoritative schemas public=%t runtime=%t",
			publicLedger, runtimeExists)
	}
}

func rejectedFrozenPrefixMigrationBundle(t *testing.T, bundle MigrationBundle) MigrationBundle {
	t.Helper()
	copyBundle := MigrationBundle{entries: append([]migrationBundleEntry{}, bundle.entries...)}
	index := -1
	for candidate, entry := range copyBundle.entries {
		prefix, err := migrationNumericPrefix(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if prefix == legacyCutoverFinalMigrationPrefix {
			index = candidate
			break
		}
	}
	if index < 0 {
		t.Fatalf("bundle lacks frozen migration prefix %03d", legacyCutoverFinalMigrationPrefix)
	}
	copyBundle.entries[index].body = []byte("SELECT missing_cutover_test_function();\n")
	copyBundle.entries[index].sha256 = digestMigrationBytes(copyBundle.entries[index].body)
	var manifest strings.Builder
	for _, entry := range copyBundle.entries {
		fmt.Fprintf(&manifest, "%s  %s\n", entry.sha256, entry.name)
	}
	copyBundle.manifest = []byte(manifest.String())
	copyBundle.manifestSHA256 = digestMigrationBytes(copyBundle.manifest)
	if err := copyBundle.validate(); err != nil {
		t.Fatal(err)
	}
	return copyBundle
}
