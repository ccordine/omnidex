package queue

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/station"
	"github.com/jackc/pgx/v5/pgxpool"
)

type historicalStationFixture struct {
	station  station.ID
	workKind string
}

func TestPostgresContextSieveCutoverPreservesCompletedLegacyStationOpenings(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "126"),
	); err != nil {
		t.Fatal(err)
	}
	claim := contextSieveMigrationClaim(t, repository, "completed-legacy-openings")

	fixtures := []historicalStationFixture{
		{station: station.ID("coding_requirements"), workKind: "application_requirements"},
		{station: station.ID("coding_workload"), workKind: "application_file_content"},
		{station: station.ID("coding_workload"), workKind: "application_job_specification_repair"},
		{station: station.ID("coding_workload"), workKind: "application_job_specification_review"},
		{station: station.ID("coding_workload_review"), workKind: "application_job_specification_review"},
	}
	priorDefinition := replaceStationOwnershipWithHistoricalInsertAuthority(t, pool)
	openings := make([]StationGapOpening, 0, len(fixtures))
	for index, fixture := range fixtures {
		job := historicalLegacyPortableJob(t, fixture.workKind, index)
		opening := historicalContextSieveOpening(t, claim, job, fixture.station)
		insertContextSieveMigrationOpening(t, pool, &opening)
		openings = append(openings, opening)
	}
	if _, err := pool.Exec(t.Context(), priorDefinition); err != nil {
		t.Fatal(err)
	}

	for _, opening := range openings {
		persistHistoricalStationDiscoveryFailure(t, repository, claim.Authority, opening)
		if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
			Authority: claim.Authority,
			OpeningID: opening.ID,
			GapID:     opening.GapID,
			Status:    StationGapFailed,
			Error:     "legacy station stopped before its retirement",
		}); err != nil {
			t.Fatal(err)
		}
	}

	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "127"),
	); err != nil {
		t.Fatalf("completed legacy station history blocked cutover: %v", err)
	}
	for _, opening := range openings {
		var retained, owned bool
		if err := pool.QueryRow(t.Context(), `
			SELECT outcome.id IS NOT NULL,
			       station_owns_portable_work(
			           opening.station,opening.work_kind,
			           opening.portable_payload::jsonb
			       )
			FROM station_gap_openings AS opening
			LEFT JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
			WHERE opening.id=$1
		`, opening.ID).Scan(&retained, &owned); err != nil {
			t.Fatal(err)
		}
		if !retained || !owned {
			t.Fatalf(
				"legacy opening %d retained/owned=%t/%t",
				opening.ID, retained, owned,
			)
		}
	}
}

func TestPostgresContextSieveCutoverRejectsEveryUnresolvedLegacyStationChain(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "126"),
	); err != nil {
		t.Fatal(err)
	}
	claim := contextSieveMigrationClaim(t, repository, "unresolved-legacy-chains")
	fixtures := []historicalStationFixture{
		{station: station.ID("coding_requirements"), workKind: "application_requirements"},
		{station: station.ID("coding_workload"), workKind: "application_file_content"},
		{station: station.ID("coding_workload"), workKind: "application_job_specification_repair"},
		{station: station.ID("coding_workload"), workKind: "application_job_specification_review"},
		{station: station.ID("coding_workload_review"), workKind: "application_job_specification_review"},
	}
	completed := make([]StationGapOpening, 0, len(fixtures)*2)
	marker := 100
	for _, fixture := range fixtures {
		for _, corrected := range []bool{false, true} {
			opening := insertUnresolvedHistoricalLegacyOpening(
				t, pool, claim, fixture, corrected, marker,
			)
			marker++
			err := repository.EnsureSchema(
				t.Context(), loadMigrationBundleThroughPrefix(t, "127"),
			)
			if err == nil || !strings.Contains(
				err.Error(), "unresolved opening contains retired station work",
			) {
				t.Fatalf(
					"legacy kind %s corrected=%t migration error=%v",
					fixture.workKind, corrected, err,
				)
			}
			assertContextSieveCutoverNotInstalled(t, pool)
			closeHistoricalLegacyOpening(t, repository, claim, opening)
			completed = append(completed, opening)
		}
	}

	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "127"),
	); err != nil {
		t.Fatalf("completed direct and corrected legacy history blocked cutover: %v", err)
	}
	var retained int
	if err := pool.QueryRow(t.Context(), `
		SELECT count(*)
		FROM station_gap_openings AS opening
		JOIN station_gap_outcomes AS outcome ON outcome.opening_id=opening.id
		WHERE opening.id=ANY($1)
	`, historicalOpeningIDs(completed)).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if retained != len(completed) {
		t.Fatalf("completed legacy openings retained=%d want %d", retained, len(completed))
	}
}

