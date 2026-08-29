package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

const stationResponseSchemaResourceMigration = "099_station_response_schema_resource.sql"

func TestStationResponseSchemaResourceMigrationDropsOnlyLegacyRuler(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../migrations/" + stationResponseSchemaResourceMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"octet_length(response_schema) <= 32768",
		"ALTER TABLE station_gap_openings DROP CONSTRAINT",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("response-schema resource migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"ADD CONSTRAINT", "1048576", "station_call_openings", "UPDATE ", "DELETE ", "INSERT ",
	} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("response-schema resource migration contains unrelated authority %q", forbidden)
		}
	}
}

func TestStationResponseSchemaResourceMigrationRemovesLiveDatabaseRuler(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "098")); err != nil {
		t.Fatal(err)
	}
	if count := stationResponseSchemaLegacyConstraintCount(t, pool); count != 1 {
		t.Fatalf("legacy response-schema constraint count=%d want 1", count)
	}
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "099")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, stationResponseSchemaResourceMigration, 1)
	if count := stationResponseSchemaLegacyConstraintCount(t, pool); count != 0 {
		t.Fatalf("response-schema byte ruler survived migration: count=%d", count)
	}

	var projectionDefinition string
	if err := pool.QueryRow(t.Context(), `
		SELECT pg_get_constraintdef(oid, true)
		FROM pg_constraint
		WHERE conrelid='station_gap_openings'::regclass
		  AND conname='station_gap_openings_projection_resource_ceiling'
	`).Scan(&projectionDefinition); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(projectionDefinition, "1048576") {
		t.Fatalf("coarse projection resource authority changed: %s", projectionDefinition)
	}
}

func stationResponseSchemaLegacyConstraintCount(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*)
		FROM pg_constraint
		WHERE conrelid='station_gap_openings'::regclass AND contype='c'
		  AND pg_get_constraintdef(oid) LIKE '%octet_length(response_schema) <= 32768%'
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}
