package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const applicationFrontDoorStationMigration = "106_application_front_door_stations.sql"
const directSemanticReviewMigration = "109_direct_semantic_review_candidates.sql"
const removeCeremonialFrontDoorReviewMigration = "110_remove_ceremonial_front_door_reviews.sql"

func TestApplicationFrontDoorMigrationReplacesQuoteGateOwnership(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + applicationFrontDoorStationMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"CREATE OR REPLACE FUNCTION station_owns_portable_work",
		"WHEN 'application_context_needs' THEN station='coding_requirements'",
		"WHEN 'application_intent' THEN station='coding_requirements'",
		"WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(",
		"LANGUAGE SQL IMMUTABLE STRICT",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("application front-door migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"WHEN 'application_requirements'", "ALTER TABLE", "UPDATE ", "DELETE ", "INSERT ", "fallback",
	} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("application front-door migration contains forbidden %q", forbidden)
		}
	}
}

func TestApplicationFrontDoorMigrationMatchesCompleteCodeOwnedRouting(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, applicationFrontDoorStationMigration, 1)

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
}

func applicationFrontDoorWorkKind(kind assemblyline.WorkKind) bool {
	switch kind {
	case assemblyline.WorkKind("application_context_needs"),
		assemblyline.WorkKind("application_intent"),
		assemblyline.WorkApplicationProjectStackConstraint,
		assemblyline.WorkApplicationServiceEndpointRequirement,
		assemblyline.WorkApplicationServiceEndpointExposure,
		assemblyline.WorkApplicationServiceEndpointMethod,
		assemblyline.WorkApplicationServiceEndpointRouteTemplate,
		assemblyline.WorkApplicationServiceEndpointRequestMedia,
		assemblyline.WorkApplicationServiceEndpointResponseMedia,
		assemblyline.WorkApplicationServiceEndpointSuccessStatus:
		return true
	default:
		return false
	}
}

func TestDirectSemanticReviewMigrationRemovesObsoleteRepairWorkKinds(t *testing.T) {
	t.Parallel()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "109")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, directSemanticReviewMigration, 1)
	for _, obsolete := range []string{"application_intent_repair", "application_job_specification_repair"} {
		var accepted bool
		if err := pool.QueryRow(t.Context(), `
			SELECT station_owns_portable_work('coding_workload_review',$1,'{}'::jsonb)
		`, obsolete).Scan(&accepted); err != nil {
			t.Fatal(err)
		}
		if accepted {
			t.Fatalf("obsolete work kind %q remains routable", obsolete)
		}
	}
}

func TestCeremonialFrontDoorReviewMigrationRemovesRetiredReviewKinds(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + removeCeremonialFrontDoorReviewMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, retired := range []string{"application_intent_review", "application_job_specification_review"} {
		if strings.Contains(source, retired) {
			t.Fatalf("migration still routes retired ceremonial kind %q", retired)
		}
	}
	if !strings.Contains(source, "application_acceptance_grounding_review") {
		t.Fatal("migration removed the independent acceptance-grounding station")
	}
}
