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
	"github.com/gryph/omnidex/internal/station"
)

const artifactSemanticRelationSplitMigration = "180_artifact_semantic_relation_split.sql"

var artifactSemanticRelationOwners = []struct {
	station string
	kind    string
}{
	{"coding_repository_artifact_absence", "repository_artifact_absence"},
	{"coding_plain_text_artifact_creation", "plain_text_artifact_creation"},
}

func TestArtifactSemanticRelationSplitMigrationIsExactAndClosed(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + artifactSemanticRelationSplitMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE station_gap_openings, station_gap_outcomes, jobs",
		"e08e8fe89efa49d540d56769bde6a708cce6945c371236877bde3eab472aac24",
		"3d92903f413de33e2c35ad35217754e3d5ae5089e1d5e01c7069c6fded2eea47",
		"requires the exact migration 177 station function",
		"requires a fresh reset: incompatible active legacy opening exists",
		"WHEN 'repository_artifact_absence' THEN station='coding_repository_artifact_absence'",
		"WHEN 'plain_text_artifact_creation' THEN station='coding_plain_text_artifact_creation'",
		"CREATE OR REPLACE FUNCTION station_owns_portable_work",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("artifact semantic relation migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"WHEN 'known_artifact_truth'",
		"WHEN 'response_correction'",
		"COALESCE(station_owns_portable_work",
		"IF NOT EXISTS", "UPDATE ", "DELETE FROM", "CASCADE", "fallback",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("artifact semantic relation migration contains forbidden authority %q", forbidden)
		}
	}
}

func TestPostgresArtifactSemanticRelationsHaveOnlySeparateDirectOwners(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, artifactSemanticRelationSplitMigration, 1)

	for _, owner := range artifactSemanticRelationOwners {
		var direct, wrong bool
		if err := pool.QueryRow(t.Context(), `
			SELECT
				station_owns_portable_work($1,$2,'{}'::jsonb),
				station_owns_portable_work('not_the_owner',$2,'{}'::jsonb)
		`, owner.station, owner.kind).Scan(&direct, &wrong); err != nil {
			t.Fatal(err)
		}
		if !direct || wrong {
			t.Fatalf("artifact relation %s direct/wrong=%t/%t", owner.kind, direct, wrong)
		}
	}

	for _, retired := range []struct {
		station string
		kind    string
	}{
		{"coding_known_artifact_truth", "known_artifact_truth"},
		{"coding_repository_artifact_absence", "plain_text_artifact_creation"},
		{"coding_plain_text_artifact_creation", "repository_artifact_absence"},
	} {
		var accepted bool
		if err := pool.QueryRow(t.Context(), `
			SELECT station_owns_portable_work($1,$2,'{}'::jsonb)
		`, retired.station, retired.kind).Scan(&accepted); err != nil {
			t.Fatal(err)
		}
		if accepted {
			t.Fatalf("retired or cross-owned artifact relation remains accepted: %+v", retired)
		}
	}
}

func TestPostgresArtifactSemanticRelationSplitRejectsChangedPriorFunction(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "179"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		CREATE OR REPLACE FUNCTION station_owns_portable_work(
			station TEXT, work_kind TEXT, payload JSONB
		)
		RETURNS BOOLEAN AS 'SELECT TRUE' LANGUAGE SQL IMMUTABLE STRICT
	`); err != nil {
		t.Fatal(err)
	}

	err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(err.Error(), "requires the exact migration 177 station function") {
		t.Fatalf("changed prior station function migration error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, artifactSemanticRelationSplitMigration, 0)
}

func TestPostgresArtifactSemanticRelationSplitRejectsActiveLegacyOpening(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "179"),
	); err != nil {
		t.Fatal(err)
	}
	claim := claimStationTestJob(t, repository, "artifact-relation-active-legacy")
	opening := currentLegacyArtifactTruthOpening(t, claim)
	insertContextSieveMigrationOpening(t, pool, &opening)

	err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(
		err.Error(), "requires a fresh reset: incompatible active legacy opening exists",
	) {
		t.Fatalf("active legacy artifact opening migration error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, artifactSemanticRelationSplitMigration, 0)
}

func currentLegacyArtifactTruthOpening(
	t *testing.T,
	claim *model.ClaimedStep,
) StationGapOpening {
	t.Helper()
	payload := json.RawMessage(
		`{"requirement_quote":"One known semantic artifact must no longer exist."}`,
	)
	job := assemblyline.PortableJob{
		Schema:  assemblyline.PortableJobSchemaV2,
		Kind:    assemblyline.WorkKind("known_artifact_truth"),
		Payload: payload,
	}
	job.ID = historicalPortableID(job.Schema, string(job.Kind), job.Payload)
	envelope, err := exactjson.Canonical(job)
	if err != nil {
		t.Fatal(err)
	}
	responseSchema := json.RawMessage(`null`)
	projection, err := exactjson.Canonical(struct {
		Prompt         string          `json:"prompt"`
		Renderer       string          `json:"renderer"`
		ResponseSchema json.RawMessage `json:"response_schema"`
	}{
		Prompt:         "historical known artifact truth prompt",
		Renderer:       historicalPortableRendererV4,
		ResponseSchema: responseSchema,
	})
	if err != nil {
		t.Fatal(err)
	}
	return StationGapOpening{
		JobID: claim.Authority.JobID, Generation: claim.Authority.Generation,
		StepID: claim.Authority.StepID, StepAttempt: claim.Authority.Attempt,
		WorkerID: claim.Authority.WorkerID, GapID: job.ID,
		Station: station.ID("coding_known_artifact_truth"),
		Scope:   "portable_semantic_worker", PortableSchema: job.Schema,
		WorkID: job.ID, WorkKind: string(job.Kind), PortablePayload: string(job.Payload),
		PortablePayloadSHA256:  stationGapSHA256(string(job.Payload)),
		PortableEnvelope:       string(envelope),
		PortableEnvelopeSHA256: stationGapSHA256(string(envelope)),
		RendererVersion:        historicalPortableRendererV4,
		Prompt:                 "historical known artifact truth prompt",
		ProjectionEnvelope:     string(projection),
		ProjectionSHA256:       stationGapSHA256(string(projection)),
		ContextTokens:          32768, MaxOutputTokens: 32768,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	}
}
