package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/jackc/pgx/v5"
)

const portableRendererV6Migration = "185_portable_renderer_v6.sql"

func TestPortableRendererV6MigrationPreservesFrozenHistory(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + portableRendererV6Migration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"'omnidex.render-portable-job.v5'",
		"'omnidex.render-portable-job.v6'",
		"require_current_station_gap_renderer",
		"station_gap_openings_prompt_projection",
		"BEFORE INSERT ON station_gap_openings",
		"outcome.id IS NULL",
		"59fec256f7ee7ba609115e0c37f4ac9ca1fe7d475e1c00a31ee66b9f5a17dc58",
		"station_gap_openings_immutable",
		"station_gap_openings_truncate_immutable",
		"station_gap_outcomes_immutable",
		"station_gap_outcomes_truncate_immutable",
		"fragment_generation_replacement_authority_is_exact",
		"43f4b8e677fd23aa0e667566b613d7869c725ac5ea68f5d7ce7b245bb768e1ae",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("renderer V6 migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"DELETE FROM", "TRUNCATE ", "UPDATE station_gap_openings"} {
		if strings.Contains(source, forbidden) {
			t.Errorf("renderer V6 migration mutates frozen history through %q", forbidden)
		}
	}
}

func TestPostgresPortableRendererV6RejectsMutableHistoryAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "184"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		DROP TRIGGER station_gap_openings_immutable ON station_gap_openings
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(
		err.Error(), "requires exact terminal V5 history",
	) {
		t.Fatalf("mutable renderer history authority error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, portableRendererV6Migration, 0)
}

func TestPostgresPortableRendererV6RejectsMissingReplacementLineageAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "184"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		DROP TRIGGER station_gap_replacement_origin_required
		ON station_gap_openings
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(
		err.Error(), "requires exact terminal V5 history",
	) {
		t.Fatalf("missing replacement lineage authority error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, portableRendererV6Migration, 0)
}

func TestPostgresPortableRendererV6Authority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "184")); err != nil {
		t.Fatal(err)
	}
	historicalClaim := claimStationTestJob(t, repository, "renderer-v5-history")
	historical, err := validateStationGapOpening(
		stationGapOpenFixture(t, historicalClaim.Authority),
	)
	if err != nil {
		t.Fatal(err)
	}
	historical.RendererVersion = assemblyline.HistoricalPortableRendererV5
	projection, err := exactjson.Canonical(struct {
		Prompt   string `json:"prompt"`
		Renderer string `json:"renderer"`
	}{historical.Prompt, historical.RendererVersion})
	if err != nil {
		t.Fatal(err)
	}
	historical.ProjectionEnvelope = string(projection)
	historical.ProjectionSHA256 = stationGapSHA256(string(projection))
	tx, err := pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := insertStationGapOpeningTx(t.Context(), tx, &historical); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	persistStationDiscoveryFailure(t, repository, historicalClaim.Authority, historical)
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: historicalClaim.Authority, OpeningID: historical.ID,
		GapID: historical.GapID, Status: StationGapFailed,
		Error: "frozen V5 fixture ended before renderer cutover",
	}); err != nil {
		t.Fatal(err)
	}
	cancelStationTestClaim(t, repository, historicalClaim, "renderer-v5-history")

	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "185"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, portableRendererV6Migration, 1)

	var rendererHash, guardHash string
	if err := pool.QueryRow(t.Context(), `
		SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
		FROM pg_constraint
		WHERE conrelid='station_gap_openings'::regclass
		  AND conname='station_gap_openings_renderer_version_check'
		  AND convalidated
	`).Scan(&rendererHash); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `
		SELECT encode(digest(convert_to(prosrc,'UTF8'),'sha256'),'hex')
		FROM pg_proc
		WHERE oid=to_regprocedure('require_current_station_gap_renderer()')
	`).Scan(&guardHash); err != nil {
		t.Fatal(err)
	}
	if rendererHash != "fbb1c028c25638e7575af485d5111db344d91277b7d21e082296f62791f9b2e1" ||
		guardHash != "a6fa583c1149290c2fb16434b167c3845abf1b2fcd227065a7380dd5073de782" {
		t.Fatalf("renderer V6 authority hashes=%s/%s", rendererHash, guardHash)
	}
	var retained string
	if err := pool.QueryRow(t.Context(), `
		SELECT renderer_version FROM station_gap_openings WHERE id=$1
	`, historical.ID).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != assemblyline.HistoricalPortableRendererV5 {
		t.Fatalf("frozen renderer=%q", retained)
	}

	currentClaim := claimStationTestJob(t, repository, "renderer-v6-current")
	current, err := validateStationGapOpening(
		stationGapOpenFixture(t, currentClaim.Authority),
	)
	if err != nil {
		t.Fatal(err)
	}
	current.RendererVersion = assemblyline.HistoricalPortableRendererV6
	projection, err = exactjson.Canonical(struct {
		Prompt   string `json:"prompt"`
		Renderer string `json:"renderer"`
	}{current.Prompt, current.RendererVersion})
	if err != nil {
		t.Fatal(err)
	}
	current.ProjectionEnvelope = string(projection)
	current.ProjectionSHA256 = stationGapSHA256(string(projection))
	tx, err = pool.BeginTx(t.Context(), pgx.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := insertStationGapOpeningTx(t.Context(), tx, &current); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if current.RendererVersion != assemblyline.HistoricalPortableRendererV6 {
		t.Fatalf("new opening renderer=%q", current.RendererVersion)
	}
	_, err = pool.Exec(t.Context(), `
		INSERT INTO station_gap_openings (
			job_id,generation,step_id,step_attempt,worker_id,gap_id,station,scope,
			portable_schema,work_id,work_kind,portable_payload,portable_payload_sha256,
			portable_envelope,portable_envelope_sha256,renderer_version,prompt,
			projection_envelope,projection_sha256,semantic_uncertainty_contract,
			semantic_uncertainty_contract_sha256,context_tokens,max_output_tokens,
			output_limit_mode
		)
		SELECT job_id,generation,step_id,step_attempt,worker_id,gap_id,station,scope,
		       portable_schema,work_id,work_kind,portable_payload,portable_payload_sha256,
		       portable_envelope,portable_envelope_sha256,$2,prompt,
		       projection_envelope,projection_sha256,semantic_uncertainty_contract,
		       semantic_uncertainty_contract_sha256,context_tokens,max_output_tokens,
		       output_limit_mode
		FROM station_gap_openings WHERE id=$1
	`, current.ID, assemblyline.HistoricalPortableRendererV5)
	if err == nil || !strings.Contains(err.Error(), "requires portable renderer V6") {
		t.Fatalf("new V5 opening error=%v", err)
	}
}
