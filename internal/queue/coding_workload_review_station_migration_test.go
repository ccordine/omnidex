package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

const codingWorkloadReviewStationMigration = "101_coding_workload_review_station.sql"

func TestCodingWorkloadReviewMigrationChangesOnlyExactReviewOwnership(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../migrations/" + codingWorkloadReviewStationMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"CREATE OR REPLACE FUNCTION station_owns_portable_work",
		"WHEN 'application_job_specification' THEN station='coding_workload'",
		"WHEN 'application_job_specification_review' THEN station='coding_workload_review'",
		"WHEN 'application_job_specification_repair' THEN station='coding_workload'",
		"WHEN 'application_acceptance_grounding_review' THEN station='coding_workload'",
		"WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(",
		"LANGUAGE SQL IMMUTABLE STRICT",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("coding workload review migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"ALTER TABLE", "UPDATE ", "DELETE ", "INSERT ", "fallback"} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("coding workload review migration contains forbidden %q", forbidden)
		}
	}
}

func TestCodingWorkloadReviewMigrationPreservesPlannerAndMovesOnlyReview(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "100")); err != nil {
		t.Fatal(err)
	}

	var oldOwner, newOwner bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work($1,$3,'{}'::jsonb),
			station_owns_portable_work($2,$3,'{}'::jsonb)
	`, station.CodingWorkload, station.CodingWorkloadReview,
		assemblyline.WorkApplicationJobSpecificationReview).Scan(&oldOwner, &newOwner); err != nil {
		t.Fatal(err)
	}
	if !oldOwner || newOwner {
		t.Fatalf("migration 100 owners old/new=%v/%v want true/false", oldOwner, newOwner)
	}

	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "101")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, codingWorkloadReviewStationMigration, 1)

	for _, kind := range assemblyline.AllWorkKinds() {
		if kind == assemblyline.WorkResponseCorrection {
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

	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work($1,$3,'{}'::jsonb),
			station_owns_portable_work($2,$3,'{}'::jsonb)
	`, station.CodingWorkload, station.CodingWorkloadReview,
		assemblyline.WorkApplicationJobSpecificationReview).Scan(&oldOwner, &newOwner); err != nil {
		t.Fatal(err)
	}
	if oldOwner || !newOwner {
		t.Fatalf("migration 101 owners old/new=%v/%v want false/true", oldOwner, newOwner)
	}
}
