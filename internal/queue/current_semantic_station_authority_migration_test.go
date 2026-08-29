package queue

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
)

const currentSemanticStationAuthorityMigration = "171_current_semantic_station_authority.sql"

func TestCurrentSemanticStationAuthorityMigrationIsExplicit(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + currentSemanticStationAuthorityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE OR REPLACE FUNCTION station_owns_portable_work",
		"DROP TRIGGER station_gap_openings_enforce_context_sieve_insert",
		"DROP FUNCTION enforce_context_sieve_station_opening_insert()",
		"CREATE FUNCTION current_model_config_is_valid",
		"projects_current_model_config",
		"jobs_current_model_config",
		"non-current model routing exists",
		"semantic station retirement requires a fresh reset",
		"WHEN 'application_requirement' THEN station='coding_requirements'",
		"WHEN 'web_synthesis_paragraph' THEN station='web_grounded_synthesis'",
		"WHEN 'fragment_correction' THEN station='coding_fragment_correction'",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("station authority migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"WHEN 'response_correction'",
		"WHEN 'context_search_term_coverage'", "WHEN 'context_search_term'",
		"WHEN 'repository_search_anchor_coverage'", "WHEN 'repository_search_anchor'",
		"WHEN 'repository_grounded_issue_detail'", "WHEN 'repository_grounded_issue_kind'",
		"WHEN 'repository_grounded_correction'", "WHEN 'application_job_objective'",
		"WHEN 'application_state_field_name'", "WHEN 'web_review_claim'",
		"DROP TRIGGER IF EXISTS", "DROP FUNCTION IF EXISTS", "CASCADE",
		"UPDATE ", "DELETE FROM",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("station authority migration retains forbidden authority %q", forbidden)
		}
	}
}

func TestCurrentSemanticStationAuthorityDatabaseShape(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	if err := New(pool).EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, currentSemanticStationAuthorityMigration, 1)

	for _, kind := range assemblyline.AllWorkKinds() {
		want, err := stationForPortableWorkKind(kind)
		if err != nil {
			t.Fatalf("registered work kind %q has no station: %v", kind, err)
		}
		var accepted, wrong bool
		if err := pool.QueryRow(ctx, `
			SELECT
				station_owns_portable_work($1,$2,'{}'::jsonb),
				station_owns_portable_work('not_the_owner',$2,'{}'::jsonb)
		`, want, kind).Scan(&accepted, &wrong); err != nil {
			t.Fatal(err)
		}
		if !accepted || wrong {
			t.Fatalf("station owner for %q accepted/wrong=%v/%v", kind, accepted, wrong)
		}
	}

	var guardExists bool
	if err := pool.QueryRow(ctx, `
		SELECT
			to_regprocedure('enforce_context_sieve_station_opening_insert()') IS NOT NULL OR
			EXISTS (
				SELECT 1 FROM pg_trigger
				WHERE tgrelid='station_gap_openings'::regclass AND
				      tgname='station_gap_openings_enforce_context_sieve_insert' AND
				      NOT tgisinternal
			)
	`).Scan(&guardExists); err != nil {
		t.Fatal(err)
	}
	if guardExists {
		t.Fatal("retired station opening guard remains installed")
	}

	for _, field := range modelconfig.Fields {
		var accepted bool
		if err := pool.QueryRow(ctx, `
			SELECT current_model_config_is_valid(jsonb_build_object($1::text,'qwen'))
		`, field.Key).Scan(&accepted); err != nil {
			t.Fatal(err)
		}
		if !accepted {
			t.Fatalf("current model route %q was rejected by PostgreSQL", field.Key)
		}
	}
	for name, raw := range map[string]string{
		"retired":       `{"web_claim_evidence_review_model":"qwen"}`,
		"unknown":       `{"unknown_model":"qwen"}`,
		"non-object":    `[]`,
		"non-string":    `{"conversation_response_model":7}`,
		"blank":         `{"conversation_response_model":""}`,
		"non-canonical": `{"conversation_response_model":" qwen "}`,
	} {
		var accepted bool
		if err := pool.QueryRow(ctx, `
			SELECT current_model_config_is_valid($1::jsonb)
		`, raw).Scan(&accepted); err != nil {
			t.Fatal(err)
		}
		if accepted {
			t.Fatalf("invalid %s model config was accepted: %s", name, raw)
		}
	}

	for _, retired := range []string{
		"response_correction", "context_search_term_coverage", "context_search_term",
		"repository_search_anchor_coverage", "repository_search_anchor",
		"repository_grounded_issue_detail", "repository_grounded_issue_kind",
		"repository_grounded_correction", "application_job_objective",
		"application_state_field_name",
		"web_review_claim", "web_grounded_synthesis_correction",
	} {
		var accepted bool
		if err := pool.QueryRow(ctx, `
			SELECT station_owns_portable_work('not_authoritative',$1,'{}'::jsonb)
		`, retired).Scan(&accepted); err != nil {
			t.Fatal(err)
		}
		if accepted {
			t.Fatalf("retired work kind %q remains authoritative", retired)
		}
	}
}

func TestCurrentSemanticStationAuthorityRejectsPersistedRetiredModelRoutes(t *testing.T) {
	for _, fixture := range []struct {
		name string
		seed func(context.Context, *Repository) error
	}{
		{
			name: "project",
			seed: func(ctx context.Context, repository *Repository) error {
				project, err := repository.CreateProject(ctx, "retired-route", "/tmp/retired-route", "")
				if err != nil {
					return err
				}
				_, err = repository.pool.Exec(ctx, `
					UPDATE projects
					SET settings='{"model_config":{"context_search_terms_model":"qwen"}}'::jsonb
					WHERE id=$1
				`, project.ID)
				return err
			},
		},
		{
			name: "job",
			seed: func(ctx context.Context, repository *Repository) error {
				_, err := repository.EnqueueJob(
					ctx, "retired route", model.PipelineCoding,
					json.RawMessage(`{"model_config":{"web_search_terms_model":"qwen"}}`),
				)
				return err
			},
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			pool := openIsolatedMigrationPool(t)
			repository := New(pool)
			if err := repository.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "170")); err != nil {
				t.Fatal(err)
			}
			if err := fixture.seed(ctx, repository); err != nil {
				t.Fatal(err)
			}
			err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t))
			if err == nil || !strings.Contains(err.Error(), "non-current model routing exists") {
				t.Fatalf("migration error=%v", err)
			}
		})
	}
}
