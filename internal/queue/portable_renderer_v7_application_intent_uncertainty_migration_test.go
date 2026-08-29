package queue

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelconfig"
	"github.com/gryph/omnidex/internal/scrum"
	"github.com/gryph/omnidex/internal/station"
	"github.com/jackc/pgx/v5"
)

const portableRendererV7ApplicationIntentUncertaintyMigration = "187_portable_renderer_v7_application_intent_uncertainty_v2.sql"

func TestPortableRendererV7ApplicationIntentUncertaintyMigrationIsExactAndNonMutating(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + portableRendererV7ApplicationIntentUncertaintyMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"portable renderer V7 requires exact terminal V5/V6 history and migration 186 authority",
		"'omnidex.render-portable-job.v5'",
		"'omnidex.render-portable-job.v6'",
		"'omnidex.render-portable-job.v7'",
		"'application_product_context'",
		"'application_requirement_coverage'",
		"'application_requirement'",
		"'application_project_stack_constraint'",
		"THEN '.v2' ELSE '.v1'",
		"new station gap opening requires portable renderer V7",
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
			t.Errorf("renderer V7 migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_gap_openings", "DELETE FROM", "TRUNCATE ",
		"DROP TABLE", "DROP COLUMN",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("renderer V7 migration mutates frozen history through %q", forbidden)
		}
	}
}

func TestPostgresPortableRendererV7PreservesApplicationIntentContractHistory(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "184"),
	); err != nil {
		t.Fatal(err)
	}

	v5Product := persistTerminalHistoricalIntentGap(
		t, repository, assemblyline.HistoricalPortableRendererV5, "renderer-v5-product",
		applicationProductContextGapRecord,
	)
	v5Requirement := persistTerminalHistoricalIntentGap(
		t, repository, assemblyline.HistoricalPortableRendererV5, "renderer-v5-requirement",
		applicationRequirementGapRecord,
	)
	v5Stack := persistTerminalHistoricalIntentGap(
		t, repository, assemblyline.HistoricalPortableRendererV5, "renderer-v5-stack",
		applicationProjectStackConstraintGapRecord,
	)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "185"),
	); err != nil {
		t.Fatal(err)
	}
	v6Product := persistTerminalHistoricalIntentGap(
		t, repository, assemblyline.HistoricalPortableRendererV6, "renderer-v6-product",
		applicationProductContextGapRecord,
	)
	v6Requirement := persistTerminalHistoricalIntentGap(
		t, repository, assemblyline.HistoricalPortableRendererV6, "renderer-v6-requirement",
		applicationRequirementGapRecord,
	)
	v6Stack := persistTerminalHistoricalIntentGap(
		t, repository, assemblyline.HistoricalPortableRendererV6, "renderer-v6-stack",
		applicationProjectStackConstraintGapRecord,
	)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "186"),
	); err != nil {
		t.Fatal(err)
	}
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "187"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, portableRendererV7ApplicationIntentUncertaintyMigration, 1)

	for _, historical := range []StationGapOpening{
		v5Product, v5Requirement, v5Stack, v6Product, v6Requirement, v6Stack,
	} {
		var renderer, contractID string
		if err := pool.QueryRow(t.Context(), `
			SELECT renderer_version,semantic_uncertainty_contract->>'id'
			FROM station_gap_openings WHERE id=$1
		`, historical.ID).Scan(&renderer, &contractID); err != nil {
			t.Fatal(err)
		}
		if renderer != historical.RendererVersion || !strings.HasSuffix(contractID, ".v1") {
			t.Fatalf("historical opening renderer/contract=%q/%q", renderer, contractID)
		}
	}

	for marker, record := range map[string]func(
		*testing.T, model.StepAttemptAuthority,
	) StationGapOpenRecord{
		"renderer-v7-product":     applicationProductContextGapRecord,
		"renderer-v7-requirement": applicationRequirementGapRecord,
		"renderer-v7-stack":       applicationProjectStackConstraintGapRecord,
	} {
		current := persistTerminalHistoricalIntentGap(
			t, repository, assemblyline.HistoricalPortableRendererV7, marker, record,
		)
		if current.RendererVersion != assemblyline.HistoricalPortableRendererV7 ||
			!strings.HasSuffix(current.SemanticUncertaintyContract.ID, ".v2") {
			t.Fatalf(
				"current opening renderer/contract=%q/%q",
				current.RendererVersion, current.SemanticUncertaintyContract.ID,
			)
		}
	}
}

