package queue

import (
	"os"
	"strings"
	"testing"
)

func TestSemanticGapAuthorityMigrationIsAppendOnlyAndCompletionGuarded(t *testing.T) {
	t.Parallel()

	gapRaw, err := os.ReadFile("../../migrations/067_semantic_gap_authority.sql")
	if err != nil {
		t.Fatal(err)
	}
	callRaw, err := os.ReadFile("../../migrations/068_station_call_authority.sql")
	if err != nil {
		t.Fatal(err)
	}
	gapSource, callSource := string(gapRaw), string(callRaw)
	source := gapSource + callSource
	for _, required := range []string{
		"CREATE TABLE station_gap_openings", "CREATE TABLE station_gap_outcomes",
		"CREATE TABLE station_provider_discoveries", "CREATE TABLE station_provider_discovery_captures",
		"CREATE TABLE station_provider_discovery_receipts", "station_provider_discoveries_one_gap",
		"CREATE TABLE station_call_openings", "CREATE TABLE station_call_response_captures",
		"CREATE TABLE station_call_identity_captures", "CREATE TABLE station_call_receipts",
		"station_call_openings_one_gap", "station_call_receipts_one_terminal",
		"portable_schema", "portable_payload", "portable_envelope_sha256",
		"renderer_version", "projection_envelope", "projection_sha256", "opening_id",
		"validate_station_gap_outcome_insert", "prevent_station_gap_history_mutation",
		"BEFORE UPDATE OR DELETE", "BEFORE TRUNCATE", "station_gap_openings_one_identity",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("semantic-gap migration omitted %q", required)
		}
	}
	for _, required := range []string{
		"DROP TRIGGER station_gap_outcomes_require_discovery_receipt ON station_gap_outcomes",
		"DROP FUNCTION require_terminal_discovery_before_gap_outcome()",
		"NEW.response IS DISTINCT FROM call_response",
	} {
		if !strings.Contains(callSource, required) {
			t.Fatalf("station-call migration omitted transition guard %q", required)
		}
	}
	if strings.Contains(gapSource, "CREATE TABLE station_call_openings") {
		t.Fatal("typed-gap migration contains exact provider call authority")
	}
}
