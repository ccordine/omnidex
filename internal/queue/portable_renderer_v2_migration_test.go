package queue

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
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
		"omnidex.render-portable-job.v1",
		"omnidex.render-portable-job.v2",
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
	if strings.Contains(source, "omnidex.render-portable-job.v3") {
		t.Fatalf("historical V2 migration contains future renderer %q", "omnidex.render-portable-job.v3")
	}
}

func TestPortableRendererV2MigratesHistoricalV1AndRejectsUnknown(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "091")); err != nil {
		t.Fatal(err)
	}

	historical := rendererMigrationOpening(t, pool, "renderer-v1")
	historical = openingWithRenderer(t, historical, "omnidex.render-portable-job.v1")
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
	if retained != "omnidex.render-portable-job.v1" {
		t.Fatalf("historical renderer=%q", retained)
	}

	current := rendererMigrationOpening(t, pool, "renderer-v2")
	current = openingWithRenderer(t, current, "omnidex.render-portable-job.v2")
	current = insertRendererMigrationOpening(t, pool, current, false)
	if current.RendererVersion != "omnidex.render-portable-job.v2" {
		t.Fatalf("current renderer=%q", current.RendererVersion)
	}

	unknown := rendererMigrationOpening(t, pool, "renderer-unknown")
	unknown = openingWithRenderer(t, unknown, "omnidex.render-portable-job.v999")
	_ = insertRendererMigrationOpening(t, pool, unknown, true)
}

func TestPortableRendererV2FreshSchemaAcceptsCurrentRuntimeOpening(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "092")); err != nil {
		t.Fatal(err)
	}
	current := rendererMigrationOpening(t, pool, "renderer-v2-fresh")
	current = openingWithRenderer(t, current, "omnidex.render-portable-job.v2")
	current = insertRendererMigrationOpening(t, pool, current, false)
	var persisted string
	if err := pool.QueryRow(t.Context(), `
		SELECT renderer_version FROM station_gap_openings WHERE id=$1
	`, current.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != "omnidex.render-portable-job.v2" {
		t.Fatalf("fresh renderer=%q", persisted)
	}
}

func rendererMigrationOpening(
	t *testing.T,
	pool *pgxpool.Pool,
	marker string,
) StationGapOpening {
	t.Helper()
	claim := seedPreInlineExecutionMigrationClaim(t, t.Context(), pool, marker)
	payload := json.RawMessage(
		`{"exact_instruction":"Exact renderer question.","kind":"answer"}`,
	)
	portable := assemblyline.PortableJob{
		Schema:  "omnidex.portable-job.v1",
		Kind:    assemblyline.WorkConversationResponse,
		Payload: payload,
	}
	portable.ID = historicalPortableID(
		portable.Schema, string(portable.Kind), portable.Payload,
	)
	portableEnvelope, err := exactjson.Canonical(portable)
	if err != nil {
		t.Fatal(err)
	}
	const (
		prompt   = "Exact historical renderer question."
		renderer = "omnidex.render-portable-job.v3"
	)
	responseSchema := json.RawMessage(`{}`)
	projection, err := exactjson.Canonical(struct {
		Prompt         string          `json:"prompt"`
		Renderer       string          `json:"renderer"`
		ResponseSchema json.RawMessage `json:"response_schema"`
	}{prompt, renderer, responseSchema})
	if err != nil {
		t.Fatal(err)
	}
	return StationGapOpening{
		JobID: claim.Authority.JobID, Generation: claim.Authority.Generation,
		StepID: claim.Authority.StepID, StepAttempt: claim.Authority.Attempt,
		WorkerID: claim.Authority.WorkerID, GapID: portable.ID,
		Station: station.ConversationResponse, Scope: "portable_semantic_worker",
		PortableSchema: portable.Schema, WorkID: portable.ID,
		WorkKind: string(portable.Kind), PortablePayload: string(portable.Payload),
		PortablePayloadSHA256:  stationGapSHA256(string(portable.Payload)),
		PortableEnvelope:       string(portableEnvelope),
		PortableEnvelopeSHA256: stationGapSHA256(string(portableEnvelope)),
		RendererVersion:        renderer, Prompt: prompt,
		ProjectionEnvelope: string(projection),
		ProjectionSHA256:   stationGapSHA256(string(projection)),
		ContextTokens:      8192, MaxOutputTokens: 8192,
	}
}

func openingWithRenderer(t *testing.T, opening StationGapOpening, renderer string) StationGapOpening {
	t.Helper()
	projection, err := exactjson.Canonical(struct {
		Prompt         string          `json:"prompt"`
		Renderer       string          `json:"renderer"`
		ResponseSchema json.RawMessage `json:"response_schema"`
	}{opening.Prompt, renderer, json.RawMessage(`{}`)})
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
	err = tx.QueryRow(t.Context(), `
		INSERT INTO station_gap_openings (
			job_id,generation,step_id,step_attempt,worker_id,gap_id,station,scope,
			portable_schema,work_id,work_kind,portable_payload,portable_payload_sha256,
			portable_envelope,portable_envelope_sha256,renderer_version,prompt,response_schema,
			projection_envelope,projection_sha256,context_tokens,max_output_tokens
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22
		)
		RETURNING id,created_at
	`, opening.JobID, opening.Generation, opening.StepID, opening.StepAttempt,
		opening.WorkerID, opening.GapID, opening.Station, opening.Scope, opening.PortableSchema,
		opening.WorkID, opening.WorkKind, opening.PortablePayload, opening.PortablePayloadSHA256,
		opening.PortableEnvelope, opening.PortableEnvelopeSHA256, opening.RendererVersion,
		opening.Prompt, "{}", opening.ProjectionEnvelope,
		opening.ProjectionSHA256, opening.ContextTokens, opening.MaxOutputTokens).Scan(
		&opening.ID, &opening.CreatedAt,
	)
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
