package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

const semanticUncertaintyContractAuthorityMigration = "182_semantic_uncertainty_contract_authority.sql"

func TestSemanticUncertaintyContractAuthorityMigrationIsFreshAndNonModelVisible(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + semanticUncertaintyContractAuthorityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE station_gap_openings, station_gap_outcomes",
		"ADD COLUMN semantic_uncertainty_contract JSONB NOT NULL",
		"ADD COLUMN semantic_uncertainty_contract_sha256 TEXT NOT NULL",
		"station_gap_openings_semantic_uncertainty_shape",
		"station_gap_openings_semantic_uncertainty_digest",
		"omnidex.semantic-uncertainty.'||work_kind||'.v1",
		"requires a fresh reset: inference history exists",
		"decode('00','hex')",
		"ab24dce607eb8b487a5428124003ff8d1936cfb4119613741a79436952cc9583",
		"e2a7b97bdf12e8edfc0f9625a528d2ec0b00d3fd846405c01a8c37274bced6ea",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("semantic uncertainty migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_gap_openings", "DELETE FROM", "TRUNCATE ",
		"DEFAULT '", "NOT VALID", " CASCADE", "projection_envelope=",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("semantic uncertainty migration contains forbidden authority %q", forbidden)
		}
	}
}

func TestPostgresSemanticUncertaintyContractAuthorityIsExact(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, semanticUncertaintyContractAuthorityMigration, 1)

	types := map[string]string{}
	rows, err := pool.Query(t.Context(), `
		SELECT column_name,data_type||'/'||is_nullable||'/'||COALESCE(column_default,'')
		FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='station_gap_openings'
		  AND column_name IN (
		      'semantic_uncertainty_contract',
		      'semantic_uncertainty_contract_sha256'
		  )
	`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var name, description string
		if err := rows.Scan(&name, &description); err != nil {
			t.Fatal(err)
		}
		types[name] = description
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if types["semantic_uncertainty_contract"] != "jsonb/NO/" ||
		types["semantic_uncertainty_contract_sha256"] != "text/NO/" {
		t.Fatalf("semantic uncertainty columns=%v", types)
	}

	want := map[string]string{
		"station_gap_openings_semantic_uncertainty_shape":  "ab24dce607eb8b487a5428124003ff8d1936cfb4119613741a79436952cc9583",
		"station_gap_openings_semantic_uncertainty_digest": "e2a7b97bdf12e8edfc0f9625a528d2ec0b00d3fd846405c01a8c37274bced6ea",
	}
	got := stationGapOpeningConstraintHashes(t, pool, stationGapOpeningMapKeys(want))
	for name, digest := range want {
		if got[name] != digest {
			t.Errorf("semantic uncertainty constraint %q=%q want %q", name, got[name], digest)
		}
	}
}

func TestPostgresSemanticUncertaintyAuthorityRejectsChangedPriorContract(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "181"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE station_gap_openings
		DROP CONSTRAINT station_gap_openings_projection_v5,
		ADD CONSTRAINT station_gap_openings_projection_v5 CHECK (TRUE)
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(err.Error(), "requires the exact response-schema retirement constraints") {
		t.Fatalf("changed prior semantic uncertainty authority error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, semanticUncertaintyContractAuthorityMigration, 0)
}

func TestPostgresSemanticUncertaintyAuthorityRejectsImmutableHistory(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "181"),
	); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(
		t.Context(), "semantic-uncertainty-history", model.PipelineCoding, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "semantic-uncertainty-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("history claim=%+v err=%v", claim, err)
	}
	opening, err := validateStationGapOpening(stationGapOpenFixture(t, claim.Authority))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO station_gap_openings (
			job_id,generation,step_id,step_attempt,worker_id,gap_id,station,scope,
			portable_schema,work_id,work_kind,portable_payload,portable_payload_sha256,
			portable_envelope,portable_envelope_sha256,renderer_version,prompt,
			projection_envelope,projection_sha256,context_tokens,max_output_tokens,output_limit_mode
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22
		)
	`, opening.JobID, opening.Generation, opening.StepID, opening.StepAttempt,
		opening.WorkerID, opening.GapID, opening.Station, opening.Scope,
		opening.PortableSchema, opening.WorkID, opening.WorkKind, opening.PortablePayload,
		opening.PortablePayloadSHA256, opening.PortableEnvelope,
		opening.PortableEnvelopeSHA256, opening.RendererVersion, opening.Prompt,
		opening.ProjectionEnvelope, opening.ProjectionSHA256, opening.ContextTokens,
		opening.MaxOutputTokens, opening.OutputLimitMode); err != nil {
		t.Fatal(err)
	}
	err = repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(err.Error(), "requires a fresh reset: inference history exists") {
		t.Fatalf("immutable semantic uncertainty history error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, semanticUncertaintyContractAuthorityMigration, 0)
}
