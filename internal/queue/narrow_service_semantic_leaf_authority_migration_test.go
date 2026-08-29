package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

const narrowServiceSemanticLeafAuthorityMigration = "177_narrow_service_semantic_leaf_authority.sql"

var narrowServiceSemanticLeafOwners = []struct {
	station string
	kind    string
}{
	{"coding_service_endpoint_exposure", "application_service_endpoint_exposure"},
	{"coding_service_endpoint_method", "application_service_endpoint_method"},
	{"coding_service_endpoint_route_template", "application_service_endpoint_route_template"},
	{"coding_service_endpoint_request_media", "application_service_endpoint_request_media"},
	{"coding_service_endpoint_response_media", "application_service_endpoint_response_media"},
	{"coding_service_endpoint_success_status", "application_service_endpoint_success_status"},
	{"coding_application_state_field_coverage", "application_state_field_coverage"},
	{"coding_application_state_field_purpose", "application_state_field_purpose"},
	{"coding_application_state_field_kind", "application_state_field_kind"},
	{"coding_application_record_field_coverage", "application_record_field_coverage"},
	{"coding_application_record_field_purpose", "application_record_field_purpose"},
	{"coding_application_record_field_kind", "application_record_field_kind"},
}

func TestNarrowServiceSemanticLeafAuthorityMigrationIsExactAndClosed(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + narrowServiceSemanticLeafAuthorityMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE station_gap_openings, station_gap_outcomes, jobs",
		"7697634f9396160a6cc0d6091ac6cbee9bd8a1e553be040420b28fd6138c8167",
		"e08e8fe89efa49d540d56769bde6a708cce6945c371236877bde3eab472aac24",
		"requires the exact migration 176 station function",
		"requires a fresh reset: active invalid station opening exists",
		"CREATE OR REPLACE FUNCTION station_owns_portable_work",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("narrow service semantic leaf migration omitted %q", required)
		}
	}
	for _, owner := range narrowServiceSemanticLeafOwners {
		required := "WHEN '" + owner.kind + "' THEN station='" + owner.station + "'"
		if !strings.Contains(source, required) {
			t.Errorf("narrow service semantic leaf migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"WHEN 'application_service_endpoint_contract'",
		"WHEN 'application_service_state_interface'",
		"WHEN 'application_state_field_name'",
		"WHEN 'application_record_field_name'",
		"WHEN 'response_correction'",
		"COALESCE(station_owns_portable_work",
		"IF NOT EXISTS", "UPDATE ", "DELETE FROM", "CASCADE", "fallback",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("narrow service semantic leaf migration contains forbidden authority %q", forbidden)
		}
	}
}

func TestPostgresNarrowServiceSemanticLeafAuthorityOwnsOnlyDirectLeaves(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, narrowServiceSemanticLeafAuthorityMigration, 1)

	for _, owner := range narrowServiceSemanticLeafOwners {
		var direct, wrong bool
		if err := pool.QueryRow(t.Context(), `
			SELECT
				station_owns_portable_work($1,$2,'{}'::jsonb),
				station_owns_portable_work('not_the_owner',$2,'{}'::jsonb)
		`, owner.station, owner.kind).Scan(&direct, &wrong); err != nil {
			t.Fatal(err)
		}
		if !direct || wrong {
			t.Fatalf("leaf %s direct/wrong=%t/%t", owner.kind, direct, wrong)
		}
	}

	for _, retired := range []string{
		"application_service_endpoint_contract",
		"application_service_state_interface",
		"application_state_field_name",
		"application_record_field_name",
		"response_correction",
	} {
		var accepted bool
		if err := pool.QueryRow(t.Context(), `
			SELECT station_owns_portable_work(
				'not_an_authoritative_station',$1,'{}'::jsonb
			)
		`, retired).Scan(&accepted); err != nil {
			t.Fatal(err)
		}
		if accepted {
			t.Fatalf("retired aggregate or correction work %q remains owned", retired)
		}
	}

	var retained bool
	if err := pool.QueryRow(t.Context(), `
		SELECT station_owns_portable_work(
			'coding_requirements','application_requirement','{}'::jsonb
		)
	`).Scan(&retained); err != nil {
		t.Fatal(err)
	}
	if !retained {
		t.Fatal("migration discarded an existing current station mapping")
	}
}

func TestPostgresNarrowServiceSemanticLeafAuthorityRejectsChangedPriorFunction(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "176"),
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
	if err == nil || !strings.Contains(err.Error(), "requires the exact migration 176 station function") {
		t.Fatalf("changed prior station function migration error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, narrowServiceSemanticLeafAuthorityMigration, 0)
}

func TestPostgresNarrowServiceSemanticLeafAuthorityRejectsActiveInvalidOpening(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "176"),
	); err != nil {
		t.Fatal(err)
	}
	claim := claimStationTestJob(t, repository, "narrow-leaf-invalid-opening")
	record := stationGapOpenFixture(t, claim.Authority)
	opening, err := validateStationGapOpening(record)
	if err != nil {
		t.Fatal(err)
	}
	opening.Station = station.GroundedAnswer
	opening = freezeHistoricalRawStationGapV4(t, opening)

	var priorSource string
	if err := pool.QueryRow(t.Context(), `
		SELECT prosrc
		FROM pg_proc
		WHERE oid=to_regprocedure('station_owns_portable_work(text,text,jsonb)')
	`).Scan(&priorSource); err != nil {
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
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := insertHistoricalStationGapOpeningTx(t.Context(), tx, &opening); err != nil {
		_ = tx.Rollback(t.Context())
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	restore := `CREATE OR REPLACE FUNCTION station_owns_portable_work(
		station TEXT, work_kind TEXT, payload JSONB
	) RETURNS BOOLEAN AS $station_body$` + priorSource +
		`$station_body$ LANGUAGE SQL IMMUTABLE STRICT`
	if _, err := pool.Exec(t.Context(), restore); err != nil {
		t.Fatal(err)
	}

	err = repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(err.Error(), "active invalid station opening exists") {
		t.Fatalf("active invalid opening migration error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, narrowServiceSemanticLeafAuthorityMigration, 0)
}
