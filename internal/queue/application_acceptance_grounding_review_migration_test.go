package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

const applicationAcceptanceGroundingReviewMigration = "098_application_acceptance_grounding_review_station.sql"

func TestApplicationAcceptanceGroundingReviewMigrationReplacesOnlyStationOwnership(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../migrations/" + applicationAcceptanceGroundingReviewMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"CREATE OR REPLACE FUNCTION station_owns_portable_work",
		"WHEN 'application_classification' THEN station='coding_surface'",
		"WHEN 'application_requirements' THEN station='coding_requirements'",
		"WHEN 'repository_requirements' THEN station='coding_requirements'",
		"WHEN 'application_job_specification' THEN station='coding_workload'",
		"WHEN 'application_job_specification_review' THEN station='coding_workload'",
		"WHEN 'application_job_specification_repair' THEN station='coding_workload'",
		"WHEN 'application_acceptance_grounding_review' THEN station='coding_workload'",
		"WHEN 'repository_search_term' THEN station='coding_repository_search_term'",
		"WHEN 'repository_change_surface' THEN station='coding_repository_change_surface'",
		"WHEN 'repository_evidence_relevance' THEN station='repository_evidence_relevance'",
		"WHEN 'repository_grounded_review' THEN station='repository_grounded_review'",
		"WHEN 'repository_grounded_correction' THEN station='repository_grounded_correction'",
		"WHEN 'conversation_context_selection' THEN station='conversation_context_selection'",
		"WHEN 'memory_context_selection' THEN station='memory_context_selection'",
		"WHEN 'conversation_objective_kind' THEN station='conversation_objective_kind'",
		"WHEN 'conversation_response' THEN station='conversation_response'",
		"WHEN 'grounded_answer' THEN station='grounded_answer'",
		"WHEN 'web_search_terms' THEN station='web_search_terms'",
		"WHEN 'web_relevance' THEN station='web_relevance'",
		"WHEN 'web_grounded_synthesis' THEN station='web_grounded_synthesis'",
		"WHEN 'web_grounded_synthesis_correction' THEN station='web_grounded_synthesis_correction'",
		"WHEN 'web_claim_evidence_review' THEN station='web_claim_evidence_review'",
		"WHEN 'artifact_handling' THEN station='coding_artifact_handling'",
		"WHEN 'known_artifact_truth' THEN station='coding_known_artifact_truth'",
		"WHEN 'declaration_artifact_boundary' THEN station='coding_declaration_artifact_boundary'",
		"WHEN 'artifact_candidate_selection' THEN station='coding_artifact_candidate_selection'",
		"WHEN 'capability_relation' THEN station='coding_capability_relation'",
		"WHEN 'skill_selection' THEN station='coding_skill_selection'",
		"WHEN 'fragment_generation' THEN station='coding_fragment'",
		"WHEN 'fragment_modification' THEN station='coding_fragment'",
		"WHEN 'fragment_correction' THEN station='coding_fragment_correction'",
		"WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(",
		"LANGUAGE SQL IMMUTABLE STRICT",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("acceptance-grounding station migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"ALTER TABLE",
		"UPDATE ",
		"DELETE ",
		"INSERT ",
		"application_workload_task",
		"fallback",
		"compatibility",
	} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("acceptance-grounding station migration contains forbidden %q", forbidden)
		}
	}
}

func TestApplicationAcceptanceGroundingReviewMigrationPreservesEveryRegisteredStationOwner(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "097")); err != nil {
		t.Fatal(err)
	}

	var acceptedBefore bool
	if err := pool.QueryRow(t.Context(), `
		SELECT station_owns_portable_work($1,$2,'{}'::jsonb)
	`, station.CodingWorkload, assemblyline.WorkApplicationAcceptanceGroundingReview).Scan(&acceptedBefore); err != nil {
		t.Fatal(err)
	}
	if acceptedBefore {
		t.Fatal("migration 097 unexpectedly owns the new acceptance-grounding work kind")
	}

	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "098")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, applicationAcceptanceGroundingReviewMigration, 1)

	for _, kind := range assemblyline.AllWorkKinds() {
		if kind == assemblyline.WorkResponseCorrection {
			continue
		}
		want, err := stationForPortableWorkKind(kind)
		if err != nil {
			t.Fatalf("registered work kind %q has no runtime station: %v", kind, err)
		}
		var accepted, wrongStation bool
		if err := pool.QueryRow(t.Context(), `
			SELECT
				station_owns_portable_work($1,$2,'{}'::jsonb),
				station_owns_portable_work('not_the_owner',$2,'{}'::jsonb)
		`, want, kind).Scan(&accepted, &wrongStation); err != nil {
			t.Fatal(err)
		}
		if !accepted || wrongStation {
			t.Fatalf(
				"station ownership for %q: accepted=%v wrong_station=%v want true,false",
				kind, accepted, wrongStation,
			)
		}
	}

	for _, testCase := range []struct {
		station string
		kind    string
	}{
		{station: "coding_requirements", kind: "application_acceptance_grounding_review"},
		{station: "coding_workload", kind: "application_workload_task"},
		{station: "coding_workload", kind: "unregistered_work"},
	} {
		var accepted bool
		if err := pool.QueryRow(t.Context(), `
			SELECT station_owns_portable_work($1,$2,'{}'::jsonb)
		`, testCase.station, testCase.kind).Scan(&accepted); err != nil {
			t.Fatal(err)
		}
		if accepted {
			t.Fatalf("station_owns_portable_work(%q,%q) accepted forbidden pair", testCase.station, testCase.kind)
		}
	}

	var correctionAccepted, correctionWrongStation bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_workload',
				'response_correction',
				'{"original":{"kind":"application_acceptance_grounding_review","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_requirements',
				'response_correction',
				'{"original":{"kind":"application_acceptance_grounding_review","payload":{}}}'::jsonb
			)
	`).Scan(&correctionAccepted, &correctionWrongStation); err != nil {
		t.Fatal(err)
	}
	if !correctionAccepted || correctionWrongStation {
		t.Fatalf(
			"acceptance-grounding correction ownership accepted=%v wrong_station=%v want true,false",
			correctionAccepted, correctionWrongStation,
		)
	}
}
