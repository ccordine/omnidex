package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
	"github.com/jackc/pgx/v5"
)

const portableRendererV8RequirementCoverageAuthorityMigration = "188_portable_renderer_v8_requirement_coverage_authority.sql"

func TestPortableRendererV8RequirementCoverageAuthorityMigrationIsExactAndNonMutating(
	t *testing.T,
) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + portableRendererV8RequirementCoverageAuthorityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"portable renderer V8 requires exact terminal V5/V6/V7 history and migration 187 authority",
		"'omnidex.render-portable-job.v5'",
		"'omnidex.render-portable-job.v6'",
		"'omnidex.render-portable-job.v7'",
		"'omnidex.render-portable-job.v8'",
		"work_kind='application_requirement' THEN '.v3'",
		"'application_requirement_coverage'",
		"'application_requirement_candidate_cardinality'",
		"'application_requirement_candidate_split'",
		"'application_requirement_candidate_split_correction'",
		"OR renderer_version='omnidex.render-portable-job.v8'",
		"'application_project_stack_constraint'",
		"THEN '.v2' ELSE '.v1'",
		"new station gap opening requires portable renderer V8",
		"pipeline IN ('coding','scrum')",
		"status IN ('pending','running','waiting_input')",
		"outcome.id IS NULL",
		"station_gap_openings_immutable",
		"station_gap_openings_truncate_immutable",
		"station_gap_outcomes_immutable",
		"station_gap_outcomes_truncate_immutable",
		"fragment_generation_replacement_authority_is_exact",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("renderer V8 migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_gap_openings", "DELETE FROM", "TRUNCATE ",
		"DROP TABLE", "DROP COLUMN",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("renderer V8 migration mutates frozen history through %q", forbidden)
		}
	}
}

