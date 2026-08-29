package queue

import (
	"os"
	"strings"
	"testing"
)

const applicationServiceDeploymentIntentMigration = "139_application_service_deployment_intent_station.sql"

func TestApplicationServiceDeploymentIntentMigrationRegistersExactAuthority(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + applicationServiceDeploymentIntentMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"cannot install service deployment intent station: prior station authority differs",
		"cannot install service deployment intent station: prior opening guard differs",
		"WHEN 'application_service_deployment_intent' THEN station='coding_service_deployment_intent'",
		"9dbad6b51b0c9efc761a864a8050e9437b61c23845b3ab642a50ce4c3af6614f",
		"d4cdac047bd4c3fbd84bd9fcebce5760bb6d02ed23621de675b45066d336c766",
		"33bbce2c9ec84a87fa185494b40f8038fb9531592c6af64ff88fd927cb0922f1",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("service deployment intent migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"ALTER TABLE", "UPDATE ", "DELETE ", "fallback"} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("service deployment intent migration contains forbidden %q", forbidden)
		}
	}
}

func TestApplicationServiceDeploymentIntentMigrationOwnsDirectAndCorrectionWork(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "139")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, applicationServiceDeploymentIntentMigration, 1)
	var direct, wrong, cross, correction, malformed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_service_deployment_intent','application_service_deployment_intent','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_requirements','application_service_deployment_intent','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_deployment_intent','application_service_state_lifetime','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_deployment_intent','response_correction',
				'{"original":{"kind":"application_service_deployment_intent","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_service_deployment_intent','response_correction','{}'::jsonb
			)
	`).Scan(&direct, &wrong, &cross, &correction, &malformed); err != nil {
		t.Fatal(err)
	}
	if !direct || wrong || cross || !correction || malformed {
		t.Fatalf(
			"service deployment intent ownership direct/wrong/cross/correction/malformed=%v/%v/%v/%v/%v",
			direct, wrong, cross, correction, malformed,
		)
	}
}
