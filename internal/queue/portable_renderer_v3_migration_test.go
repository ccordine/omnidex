package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const portableRendererV3Migration = "093_portable_renderer_v3.sql"

func TestPortableRendererV3MigrationRequiresCleanStartAndAcceptsOnlyCurrent(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + portableRendererV3Migration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"migration 093 reset required",
		"FROM station_gap_openings",
		"CREATE OR REPLACE FUNCTION station_owns_portable_work",
		"WHEN 'application_requirements' THEN station='coding_requirements'",
		"WHEN 'repository_requirements' THEN station='coding_requirements'",
		"WHEN 'application_job_specification' THEN station='coding_workload'",
		"WHEN 'application_job_specification_review' THEN station='coding_workload'",
		"WHEN 'application_job_specification_repair' THEN station='coding_workload'",
		"DROP CONSTRAINT station_gap_openings_renderer_version_check",
		"ADD CONSTRAINT station_gap_openings_renderer_version_check",
		assemblyline.PortableRendererV3,
	} {
		if !strings.Contains(source, required) {
			t.Errorf("renderer migration lacks %q", required)
		}
	}
	for _, forbidden := range []string{
		assemblyline.PortableRendererV1,
		assemblyline.PortableRendererV2,
		"WHEN 'application_identity'",
		"WHEN 'requirement_partition'",
		"WHEN 'application_workload_draft'",
		"WHEN 'application_workload_task'",
		"WHEN 'application_workload_task_repair'",
		"UPDATE station_gap_openings",
		"DELETE FROM",
		"fallback",
		"compatibility",
		"archive",
		"preserve",
	} {
		if strings.Contains(strings.ToLower(source), strings.ToLower(forbidden)) {
			t.Errorf("renderer migration contains forbidden %q", forbidden)
		}
	}
}

func TestPortableRendererV3RejectsHistoricalRowsAtomically(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "092")); err != nil {
		t.Fatal(err)
	}

	historicalV1 := rendererMigrationOpening(t, pool, repository, "renderer-v1-before-v3")
	historicalV1 = openingWithRenderer(t, historicalV1, assemblyline.PortableRendererV1)
	historicalV1 = insertRendererMigrationOpening(t, pool, historicalV1, false)
	historicalV2 := rendererMigrationOpening(t, pool, repository, "renderer-v2-before-v3")
	historicalV2 = openingWithRenderer(t, historicalV2, assemblyline.PortableRendererV2)
	historicalV2 = insertRendererMigrationOpening(t, pool, historicalV2, false)

	err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "093"))
	if err == nil || !strings.Contains(err.Error(), "migration 093 reset required") {
		t.Fatalf("renderer V3 dirty cutover error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, portableRendererV3Migration, 0)
	for _, historical := range []StationGapOpening{historicalV1, historicalV2} {
		var retained string
		if err := pool.QueryRow(t.Context(), `
			SELECT renderer_version FROM station_gap_openings WHERE id=$1
		`, historical.ID).Scan(&retained); err != nil {
			t.Fatal(err)
		}
		if retained != historical.RendererVersion {
			t.Fatalf("rejected cutover changed historical renderer=%q want %q", retained, historical.RendererVersion)
		}
	}
}

func TestPortableRendererV3FreshSchemaAcceptsCurrentRuntimeOpening(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "093")); err != nil {
		t.Fatal(err)
	}
	current := rendererMigrationOpening(t, pool, repository, "renderer-v3-fresh")
	current = insertRendererMigrationOpening(t, pool, current, false)
	var persisted string
	if err := pool.QueryRow(t.Context(), `
		SELECT renderer_version FROM station_gap_openings WHERE id=$1
	`, current.ID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	if persisted != assemblyline.PortableRendererV3 {
		t.Fatalf("fresh renderer=%q", persisted)
	}
}

func TestPortableRendererV3FreshSchemaOwnsOnlyCurrentApplicationWork(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "093")); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		station  string
		kind     string
		accepted bool
	}{
		{"coding_requirements", "application_requirements", true},
		{"coding_requirements", "repository_requirements", true},
		{"coding_workload", "application_job_specification", true},
		{"coding_workload", "application_job_specification_review", true},
		{"coding_workload", "application_job_specification_repair", true},
		{"coding_workload", "application_requirements", false},
		{"coding_product_identity", "application_identity", false},
		{"coding_requirement_partition", "requirement_partition", false},
		{"coding_workload", "application_workload_draft", false},
		{"coding_workload", "application_workload_task", false},
		{"coding_workload", "application_workload_task_repair", false},
	}
	for _, testCase := range cases {
		var accepted bool
		if err := pool.QueryRow(t.Context(), `
			SELECT station_owns_portable_work($1,$2,'{}'::jsonb)
		`, testCase.station, testCase.kind).Scan(&accepted); err != nil {
			t.Fatal(err)
		}
		if accepted != testCase.accepted {
			t.Fatalf(
				"station_owns_portable_work(%q,%q)=%v want %v",
				testCase.station, testCase.kind, accepted, testCase.accepted,
			)
		}
	}
}
