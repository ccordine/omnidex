package queue

import (
	"os"
	"strings"
	"testing"
)

const applicationServiceStateLifetimeMigration = "138_application_service_state_lifetime_station.sql"

func TestApplicationServiceStateLifetimeMigrationRegistersExactAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + applicationServiceStateLifetimeMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"cannot install service state lifetime station: prior station authority differs",
		"cannot install service state lifetime station: prior opening guard differs",
		"WHEN 'application_service_state_lifetime' THEN station='coding_service_state_lifetime'",
		"9dbad6b51b0c9efc761a864a8050e9437b61c23845b3ab642a50ce4c3af6614f",
		"33bbce2c9ec84a87fa185494b40f8038fb9531592c6af64ff88fd927cb0922f1",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("service state lifetime migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"ALTER TABLE", "UPDATE ", "DELETE ", "fallback"} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("service state lifetime migration contains forbidden %q", forbidden)
		}
	}
}

func TestApplicationServiceStateLifetimeMigrationOwnsDirectAndCorrectionWork(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "138")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, applicationServiceStateLifetimeMigration, 1)
	var direct, wrong, cross, correction, malformed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_service_state_lifetime','application_service_state_lifetime','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_workload','application_service_state_lifetime','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_state_lifetime','application_service_endpoint_requirement','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_state_lifetime','response_correction',
				'{"original":{"kind":"application_service_state_lifetime","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_state_lifetime','response_correction','{}'::jsonb
			)
	`).Scan(&direct, &wrong, &cross, &correction, &malformed); err != nil {
		t.Fatal(err)
	}
	if !direct || wrong || cross || !correction || malformed {
		t.Fatalf(
			"service state ownership direct/wrong/cross/correction/malformed=%v/%v/%v/%v/%v",
			direct, wrong, cross, correction, malformed,
		)
	}
}