func insertUnresolvedHistoricalLegacyOpening(
	t *testing.T,
	pool *pgxpool.Pool,
	claim *model.ClaimedStep,
	fixture historicalStationFixture,
	corrected bool,
	marker int,
) StationGapOpening {
	t.Helper()
	job := historicalLegacyPortableJob(t, fixture.workKind, marker)
	if corrected {
		payload := mustCanonical(t, assemblyline.ResponseCorrectionInput{
			Original:          job,
			ValidationFailure: "legacy semantic value was invalid",
			RetainedCandidate: "{}",
		})
		job = assemblyline.PortableJob{
			Schema:  "omnidex.portable-job.v1",
			Kind:    assemblyline.WorkResponseCorrection,
			Payload: payload,
		}
		job.ID = historicalPortableID(job.Schema, string(job.Kind), job.Payload)
	}
	opening := historicalContextSieveOpening(t, claim, job, fixture.station)
	priorDefinition := replaceStationOwnershipWithHistoricalInsertAuthority(t, pool)
	insertContextSieveMigrationOpening(t, pool, &opening)
	if _, err := pool.Exec(t.Context(), priorDefinition); err != nil {
		t.Fatal(err)
	}
	return opening
}

func closeHistoricalLegacyOpening(
	t *testing.T,
	repository *Repository,
	claim *model.ClaimedStep,
	opening StationGapOpening,
) {
	t.Helper()
	persistHistoricalStationDiscoveryFailure(t, repository, claim.Authority, opening)
	if _, err := repository.CloseStationGap(t.Context(), StationGapTerminalRecord{
		Authority: claim.Authority,
		OpeningID: opening.ID,
		GapID:     opening.GapID,
		Status:    StationGapFailed,
		Error:     fmt.Sprintf("legacy station %s stopped before retirement", opening.WorkKind),
	}); err != nil {
		t.Fatal(err)
	}
}

func historicalOpeningIDs(openings []StationGapOpening) []int64 {
	ids := make([]int64, 0, len(openings))
	for _, opening := range openings {
		ids = append(ids, opening.ID)
	}
	return ids
}

func replaceStationOwnershipWithHistoricalInsertAuthority(
	t *testing.T,
	pool *pgxpool.Pool,
) string {
	t.Helper()
	var priorDefinition string
	if err := pool.QueryRow(t.Context(), `
		SELECT pg_get_functiondef(
			'station_owns_portable_work(text,text,jsonb)'::regprocedure
		)
	`).Scan(&priorDefinition); err != nil {
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
	return priorDefinition
}

func historicalLegacyPortableJob(
	t *testing.T,
	workKind string,
	marker int,
) assemblyline.PortableJob {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"historical_marker": marker})
	if err != nil {
		t.Fatal(err)
	}
	job := assemblyline.PortableJob{
		Schema:  "omnidex.portable-job.v1",
		Kind:    assemblyline.WorkKind(workKind),
		Payload: payload,
	}
	job.ID = historicalPortableID(job.Schema, string(job.Kind), job.Payload)
	return job
}
