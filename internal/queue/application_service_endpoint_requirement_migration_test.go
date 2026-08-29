package queue

import (
	"os"
	"strings"
	"testing"
)

const applicationServiceEndpointRequirementMigration = "135_application_service_endpoint_requirement_station.sql"

func TestApplicationServiceEndpointRequirementMigrationOwnsOnlyItsNarrowWork(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + applicationServiceEndpointRequirementMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"WHEN 'application_service_endpoint_requirement' THEN station='coding_service_endpoint_requirement'",
		"WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(",
		"9962ae4087be0e8bad32685271119ce1a988d7c0076c381f604038b98d0052c1",
		"65ffb44f3ba4aa764d10fadafb0928c30e02fa6abdfc192f45c2044eb79e65e3",
		"LANGUAGE SQL IMMUTABLE STRICT",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("service endpoint requirement migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"ALTER TABLE", "UPDATE ", "DELETE ", "fallback"} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("service endpoint requirement migration contains forbidden %q", forbidden)
		}
	}
}

func TestApplicationServiceEndpointRequirementMigrationPreservesExactOwnership(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "134")); err != nil {
		t.Fatal(err)
	}
	var before bool
	if err := pool.QueryRow(t.Context(), `
		SELECT station_owns_portable_work(
			'coding_service_endpoint_requirement','application_service_endpoint_requirement','{}'::jsonb
		)
	`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before {
		t.Fatal("migration 134 unexpectedly owns service endpoint requirement work")
	}
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "135")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, applicationServiceEndpointRequirementMigration, 1)
	var direct, wrong, cross, correction, malformed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_service_endpoint_requirement','application_service_endpoint_requirement','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_workload','application_service_endpoint_requirement','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_endpoint_requirement','application_service_endpoint_contract','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_endpoint_requirement','response_correction',
				'{"original":{"kind":"application_service_endpoint_requirement","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_endpoint_requirement','response_correction','{}'::jsonb
			)
	`).Scan(&direct, &wrong, &cross, &correction, &malformed); err != nil {
		t.Fatal(err)
	}
	if !direct || wrong || cross || !correction || malformed {
		t.Fatalf(
			"service endpoint requirement ownership direct/wrong/cross/correction/malformed=%v/%v/%v/%v/%v",
			direct, wrong, cross, correction, malformed,
		)
	}
}