func TestPostgresPortableRendererV7RejectsActiveCodingAndScrumJobs(t *testing.T) {
	for _, pipeline := range []string{model.PipelineCoding, model.PipelineScrum} {
		t.Run(pipeline, func(t *testing.T) {
			pool := openIsolatedMigrationPool(t)
			repository := New(pool)
			if err := repository.EnsureSchema(
				t.Context(), loadMigrationBundleThroughPrefix(t, "186"),
			); err != nil {
				t.Fatal(err)
			}
			if pipeline == model.PipelineCoding {
				if _, err := repository.EnqueueJob(
					t.Context(), "active-renderer-v6-coding", model.PipelineCoding, nil,
				); err != nil {
					t.Fatal(err)
				}
			} else {
				project, err := repository.CreateProject(
					t.Context(), "active-renderer-v6-scrum", t.TempDir(), "",
				)
				if err != nil {
					t.Fatal(err)
				}
				card, err := repository.CreateScrumCard(
					t.Context(), project.ID, "", "Active renderer V6", "", "assigned", nil, nil,
				)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := repository.EnqueueScrumJob(
					t.Context(), "active-renderer-v6-scrum", scrum.JobMetadata{
						Source: scrum.JobMetadataSource, ProjectID: project.ID, CardID: card.ID,
						CardTitle: card.Title, CardDescription: card.Description,
						ReturnColumn: card.Column, ModelConfig: modelconfig.Config{},
					},
				); err != nil {
					t.Fatal(err)
				}
			}
			err := repository.EnsureSchema(
				t.Context(), loadMigrationBundleThroughPrefix(t, "187"),
			)
			if err == nil || !strings.Contains(
				err.Error(), "requires exact terminal V5/V6 history",
			) {
				t.Fatalf("active %s renderer cutover error=%v", pipeline, err)
			}
			assertAppliedMigrationCount(
				t, pool, portableRendererV7ApplicationIntentUncertaintyMigration, 0,
			)
		})
	}
}

func TestPostgresPortableRendererV7RejectsOpenHistoricalGap(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "186"),
	); err != nil {
		t.Fatal(err)
	}
	claim := claimStationTestJob(t, repository, "renderer-v6-open-gap")
	opening, err := validateStationGapOpening(
		stationGapOpenFixture(t, claim.Authority),
	)
	if err != nil {
		t.Fatal(err)
	}
	opening.RendererVersion = assemblyline.HistoricalPortableRendererV6
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
	cancelStationTestClaim(t, repository, claim, "renderer-v6-open-gap")

	err = repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "187"),
	)
	if err == nil || !strings.Contains(
		err.Error(), "requires exact terminal V5/V6 history",
	) {
		t.Fatalf("open historical renderer gap error=%v", err)
	}
	assertAppliedMigrationCount(
		t, pool, portableRendererV7ApplicationIntentUncertaintyMigration, 0,
	)
}

func persistTerminalHistoricalIntentGap(
	t *testing.T,
	repository *Repository,
	renderer string,
	marker string,
	record func(*testing.T, model.StepAttemptAuthority) StationGapOpenRecord,
) StationGapOpening {
	t.Helper()
	claim := claimStationTestJob(t, repository, marker)
	opening, err := validateStationGapOpening(
		record(t, claim.Authority),
	)
	if err != nil {
		t.Fatal(err)
	}
	if (renderer == assemblyline.HistoricalPortableRendererV5 ||
		renderer == assemblyline.HistoricalPortableRendererV6 ||
		renderer == assemblyline.HistoricalPortableRendererV7) &&
		opening.WorkKind == string(assemblyline.WorkApplicationRequirementCoverage) {
		freezeHistoricalRequirementCoverageOpening(t, &opening, renderer)
	}
	opening.RendererVersion = renderer
	opening.SemanticUncertaintyContract, err =
		assemblyline.SemanticUncertaintyContractForPortableRenderer(
			renderer, assemblyline.WorkKind(opening.WorkKind),
		)
	if err != nil {
		t.Fatal(err)
	}
	opening.SemanticUncertaintyContractSHA256, err =
		opening.SemanticUncertaintyContract.Digest()
	if err != nil {
		t.Fatal(err)
	}
	projection, err := exactjson.Canonical(struct {
		Prompt   string `json:"prompt"`
		Renderer string `json:"renderer"`
	}{opening.Prompt, renderer})
	if err != nil {
		t.Fatal(err)
	}
	opening.ProjectionEnvelope = string(projection)
	opening.ProjectionSHA256 = stationGapSHA256(string(projection))

	tx, err := repository.pool.BeginTx(t.Context(), pgx.TxOptions{})
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
	persistStationDiscoveryFailure(t, repository, claim.Authority, opening)
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority, OpeningID: opening.ID, GapID: opening.GapID,
		Status: StationGapFailed, Error: "frozen renderer requirement fixture ended",
	}); err != nil {
		t.Fatal(err)
	}
	cancelStationTestClaim(t, repository, claim, marker)
	return opening
}

