package queue

import (
	"os"
	"strings"
	"testing"
)

const applicationServiceEndpointContractMigration = "134_application_service_endpoint_contract_station.sql"

func TestApplicationServiceEndpointContractMigrationOwnsOnlyItsNarrowWork(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + applicationServiceEndpointContractMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"WHEN 'application_service_endpoint_contract' THEN station='coding_service_endpoint_contract'",
		"WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(",
		"db234d4b3d252d8bdf6050f9edbe262a85331e8b8acf67ca2c0c028182024a57",
		"9962ae4087be0e8bad32685271119ce1a988d7c0076c381f604038b98d0052c1",
		"LANGUAGE SQL IMMUTABLE STRICT",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("service endpoint contract migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"ALTER TABLE", "UPDATE ", "DELETE ", "fallback"} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("service endpoint contract migration contains forbidden %q", forbidden)
		}
	}
}

func TestApplicationServiceEndpointContractMigrationPreservesExactOwnership(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "133")); err != nil {
		t.Fatal(err)
	}
	var before bool
	if err := pool.QueryRow(t.Context(), `
		SELECT station_owns_portable_work(
			'coding_service_endpoint_contract','application_service_endpoint_contract','{}'::jsonb
		)
	`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before {
		t.Fatal("migration 133 unexpectedly owns service endpoint contract work")
	}
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "134")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, applicationServiceEndpointContractMigration, 1)
	var direct, wrong, cross, correction, malformed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_service_endpoint_contract','application_service_endpoint_contract','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_workload','application_service_endpoint_contract','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_endpoint_contract','application_target_tree','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_endpoint_contract','response_correction',
				'{"original":{"kind":"application_service_endpoint_contract","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_endpoint_contract','response_correction','{}'::jsonb
			)
	`).Scan(&direct, &wrong, &cross, &correction, &malformed); err != nil {
		t.Fatal(err)
	}
	if !direct || wrong || cross || !correction || malformed {
		t.Fatalf(
			"service endpoint ownership direct/wrong/cross/correction/malformed=%v/%v/%v/%v/%v",
			direct, wrong, cross, correction, malformed,
		)
	}
}
