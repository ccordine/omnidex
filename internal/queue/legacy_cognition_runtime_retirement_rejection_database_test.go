package queue

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestPostgresLegacyCognitionRetirementRejectsAuthoritativeRowsAtomically(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "064")); err != nil {
		t.Fatal(err)
	}

	if _, err := pool.Exec(ctx, `
		ALTER TABLE cognition_episode_provider_identity_evidence DISABLE TRIGGER USER;
		ALTER TABLE cognition_episode_provider_identity_evidence
			DROP CONSTRAINT cognition_episode_provider_identity_evidence_episode_id_fkey,
			DROP CONSTRAINT cognition_episode_provider_identity_evidence_evidence_id_fkey;
		INSERT INTO cognition_episode_provider_identity_evidence (episode_id,evidence_id)
		VALUES ('retained-episode','retained-evidence');
	`); err != nil {
		t.Fatal(err)
	}

	err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "065"))
	if err == nil || !strings.Contains(err.Error(),
		"legacy cognition retirement blocked: authoritative rows remain in cognition_episode_provider_identity_evidence",
	) {
		t.Fatalf("065 authoritative-data rejection=%v", err)
	}
	assertAppliedMigrationCount(t, pool, "065_legacy_cognition_runtime_retirement.sql", 0)
	assertLegacyCognitionCatalogPreservedBefore065(t, ctx, pool)
	var retained int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM cognition_episode_provider_identity_evidence
		WHERE episode_id='retained-episode' AND evidence_id='retained-evidence'
	`).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != 1 {
		t.Fatalf("rejected migration retained rows=%d want 1", retained)
	}
}

func TestPostgresLegacyCognitionRetirementRejectsShadowProjectionAtomically(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "064")); err != nil {
		t.Fatal(err)
	}
	marker := fmt.Sprintf("legacy-retirement-shadow-%d", time.Now().UnixNano())
	authority, projection := seedContextProjectionTest(t, ctx, repository, pool, marker)
	if _, err := repository.StoreContextProjection(ctx, authority, projection); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE context_projections DISABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO context_projections (
			projection_id,schema_name,job_id,generation,step_id,work_id,work_kind,usage_mode,
			spec_name,spec_version,spec_sha256,renderer_version,
			scope_ref_uri,scope_ref_version,scope_ref_sha256,scope_ref_relation,
			working_set_id,working_set_version,selected_count,omitted_count,
			rendered_context,rendered_sha256,rendered_bytes,estimated_tokens,token_estimator,
			created_at,step_attempt,worker_id
		)
		SELECT
			$2,schema_name,job_id,generation,step_id,work_id,work_kind,'shadow',
			spec_name,spec_version,spec_sha256,renderer_version,
			scope_ref_uri,scope_ref_version,scope_ref_sha256,scope_ref_relation,
			working_set_id,working_set_version,selected_count,omitted_count,
			rendered_context,rendered_sha256,rendered_bytes,estimated_tokens,token_estimator,
			created_at,step_attempt,worker_id
		FROM context_projections WHERE projection_id=$1
	`, projection.ID, "context_projection_"+strings.Repeat("e", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE context_projections ENABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}

	err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "065"))
	if err == nil || !strings.Contains(err.Error(),
		"legacy cognition retirement blocked: shadow context projections remain",
	) {
		t.Fatalf("065 shadow-data rejection=%v", err)
	}
	assertAppliedMigrationCount(t, pool, "065_legacy_cognition_runtime_retirement.sql", 0)
	assertLegacyCognitionCatalogPreservedBefore065(t, ctx, pool)
	var shadow, live int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FILTER (WHERE usage_mode='shadow'),
		       COUNT(*) FILTER (WHERE usage_mode='live')
		FROM context_projections
	`).Scan(&shadow, &live); err != nil {
		t.Fatal(err)
	}
	if shadow != 1 || live != 1 {
		t.Fatalf("rejected migration preserved shadow/live=%d/%d want 1/1", shadow, live)
	}
}

func TestPostgresLegacyCognitionRetirementRejectsActiveEpisodeFirst(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "064")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		DO $fixture$
		DECLARE column_row RECORD;
		BEGIN
			FOR column_row IN
				SELECT a.attname
				FROM pg_attribute a
				WHERE a.attrelid='cognition_episodes'::regclass
				  AND a.attnum>0 AND NOT a.attisdropped AND a.attnotnull
				  AND a.attname NOT IN ('episode_id','status')
			LOOP
				EXECUTE format(
					'ALTER TABLE cognition_episodes ALTER COLUMN %I DROP NOT NULL',
					column_row.attname
				);
			END LOOP;
		END
		$fixture$
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE cognition_episodes DISABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO cognition_episodes (episode_id,status)
		VALUES ('retained-active-episode','active')
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE cognition_episodes ENABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}

	err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "065"))
	if err == nil || !strings.Contains(err.Error(),
		"legacy cognition retirement blocked: active cognition episodes remain",
	) {
		t.Fatalf("065 active-episode rejection=%v", err)
	}
	assertAppliedMigrationCount(t, pool, "065_legacy_cognition_runtime_retirement.sql", 0)
	assertLegacyCognitionCatalogPreservedBefore065(t, ctx, pool)
	var retained int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM cognition_episodes
		WHERE episode_id='retained-active-episode' AND status='active'
	`).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != 1 {
		t.Fatalf("rejected migration retained active rows=%d want 1", retained)
	}
}

func TestPostgresLegacyCognitionRetirementRejectsChangedTraceSeed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "064")); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE cognition_trace_schema_authority DISABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM cognition_trace_schema_authority`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE cognition_trace_schema_authority ENABLE TRIGGER USER`); err != nil {
		t.Fatal(err)
	}

	err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "065"))
	if err == nil || !strings.Contains(err.Error(),
		"legacy cognition retirement blocked: trace schema authority differs from migration 061",
	) {
		t.Fatalf("065 trace-seed rejection=%v", err)
	}
	assertAppliedMigrationCount(t, pool, "065_legacy_cognition_runtime_retirement.sql", 0)
	assertLegacyCognitionCatalogPreservedBefore065(t, ctx, pool)
}
