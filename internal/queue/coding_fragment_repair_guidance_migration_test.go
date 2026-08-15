package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

const codingFragmentRepairGuidanceMigration = "105_coding_fragment_repair_guidance_station.sql"

func TestCodingFragmentRepairGuidanceMigrationAddsOneExplicitOwner(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + codingFragmentRepairGuidanceMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"WHEN 'typescript_repair_guidance' THEN station='coding_fragment_repair_guidance'",
		"WHEN 'fragment_correction' THEN station='coding_fragment_correction'",
		"WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(",
		"LANGUAGE SQL IMMUTABLE STRICT",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("repair-guidance migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"ALTER TABLE", "UPDATE ", "DELETE ", "INSERT ", "fallback"} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("repair-guidance migration contains forbidden %q", forbidden)
		}
	}
}

func TestCodingFragmentRepairGuidanceMigrationMatchesCodeOwnedRouting(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "105")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, codingFragmentRepairGuidanceMigration, 1)

	for _, kind := range assemblyline.AllWorkKinds() {
		if kind == assemblyline.WorkResponseCorrection ||
			applicationFrontDoorWorkKind(kind) {
			continue
		}
		want, err := stationForPortableWorkKind(kind)
		if err != nil {
			t.Fatalf("registered work kind %q has no station: %v", kind, err)
		}
		var accepted, wrong bool
		if err := pool.QueryRow(t.Context(), `
			SELECT
				station_owns_portable_work($1,$2,'{}'::jsonb),
				station_owns_portable_work('not_the_owner',$2,'{}'::jsonb)
		`, want, kind).Scan(&accepted, &wrong); err != nil {
			t.Fatal(err)
		}
		if !accepted || wrong {
			t.Fatalf("station owner for %q accepted/wrong=%v/%v", kind, accepted, wrong)
		}
	}

	var guidance, executor bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work($1,$3,'{}'::jsonb),
			station_owns_portable_work($2,$3,'{}'::jsonb)
	`, station.CodingFragmentRepairGuidance, station.CodingFragmentCorrection,
		assemblyline.WorkTypeScriptRepairGuidance).Scan(&guidance, &executor); err != nil {
		t.Fatal(err)
	}
	if !guidance || executor {
		t.Fatalf("guidance/executor ownership=%v/%v want true/false", guidance, executor)
	}
}
