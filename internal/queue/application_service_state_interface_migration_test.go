package queue

import (
	"os"
	"strings"
	"testing"
)

const applicationServiceStateInterfaceMigration = "143_application_service_state_interface_station.sql"

func TestApplicationServiceStateInterfaceMigrationRegistersExactAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + applicationServiceStateInterfaceMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"cannot install service state interface station: prior station authority differs",
		"cannot install service state interface station: prior opening guard differs",
		"WHEN 'application_service_state_interface' THEN station='coding_service_state_interface'",
		"d4cdac047bd4c3fbd84bd9fcebce5760bb6d02ed23621de675b45066d336c766",
		"8d0f8ff8349569dbf1e32a3f75b0d315067dda3524ad5e8bf66486f4ace0e8d9",
		"33bbce2c9ec84a87fa185494b40f8038fb9531592c6af64ff88fd927cb0922f1",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("service state interface migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"ALTER TABLE", "UPDATE ", "DELETE ", "fallback"} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("service state interface migration contains forbidden %q", forbidden)
		}
	}
}

func TestApplicationServiceStateInterfaceMigrationOwnsDirectAndCorrectionWork(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "143")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, applicationServiceStateInterfaceMigration, 1)
	var direct, wrong, cross, correction, malformed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_service_state_interface','application_service_state_interface','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_workload','application_service_state_interface','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_state_interface','application_service_state_lifetime','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_state_interface','response_correction',
				'{"original":{"kind":"application_service_state_interface","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_state_interface','response_correction','{}'::jsonb
			)
	`).Scan(&direct, &wrong, &cross, &correction, &malformed); err != nil {
		t.Fatal(err)
	}
	if !direct || wrong || cross || !correction || malformed {
		t.Fatalf(
			"service state interface ownership direct/wrong/cross/correction/malformed=%v/%v/%v/%v/%v",
			direct, wrong, cross, correction, malformed,
		)
	}
}
