package queue

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
	"github.com/jackc/pgx/v5/pgxpool"
)

const portableRendererV2Migration = "092_portable_renderer_v2.sql"

func TestPortableRendererV2MigrationPreservesOnlyHistoricalAndCurrentVersions(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + portableRendererV2Migration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"DROP CONSTRAINT station_gap_openings_renderer_version_check",
		"ADD CONSTRAINT station_gap_openings_renderer_version_check",
		assemblyline.PortableRendererV1,
		assemblyline.PortableRendererV2,
	} {
		if !strings.Contains(source, required) {
			t.Errorf("renderer migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{"UPDATE station_gap_openings", "DELETE FROM", "fallback", "compatibility"} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Errorf("renderer migration contains forbidden %q", forbidden)
		}
	}
	if strings.Contains(source, assemblyline.PortableRendererV3) {
		t.Fatalf("historical V2 migration contains future renderer %q", assemblyline.PortableRendererV3)
	}
}

func TestPortableRendererV2MigratesHistoricalV1AndRejectsUnknown(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "091")); err != nil {
		t.Fatal(err)
	}

	historical := rendererMigrationOpening(t, pool, repository, "renderer-v1")
	historical = openingWithRenderer(t, historical, assemblyline.PortableRendererV1)
	historical = insertRendererMigrationOpening(t, pool, historical, false)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "092")); err != nil {
		t.Fatal(err)
	}
	var retained string
	if err := pool.QueryRow(t.Context(), `
		SELECT renderer_version FROM station_gap_openings WHERE id=$1
	`, historical.ID).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != assemblyline.PortableRendererV1 {
		t.Fatalf("historical renderer=%q", retained)
	}

	current := rendererMigrationOpening(t, pool, repository, "renderer-v2")
	current = openingWithRenderer(t, current, assemblyline.PortableRendererV2)
	current = insertRendererMigrationOpening(t, pool, current, false)
	if current.RendererVersion != assemblyline.PortableRendererV2 {
		t.Fatalf("current renderer=%q", current.RendererVersion)
	}

	unknown := rendererMigrationOpening(t, pool, repository, "renderer-unknown")
	unknown = openingWithRenderer(t, unknown, "omnidex.render-portable-job.v999")
	_ = insertRendererMigrationOpening(t, pool, unknown, true)
}

func TestPortableRendererV2FreshSchemaAcceptsCurrentRuntimeOpening(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "092")); err != nil {
		t.Fatal(err)
	}
	current := rendererMigrationOpening(t, pool, repository, "renderer-v2-fresh")
	current = openingWithRenderer(t, current, assemblyline.PortableRendererV2)
	current = insertRendererMigrationOpening(t, pool, current, false)
	var persisted string
	if err := pool.QueryRow(t.Context(), `
		SELECT renderer_version FROM station_gap_openings WHERE id=$1
	`, current.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != assemblyline.PortableRendererV2 {
		t.Fatalf("fresh renderer=%q", persisted)
	}
}

func rendererMigrationOpening(
	t *testing.T,
	pool *pgxpool.Pool,
	repository *Repository,
	marker string,
) StationGapOpening {
	t.Helper()
	job, err := repository.EnqueueJob(t.Context(), marker, model.PipelineCoding, nil)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), marker+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	portable, err := assemblyline.NewConversationResponseJob(assemblyline.ConversationResponseInput{
		Kind: assemblyline.ObjectiveKindAnswer, ExactInstruction: "Exact renderer question.",
	})
	if err != nil {
		t.Fatal(err)
	}
	opening, err := validateStationGapOpening(StationGapOpenRecord{
		Authority: claim.Authority, Job: portable, Station: station.ConversationResponse,
		ContextTokens: 8192, MaxOutputTokens: 1024,
	})
	if err != nil {
		t.Fatal(err)
	}
	return opening
}

func openingWithRenderer(t *testing.T, opening StationGapOpening, renderer string) StationGapOpening {
	t.Helper()
	projection, err := exactjson.Canonical(struct {
		Prompt         string          `json:"prompt"`
		Renderer       string          `json:"renderer"`
		ResponseSchema json.RawMessage `json:"response_schema"`
	}{opening.Prompt, renderer, opening.ResponseSchema})
	if err != nil {
		t.Fatal(err)
	}
	opening.RendererVersion = renderer
	opening.ProjectionEnvelope = string(projection)
	opening.ProjectionSHA256 = stationGapSHA256(string(projection))
	return opening
}

func insertRendererMigrationOpening(
	t *testing.T,
	pool *pgxpool.Pool,
	opening StationGapOpening,
	wantError bool,
) StationGapOpening {
	t.Helper()
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	err = insertStationGapOpeningTx(t.Context(), tx, &opening)
	if wantError {
		if err == nil || !strings.Contains(err.Error(), "renderer_version") {
			t.Fatalf("unknown renderer insert error=%v", err)
		}
		return opening
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	return opening
}
