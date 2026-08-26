package queue

import (
	"os"
	"strings"
	"testing"
)

const applicationAcceptanceGroundingRetirementMigration = "136_retire_application_acceptance_grounding.sql"

func TestApplicationAcceptanceGroundingRetirementMigrationClosesNewOpenings(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../migrations/" + applicationAcceptanceGroundingRetirementMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"d6a479f722926498992a63d043383b8c313a643fa8b310d3303571cb558a04e0",
		"617105467a6ac384cb40cb8ef08503056e21507c974f29cfa02ec95215865310",
		"'application_acceptance_grounding_review'",
		"retired station work kind % cannot create a new opening",
		"retired station work kind % cannot create a correction opening",
		"cannot retire acceptance grounding while an active opening is unresolved",
		"original_kind IS DISTINCT FROM 'application_job_specification'",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("acceptance-grounding retirement migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"ALTER TABLE", "UPDATE ", "DELETE ", "fallback"} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("acceptance-grounding retirement migration contains forbidden %q", forbidden)
		}
	}
}

func TestApplicationAcceptanceGroundingRetirementPreservesHistoryButRejectsNewWork(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "136"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, applicationAcceptanceGroundingRetirementMigration, 1)

	var historicalOwner, currentWorkload bool
	var guardSHA256 string
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_workload_review','application_acceptance_grounding_review','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_workload','application_job_specification','{}'::jsonb
			),
			(
				SELECT encode(digest(convert_to(prosrc,'UTF8'),'sha256'),'hex')
				FROM pg_proc
				WHERE oid='enforce_context_sieve_station_opening_insert()'::regprocedure
			)
	`).Scan(&historicalOwner, &currentWorkload, &guardSHA256); err != nil {
		t.Fatal(err)
	}
	if !historicalOwner || !currentWorkload ||
		guardSHA256 != "617105467a6ac384cb40cb8ef08503056e21507c974f29cfa02ec95215865310" {
		t.Fatalf(
			"retirement authority historical/current/guard=%t/%t/%s",
			historicalOwner, currentWorkload, guardSHA256,
		)
	}

	for _, fixture := range []struct {
		name    string
		kind    string
		payload string
		message string
	}{
		{
			name: "direct", kind: "application_acceptance_grounding_review", payload: "{}",
			message: "retired station work kind application_acceptance_grounding_review cannot create a new opening",
		},
		{
			name: "correction", kind: "response_correction",
			payload: `{"original":{"kind":"application_acceptance_grounding_review","payload":{}}}`,
			message: "retired station work kind application_acceptance_grounding_review cannot create a correction opening",
		},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			_, err := pool.Exec(t.Context(), `
				INSERT INTO station_gap_openings (work_kind,portable_payload)
				VALUES ($1,$2)
			`, fixture.kind, fixture.payload)
			if err == nil || !strings.Contains(err.Error(), fixture.message) {
				t.Fatalf("retired opening error=%v want %q", err, fixture.message)
			}
		})
	}
}
