package queue

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const objectiveCitationRequirementBindingMigration = "175_objective_citation_requirement_authority_bindings.sql"

func TestObjectiveCitationRequirementBindingCurrentSourceHasOneName(t *testing.T) {
	t.Parallel()
	legacyField := "Supports" + "Claims"
	legacyJSONKey := "supports_" + "claims"
	err := filepath.WalkDir("..", func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, forbidden := range []string{legacyField, legacyJSONKey} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("legacy objective citation binding name %q remains in %s", forbidden, path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestObjectiveCitationRequirementBindingMigrationIsFailLoudAndAtomic(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + objectiveCitationRequirementBindingMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	legacyJSONKey := "supports_" + "claims"
	for _, required := range []string{
		"LOCK TABLE evidence, step_completion_evidence_sets IN ACCESS EXCLUSIVE MODE",
		"objective citation requirement-binding migration rejected dirty evidence row",
		"objective citation requirement-binding migration rejected dirty completion set",
		"objective citation requirement-binding migration postcondition failed",
		"objective_completion_evidence_set_is_valid",
		"requirement_authority_bindings",
		legacyJSONKey,
		"DROP TRIGGER objective_completion_evidence_update_immutable ON evidence",
		"CREATE TRIGGER objective_completion_evidence_update_immutable",
		"DROP FUNCTION objective_citation_requirement_bindings_are_valid(JSONB,TEXT,TEXT)",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("requirement-binding migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"DROP TRIGGER IF EXISTS", "DROP FUNCTION IF EXISTS", "COALESCE(payload_json->",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("requirement-binding migration contains fallback authority %q", forbidden)
		}
	}
	legacyRemoval := strings.Index(source, "payload_json-'"+legacyJSONKey+"'")
	currentInsertion := strings.Index(source, "'requirement_authority_bindings'")
	if legacyRemoval < 0 || currentInsertion < 0 || currentInsertion >= legacyRemoval {
		t.Fatalf(
			"requirement-binding rewrite order current=%d legacy-removal=%d",
			currentInsertion, legacyRemoval,
		)
	}
}

func TestPostgresObjectiveCitationRequirementBindingMigrationRejectsDirtyStateThenRewrites(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "174")); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(ctx, "legacy objective citation binding", model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "legacy-objective-citation-binding-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("objective completion claim=%+v want job %d", claim, job.ID)
	}
	command := objectiveEvidenceCompletionCommand(t, claim, []evidence.Record{
		objectiveCompletionEvidence(claim, "legacy-requirement-binding"),
	})
	if err := repository.CompleteStepWithEvidence(ctx, command); err != nil {
		t.Fatal(err)
	}
	convertObjectiveCitationFixtureToLegacyBindings(t, ctx, pool)

	mutateObjectiveCitationFixture(t, ctx, pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `
			UPDATE evidence
			SET payload_json=payload_json || jsonb_build_object(
				'requirement_authority_bindings',payload_json->'supports_claims'
			)
			WHERE kind='objective_citation'
		`)
		return err
	})
	err = repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(err.Error(), "rejected dirty evidence row") {
		t.Fatalf("mixed requirement-binding migration error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, objectiveCitationRequirementBindingMigration, 0)

	mutateObjectiveCitationFixture(t, ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE evidence
			SET payload_json=jsonb_set(
				payload_json-'requirement_authority_bindings',
				'{supports_claims}','[]'::jsonb
			)
			WHERE kind='objective_citation'
		`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE step_completion_evidence_sets AS authority
			SET records_json=(
				SELECT jsonb_agg(
					jsonb_set(item.payload,'{supports_claims}','[]'::jsonb)
					ORDER BY item.ordinality
				)
				FROM jsonb_array_elements(authority.records_json)
					 WITH ORDINALITY AS item(payload,ordinality)
			)
		`)
		return err
	})
	err = repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(err.Error(), "rejected dirty evidence row") {
		t.Fatalf("malformed requirement-binding migration error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, objectiveCitationRequirementBindingMigration, 0)

	repairObjectiveCitationFixtureLegacyBindings(t, ctx, pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, objectiveCitationRequirementBindingMigration, 1)
	assertCurrentObjectiveCitationRequirementBindings(t, ctx, pool)
	if _, err := pool.Exec(ctx, `
		UPDATE evidence SET source_ref='forged'
		WHERE kind='objective_citation'
	`); err == nil || !strings.Contains(err.Error(), "immutable") {
		t.Fatalf("migrated objective citation immutability error=%v", err)
	}
}

func convertObjectiveCitationFixtureToLegacyBindings(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()
	mutateObjectiveCitationFixture(t, ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE evidence
			SET payload_json=(payload_json-'requirement_authority_bindings') ||
				jsonb_build_object(
					'supports_claims',payload_json->'requirement_authority_bindings'
				)
			WHERE kind='objective_citation'
		`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE step_completion_evidence_sets AS authority
			SET records_json=(
				SELECT jsonb_agg(
					(item.payload-'requirement_authority_bindings') ||
					jsonb_build_object(
						'supports_claims',item.payload->'requirement_authority_bindings'
					)
					ORDER BY item.ordinality
				)
				FROM jsonb_array_elements(authority.records_json)
					 WITH ORDINALITY AS item(payload,ordinality)
			)
		`)
		return err
	})
}

