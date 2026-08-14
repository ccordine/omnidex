package queue

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/db"
)

func TestPostgresLegacyCutoverRejectsCatalogDriftWithoutMutation(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	if _, err := fixture.Pool.Exec(t.Context(), `
		CREATE TABLE public.unexpected_legacy_table (id BIGINT PRIMARY KEY)
	`); err != nil {
		t.Fatal(err)
	}
	fixture.Pool.Close()
	pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	_, err := PreserveLegacyPublic(
		t.Context(), pool, db.DefaultRuntimeSchema, fixture.Bundle,
	)
	if err == nil || !strings.Contains(err.Error(), "catalog differs") {
		t.Fatalf("PreserveLegacyPublic error=%v, want catalog drift rejection", err)
	}
	pool.Close()
	assertLegacyRollbackState(t, fixture.DatabaseURL)
}

func TestPostgresLegacyCutoverRejectsLedgerDriftWithoutMutation(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	if _, err := fixture.Pool.Exec(t.Context(), `
		DELETE FROM public.schema_migrations WHERE filename='024_llm_evidence.sql'
	`); err != nil {
		t.Fatal(err)
	}
	fixture.Pool.Close()
	pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	_, err := PreserveLegacyPublic(
		t.Context(), pool, db.DefaultRuntimeSchema, fixture.Bundle,
	)
	if err == nil || !strings.Contains(err.Error(), "ledger") {
		t.Fatalf("PreserveLegacyPublic error=%v, want ledger rejection", err)
	}
	pool.Close()
	assertLegacyRollbackState(t, fixture.DatabaseURL)
}

func TestPostgresLegacyCutoverRejectsChangedReleaseBundleWithoutMutation(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	fixture.Pool.Close()
	pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	_, err := PreserveLegacyPublic(
		t.Context(), pool, db.DefaultRuntimeSchema,
		rejectedFinalMigrationBundle(t, fixture.Bundle),
	)
	if err == nil || !strings.Contains(err.Error(), "release migration manifest differs") {
		t.Fatalf("PreserveLegacyPublic error=%v, want changed release bundle rejection", err)
	}
	pool.Close()
	assertLegacyRollbackState(t, fixture.DatabaseURL)
}

func TestPostgresLegacyCutoverRejectsNonterminalRetiredPipelineWithoutMutation(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	if _, err := fixture.Pool.Exec(t.Context(), `
		INSERT INTO public.jobs(instruction,pipeline,status)
		VALUES ('must remain legacy','agent','pending')
	`); err != nil {
		t.Fatal(err)
	}
	fixture.Pool.Close()
	pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	_, err := PreserveLegacyPublic(
		t.Context(), pool, db.DefaultRuntimeSchema, fixture.Bundle,
	)
	if err == nil || !strings.Contains(err.Error(), "nonterminal retired or unregistered pipeline") {
		t.Fatalf("PreserveLegacyPublic error=%v, want retired-pipeline rejection", err)
	}
	pool.Close()
	assertLegacyRollbackState(t, fixture.DatabaseURL)
}

func TestPostgresLegacyCutoverRejectsRuntimeCollisionWithoutMutation(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	if _, err := fixture.Pool.Exec(t.Context(), `CREATE SCHEMA omnidex_runtime`); err != nil {
		t.Fatal(err)
	}
	fixture.Pool.Close()
	pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	_, err := PreserveLegacyPublic(
		t.Context(), pool, db.DefaultRuntimeSchema, fixture.Bundle,
	)
	if err == nil || !strings.Contains(err.Error(), "neither the exact legacy") {
		t.Fatalf("PreserveLegacyPublic error=%v, want runtime collision rejection", err)
	}
	pool.Close()
	var retained bool
	check := openLegacyCutoverPool(t, fixture.DatabaseURL)
	if err := check.QueryRow(t.Context(), `
		SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='omnidex_runtime')
	`).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if !retained {
		t.Fatal("runtime collision was mutated by rejected cutover")
	}
}

func TestPostgresLegacyCutoverRejectsAnotherDatabaseSession(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	guard, err := fixture.Pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Release()
	pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	_, err = PreserveLegacyPublic(
		t.Context(), pool, db.DefaultRuntimeSchema, fixture.Bundle,
	)
	if err == nil || !strings.Contains(err.Error(), "zero other database sessions") {
		t.Fatalf("PreserveLegacyPublic error=%v, want active-session rejection", err)
	}
}

func TestPostgresLegacyCutoverRejectsObjectACLWithoutMutation(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	if _, err := fixture.Pool.Exec(t.Context(), `
		GRANT SELECT ON public.projects TO PUBLIC
	`); err != nil {
		t.Fatal(err)
	}
	fixture.Pool.Close()
	pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	_, err := PreserveLegacyPublic(
		t.Context(), pool, db.DefaultRuntimeSchema, fixture.Bundle,
	)
	if err == nil || !strings.Contains(err.Error(), "ownership or ACL") {
		t.Fatalf("PreserveLegacyPublic error=%v, want ACL rejection", err)
	}
	pool.Close()
	assertLegacyRollbackState(t, fixture.DatabaseURL)
}

