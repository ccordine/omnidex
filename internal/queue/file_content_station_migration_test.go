package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

const fileContentStationMigration = "112_file_content_station.sql"

func TestFileContentMigrationAddsOneExplicitOwner(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + fileContentStationMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"WHEN 'application_file_content' THEN station='coding_workload'",
		"WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(",
		"LANGUAGE SQL IMMUTABLE STRICT",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("file-content migration omitted %q", required)
		}
	}
}

func TestFileContentMigrationMatchesCodeOwnedRouting(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "112")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, fileContentStationMigration, 1)

	var owner, wrongOwner bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work($1,$3,'{}'::jsonb),
			station_owns_portable_work($2,$3,'{}'::jsonb)
	`, station.CodingWorkload, station.CodingWorkloadReview,
		assemblyline.WorkApplicationFileContent).Scan(&owner, &wrongOwner); err != nil {
		t.Fatal(err)
	}
	if !owner || wrongOwner {
		t.Fatalf("file-content station ownership=%v/%v want true/false", owner, wrongOwner)
	}
}
