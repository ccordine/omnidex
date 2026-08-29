package queue

import (
	"os"
	"strings"
	"testing"
)

const stationPromptTransportMigration = "097_station_prompt_transport_resource.sql"

func TestStationPromptTransportMigrationUsesOneCoarseRequestCeiling(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../migrations/" + stationPromptTransportMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE station_gap_openings, station_call_openings IN ACCESS EXCLUSIVE MODE",
		"station_gap_openings_portable_payload_resource_ceiling",
		"station_gap_openings_portable_envelope_resource_ceiling",
		"station_gap_openings_prompt_resource_ceiling",
		"station_gap_openings_projection_resource_ceiling",
		"station_call_openings_wire_request_resource_ceiling",
		"station_call_openings_wire_request_bytes_resource_ceiling",
		"station_call_openings_model_input_resource_ceiling",
		"octet_length(portable_payload)<=1048576",
		"octet_length(portable_envelope)<=1048576",
		"octet_length(prompt)<=1048576",
		"octet_length(projection_envelope)<=1048576",
		"octet_length(wire_request) BETWEEN 1 AND 1048576",
		"wire_request_bytes BETWEEN 1 AND 1048576",
		"octet_length(model_input) BETWEEN 1 AND 1048576",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("station prompt transport migration omitted %q", required)
		}
	}
}

func TestStationPromptTransportDatabaseConstraintsShareCoarseCeiling(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	if err := New(pool).EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "097")); err != nil {
		t.Fatal(err)
	}

	constraints := map[string]string{
		"station_gap_openings_portable_payload_resource_ceiling":    "station_gap_openings",
		"station_gap_openings_portable_envelope_resource_ceiling":   "station_gap_openings",
		"station_gap_openings_prompt_resource_ceiling":              "station_gap_openings",
		"station_gap_openings_projection_resource_ceiling":          "station_gap_openings",
		"station_call_openings_wire_request_resource_ceiling":       "station_call_openings",
		"station_call_openings_wire_request_bytes_resource_ceiling": "station_call_openings",
		"station_call_openings_model_input_resource_ceiling":        "station_call_openings",
	}
	for name, table := range constraints {
		var definition string
		if err := pool.QueryRow(t.Context(), `
			SELECT pg_get_constraintdef(oid, true)
			FROM pg_constraint
			WHERE conrelid=$1::regclass AND conname=$2
		`, table, name).Scan(&definition); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(definition, "1048576") {
			t.Fatalf("%s.%s definition=%q lacks the shared coarse ceiling", table, name, definition)
		}
	}
}