func TestPostgresLegacyCutoverRejectsPublicSchemaACLWithoutMutation(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	if _, err := fixture.Pool.Exec(t.Context(), `REVOKE USAGE ON SCHEMA public FROM PUBLIC`); err != nil {
		t.Fatal(err)
	}
	fixture.Pool.Close()
	pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	_, err := PreserveLegacyPublic(t.Context(), pool, db.DefaultRuntimeSchema, fixture.Bundle)
	if err == nil || !strings.Contains(err.Error(), "schema ACL differs") {
		t.Fatalf("PreserveLegacyPublic error=%v, want public schema ACL rejection", err)
	}
	pool.Close()
	assertLegacyRollbackState(t, fixture.DatabaseURL)
}

func TestPostgresLegacyCutoverRejectsPublicSchemaOwnerWithoutMutation(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	if _, err := fixture.Pool.Exec(t.Context(), `ALTER SCHEMA public OWNER TO CURRENT_USER`); err != nil {
		t.Fatal(err)
	}
	fixture.Pool.Close()
	pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	_, err := PreserveLegacyPublic(t.Context(), pool, db.DefaultRuntimeSchema, fixture.Bundle)
	if err == nil || !strings.Contains(err.Error(), "owner differs") {
		t.Fatalf("PreserveLegacyPublic error=%v, want public schema owner rejection", err)
	}
	pool.Close()
	assertLegacyRollbackState(t, fixture.DatabaseURL)
}

func TestPostgresLegacyCutoverRejectsUnsupportedObjectWithoutMutation(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	if _, err := fixture.Pool.Exec(t.Context(), `CREATE TYPE public.unexpected_state AS ENUM ('unexpected')`); err != nil {
		t.Fatal(err)
	}
	fixture.Pool.Close()
	pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	_, err := PreserveLegacyPublic(t.Context(), pool, db.DefaultRuntimeSchema, fixture.Bundle)
	if err == nil || !strings.Contains(err.Error(), "unsupported objects") {
		t.Fatalf("PreserveLegacyPublic error=%v, want unsupported object rejection", err)
	}
	pool.Close()
	assertLegacyRollbackState(t, fixture.DatabaseURL)
}

func TestPostgresLegacyCutoverRejectsExtensionDriftWithoutMutation(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	if _, err := fixture.Pool.Exec(t.Context(), `
		CREATE SCHEMA extension_drift;
		ALTER EXTENSION pgcrypto SET SCHEMA extension_drift;
	`); err != nil {
		t.Fatal(err)
	}
	fixture.Pool.Close()
	pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	_, err := PreserveLegacyPublic(
		t.Context(), pool, db.DefaultRuntimeSchema, fixture.Bundle,
	)
	if err == nil || !strings.Contains(err.Error(), "extension inventory") {
		t.Fatalf("PreserveLegacyPublic error=%v, want extension drift rejection", err)
	}
	pool.Close()
	assertLegacyRollbackState(t, fixture.DatabaseURL)
}

func TestPostgresLegacyCutoverRetryRejectsChangedDurableReceipt(t *testing.T) {
	fixture := openLegacyPublicFixture(t)
	fixture.Pool.Close()
	pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
	if _, err := PreserveLegacyPublic(
		t.Context(), pool, db.DefaultRuntimeSchema, fixture.Bundle,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		COMMENT ON SCHEMA omnidex_runtime IS '{"schema":"changed"}'
	`); err != nil {
		t.Fatal(err)
	}
	_, err := PreserveLegacyPublic(
		t.Context(), pool, db.DefaultRuntimeSchema, fixture.Bundle,
	)
	if err == nil || !strings.Contains(err.Error(), "receipt") {
		t.Fatalf("PreserveLegacyPublic retry error=%v, want receipt rejection", err)
	}
}

func TestPostgresLegacyCutoverRollsBackWhenRetiredPlanningStateIsNotEmpty(t *testing.T) {
	cases := map[string]string{
		"message": `INSERT INTO public.project_planning_messages(project_id,role,content)
			SELECT id,'user','retain me' FROM public.projects WHERE location='/tmp/planning-retained'`,
		"draft": `INSERT INTO public.project_planning_drafts(project_id,id,title)
			SELECT id,'draft-1','Retain me' FROM public.projects WHERE location='/tmp/planning-retained'`,
		"non-default config": `INSERT INTO public.project_planning_configs(project_id,model,reasoning_mode)
			SELECT id,'fixture','instant' FROM public.projects WHERE location='/tmp/planning-retained'`,
	}
	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := openLegacyPublicFixture(t)
			if _, err := fixture.Pool.Exec(t.Context(), `
				INSERT INTO public.projects(location,name)
				VALUES ('/tmp/planning-retained','retained')
			`); err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.Pool.Exec(t.Context(), setup); err != nil {
				t.Fatal(err)
			}
			fixture.Pool.Close()
			pool := openLegacyCutoverPool(t, fixture.DatabaseURL)
			_, err := PreserveLegacyPublic(
				t.Context(), pool, db.DefaultRuntimeSchema, fixture.Bundle,
			)
			if err == nil || !strings.Contains(err.Error(), "project planning retirement refuses") {
				t.Fatalf("PreserveLegacyPublic error=%v, want retained planning rejection", err)
			}
			pool.Close()
			assertLegacyRollbackState(t, fixture.DatabaseURL)
		})
	}
}