func TestPostgresPortableRendererV8PreservesV7HistoryAndOpensV8(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "187"),
	); err != nil {
		t.Fatal(err)
	}

	historical := persistTerminalHistoricalIntentGap(
		t, repository, assemblyline.HistoricalPortableRendererV7,
		"renderer-v7-stack-history", applicationProjectStackConstraintGapRecord,
	)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "188"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(
		t, pool, portableRendererV8RequirementCoverageAuthorityMigration, 1,
	)

	var renderer, payload, envelope, prompt, projection, contractID string
	if err := pool.QueryRow(t.Context(), `
		SELECT renderer_version,portable_payload,portable_envelope,prompt,
		       projection_envelope,semantic_uncertainty_contract->>'id'
		FROM station_gap_openings WHERE id=$1
	`, historical.ID).Scan(
		&renderer, &payload, &envelope, &prompt, &projection, &contractID,
	); err != nil {
		t.Fatal(err)
	}
	if renderer != assemblyline.HistoricalPortableRendererV7 ||
		payload != historical.PortablePayload || envelope != historical.PortableEnvelope ||
		prompt != historical.Prompt || projection != historical.ProjectionEnvelope ||
		!strings.HasSuffix(contractID, ".v2") {
		t.Fatalf(
			"frozen V7 history drifted: renderer=%q contract=%q payload=%t envelope=%t prompt=%t projection=%t",
			renderer, contractID, payload == historical.PortablePayload,
			envelope == historical.PortableEnvelope, prompt == historical.Prompt,
			projection == historical.ProjectionEnvelope,
		)
	}

	claim := claimStationTestJob(t, repository, "renderer-v8-current-stack")
	current, err := repository.OpenStationGap(
		t.Context(), applicationProjectStackConstraintGapRecord(t, claim.Authority),
	)
	if err != nil {
		t.Fatal(err)
	}
	if current.RendererVersion != assemblyline.PortableRendererV8 ||
		!strings.HasSuffix(current.SemanticUncertaintyContract.ID, ".v2") {
		t.Fatalf(
			"current opening renderer/contract=%q/%q",
			current.RendererVersion, current.SemanticUncertaintyContract.ID,
		)
	}

	cardinalityInput := assemblyline.ApplicationRequirementCandidateCardinalityInput{
		Candidate: "Play drum pads and a keyboard.",
	}
	cardinality, err := assemblyline.DecodeApplicationRequirementCandidateCardinalityResult(
		cardinalityInput, assemblyline.ApplicationRequirementMultipleRuntimeOutcomes,
	)
	if err != nil {
		t.Fatal(err)
	}
	cardinalityJob, err := assemblyline.NewApplicationRequirementCandidateCardinalityJob(
		cardinalityInput,
	)
	if err != nil {
		t.Fatal(err)
	}
	splitJob, err := assemblyline.NewApplicationRequirementCandidateSplitJob(
		assemblyline.ApplicationRequirementCandidateSplitInput{
			Candidate: cardinalityInput.Candidate, Cardinality: cardinality,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	correctionJob, err := assemblyline.NewApplicationRequirementCandidateSplitCorrectionJob(
		assemblyline.ApplicationRequirementCandidateSplitCorrectionInput{
			CurrentCandidate: cardinalityInput.Candidate,
			Cardinality:      cardinality,
			Defect:           assemblyline.ApplicationRequirementUnchangedSplitDefect,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	for marker, job := range map[string]assemblyline.PortableJob{
		"renderer-v8-cardinality": cardinalityJob,
		"renderer-v8-split":       splitJob,
		"renderer-v8-correction":  correctionJob,
	} {
		claim := claimStationTestJob(t, repository, marker)
		const contextTokens = 32768
		opening, err := repository.OpenStationGap(t.Context(), StationGapOpenRecord{
			Authority: claim.Authority, Job: job, Station: station.CodingRequirements,
			ContextTokens: contextTokens,
			MaxOutputTokens: portableStationTestMaxOutputTokens(
				t, job, contextTokens,
			),
			OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
		})
		if err != nil {
			t.Fatal(err)
		}
		if opening.RendererVersion != assemblyline.PortableRendererV8 ||
			!strings.HasSuffix(opening.SemanticUncertaintyContract.ID, ".v1") {
			t.Fatalf(
				"current refinement %s renderer/contract=%q/%q",
				job.Kind, opening.RendererVersion,
				opening.SemanticUncertaintyContract.ID,
			)
		}
	}
}

func TestPostgresPortableRendererV8RejectsActiveCodingJob(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "187"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.EnqueueJob(
		t.Context(), "active-renderer-v7-coding", model.PipelineCoding, nil,
	); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "188"),
	)
	if err == nil || !strings.Contains(err.Error(), "requires exact terminal V5/V6/V7 history") {
		t.Fatalf("active renderer V7 coding cutover error=%v", err)
	}
	assertAppliedMigrationCount(
		t, pool, portableRendererV8RequirementCoverageAuthorityMigration, 0,
	)
}

func TestPostgresPortableRendererV8RejectsOpenV7Gap(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "187"),
	); err != nil {
		t.Fatal(err)
	}
	claim := claimStationTestJob(t, repository, "renderer-v7-open-gap")
	opening, err := validateStationGapOpening(
		applicationProjectStackConstraintGapRecord(t, claim.Authority),
	)
	if err != nil {
		t.Fatal(err)
	}
	opening.RendererVersion = assemblyline.HistoricalPortableRendererV7
	opening.SemanticUncertaintyContract, err =
		assemblyline.SemanticUncertaintyContractForPortableRenderer(
			opening.RendererVersion, assemblyline.WorkKind(opening.WorkKind),
		)
	if err != nil {
		t.Fatal(err)
	}
	opening.SemanticUncertaintyContractSHA256, err = opening.SemanticUncertaintyContract.Digest()
	if err != nil {
		t.Fatal(err)
	}
	projection, err := exactjson.Canonical(struct {
		Prompt   string `json:"prompt"`
		Renderer string `json:"renderer"`
	}{opening.Prompt, opening.RendererVersion})
	if err != nil {
		t.Fatal(err)
	}
	opening.ProjectionEnvelope = string(projection)
	opening.ProjectionSHA256 = stationGapSHA256(string(projection))
	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := insertStationGapOpeningTx(t.Context(), tx, &opening); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	cancelStationTestClaim(t, repository, claim, "renderer-v7-open-gap")

	err = repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "188"),
	)
	if err == nil || !strings.Contains(err.Error(), "requires exact terminal V5/V6/V7 history") {
		t.Fatalf("open renderer V7 gap cutover error=%v", err)
	}
	assertAppliedMigrationCount(
		t, pool, portableRendererV8RequirementCoverageAuthorityMigration, 0,
	)
}