func freezeHistoricalRequirementCoverageOpening(
	t *testing.T,
	opening *StationGapOpening,
	renderer string,
) {
	t.Helper()
	var current assemblyline.ApplicationRequirementCoverageInput
	if err := json.Unmarshal([]byte(opening.PortablePayload), &current); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(struct {
		UserRequest          string                          `json:"user_request"`
		Context              assemblyline.ApplicationContext `json:"context"`
		ProductContext       string                          `json:"product_context"`
		AcceptedRequirements []string                        `json:"accepted_requirements"`
	}{
		UserRequest: current.UserRequest, Context: current.Context,
		ProductContext:       "A browser counter.",
		AcceptedRequirements: current.AcceptedRequirements,
	})
	if err != nil {
		t.Fatal(err)
	}
	job := assemblyline.PortableJob{
		Schema:  assemblyline.PortableJobSchemaV2,
		Kind:    assemblyline.WorkApplicationRequirementCoverage,
		Payload: payload,
	}
	job.ID = historicalPortableID(job.Schema, string(job.Kind), job.Payload)
	envelope, err := exactjson.Canonical(job)
	if err != nil {
		t.Fatal(err)
	}
	opening.GapID, opening.WorkID = job.ID, job.ID
	opening.PortablePayload = string(job.Payload)
	opening.PortablePayloadSHA256 = stationGapSHA256(opening.PortablePayload)
	opening.PortableEnvelope = string(envelope)
	opening.PortableEnvelopeSHA256 = stationGapSHA256(opening.PortableEnvelope)
	opening.Prompt = "FROZEN_APPLICATION_REQUIREMENT_COVERAGE_" +
		strings.ToUpper(strings.TrimPrefix(renderer, "omnidex.render-portable-job."))
}

func applicationProductContextGapRecord(
	t *testing.T,
	authority model.StepAttemptAuthority,
) StationGapOpenRecord {
	t.Helper()
	request := "Build a browser counter that displays and increments a count."
	context, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationProductContextJob(
		assemblyline.ApplicationProductContextInput{
			UserRequest: request,
			Context:     context,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	const contextTokens = 32768
	return StationGapOpenRecord{
		Authority: authority, Job: job, Station: station.CodingRequirements,
		ContextTokens: contextTokens,
		MaxOutputTokens: portableStationTestMaxOutputTokens(
			t, job, contextTokens,
		),
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	}
}

func applicationRequirementGapRecord(
	t *testing.T,
	authority model.StepAttemptAuthority,
) StationGapOpenRecord {
	t.Helper()
	request := "Build a browser counter that displays and increments a count."
	context, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationRequirementCoverageJob(
		assemblyline.ApplicationRequirementCoverageInput{
			UserRequest: request, Context: context,
			AcceptedRequirements: []string{"Display the current count."},
			ExcludedCandidates:   []string{},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	const contextTokens = 32768
	return StationGapOpenRecord{
		Authority: authority, Job: job, Station: station.CodingRequirements,
		ContextTokens: contextTokens,
		MaxOutputTokens: portableStationTestMaxOutputTokens(
			t, job, contextTokens,
		),
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	}
}

func applicationProjectStackConstraintGapRecord(
	t *testing.T,
	authority model.StepAttemptAuthority,
) StationGapOpenRecord {
	t.Helper()
	job, err := assemblyline.NewApplicationProjectStackConstraintJob(
		assemblyline.ApplicationProjectStackConstraintInput{
			UserRequest: "Build a command-line report using Go.",
			Candidates: []assemblyline.ApplicationProjectStackCandidate{{
				CandidateID:     "STACK_CANDIDATE_1",
				TechnicalFormat: "Go command-line application; packaging shape: one source and one verification",
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	const contextTokens = 32768
	return StationGapOpenRecord{
		Authority: authority, Job: job, Station: station.CodingProjectStackConstraint,
		ContextTokens: contextTokens,
		MaxOutputTokens: portableStationTestMaxOutputTokens(
			t, job, contextTokens,
		),
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	}
}