func repairObjectiveCitationFixtureLegacyBindings(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()
	mutateObjectiveCitationFixture(t, ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			UPDATE evidence
			SET payload_json=jsonb_set(
				payload_json,'{supports_claims}',
				jsonb_build_array(payload_json#>>'{metadata,requirement_id}')
			)
			WHERE kind='objective_citation'
		`); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			UPDATE step_completion_evidence_sets AS authority
			SET records_json=(
				SELECT jsonb_agg(
					jsonb_set(
						item.payload,'{supports_claims}',
						jsonb_build_array(item.payload#>>'{metadata,requirement_id}')
					)
					ORDER BY item.ordinality
				)
				FROM jsonb_array_elements(authority.records_json)
					 WITH ORDINALITY AS item(payload,ordinality)
			)
		`)
		return err
	})
}

func mutateObjectiveCitationFixture(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	mutate func(pgx.Tx) error,
) {
	t.Helper()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err := tx.Exec(ctx, `
		ALTER TABLE evidence
			DISABLE TRIGGER objective_completion_evidence_update_immutable;
		ALTER TABLE step_completion_evidence_sets
			DISABLE TRIGGER step_completion_evidence_sets_immutable;
	`); err != nil {
		t.Fatal(err)
	}
	if err := mutate(tx); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `
		ALTER TABLE evidence
			ENABLE TRIGGER objective_completion_evidence_update_immutable;
		ALTER TABLE step_completion_evidence_sets
			ENABLE TRIGGER step_completion_evidence_sets_immutable;
	`); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

func assertCurrentObjectiveCitationRequirementBindings(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
) {
	t.Helper()
	var invalidEvidence, invalidSets int
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM evidence
		WHERE payload_json ? 'supports_claims' OR
		      (kind='objective_citation' AND (
		          NOT payload_json ? 'requirement_authority_bindings' OR
		          payload_json->'requirement_authority_bindings'<>
		              jsonb_build_array(payload_json#>>'{metadata,requirement_id}')
		      ))
	`).Scan(&invalidEvidence); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM step_completion_evidence_sets AS authority
		WHERE EXISTS (
			SELECT 1 FROM jsonb_array_elements(authority.records_json) AS item(payload)
			WHERE item.payload ? 'supports_claims' OR
			      NOT item.payload ? 'requirement_authority_bindings'
		)
	`).Scan(&invalidSets); err != nil {
		t.Fatal(err)
	}
	if invalidEvidence != 0 || invalidSets != 0 {
		t.Fatalf("invalid migrated requirement bindings evidence=%d sets=%d", invalidEvidence, invalidSets)
	}
}
