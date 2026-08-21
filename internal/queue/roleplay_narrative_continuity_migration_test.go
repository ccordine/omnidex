package queue

import (
	"os"
	"strings"
	"testing"
)

func TestRoleplayNarrativeContinuityMigrationAddsOnlyItsExactStationMapping(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/126_roleplay_narrative_continuity_station.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE OR REPLACE FUNCTION station_owns_portable_work",
		"WHEN 'roleplay_narrative_continuity' THEN station='roleplay_narrative_continuity'",
		"station_owns_portable_work('roleplay_narrative_continuity','roleplay_narrative_continuity'",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("roleplay narrative continuity migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE station_gap_openings", "DELETE FROM station_gap_openings",
		"fallback", "compatibility",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Errorf("roleplay narrative continuity migration contains forbidden %q", forbidden)
		}
	}
}

func TestRoleplayNarrativeContinuityMigrationChangesOnlyForwardStationOwner(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "125")); err != nil {
		t.Fatal(err)
	}
	var before bool
	if err := pool.QueryRow(t.Context(), `
		SELECT station_owns_portable_work(
			'roleplay_narrative_continuity','roleplay_narrative_continuity','{}'::jsonb
		)
	`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before {
		t.Fatal("migration 125 unexpectedly owns roleplay narrative continuity")
	}
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "126")); err != nil {
		t.Fatal(err)
	}
	var exact, responseOwnsContinuity, continuityOwnsResponse bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'roleplay_narrative_continuity','roleplay_narrative_continuity','{}'::jsonb
			),
			station_owns_portable_work(
				'conversation_response','roleplay_narrative_continuity','{}'::jsonb
			),
			station_owns_portable_work(
				'roleplay_narrative_continuity','conversation_response','{}'::jsonb
			)
	`).Scan(&exact, &responseOwnsContinuity, &continuityOwnsResponse); err != nil {
		t.Fatal(err)
	}
	if !exact || responseOwnsContinuity || continuityOwnsResponse {
		t.Fatalf(
			"station ownership exact/response/continuity=%v/%v/%v",
			exact, responseOwnsContinuity, continuityOwnsResponse,
		)
	}
}
