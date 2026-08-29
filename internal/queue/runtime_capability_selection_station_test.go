package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

const runtimeCapabilitySelectionMigration = "186_runtime_capability_selection_station.sql"

func TestRuntimeCapabilitySelectionHasOneExactStationOwner(t *testing.T) {
	t.Parallel()
	job, err := assemblyline.NewRuntimeCapabilitySelectionJob(
		assemblyline.RuntimeCapabilitySelectionInput{
			LocalContext: "A bounded command.", Need: "Read one external value.",
			Dialect: "Go 1.24 command-line source", AcceptedPurposes: []string{},
			Candidates: []assemblyline.RuntimeCapabilityCandidateSummary{{
				CandidateID: "RUNTIME_CAPABILITY_1",
				Purpose:     "Read one process environment value and report whether it is defined.",
			}},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := StationForPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if owner != station.CodingRuntimeCapabilitySelection {
		t.Fatalf("runtime capability owner=%q", owner)
	}
}

func TestRuntimeCapabilitySelectionMigrationIsNarrowAndPostRendererV6(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + runtimeCapabilitySelectionMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE jobs, station_gap_openings, station_gap_outcomes, station_provider_discoveries, station_provider_discovery_receipts, station_call_openings, station_call_receipts, llm_call_evidence IN ACCESS EXCLUSIVE MODE",
		"6e03d3f28a47eae720644b268139ffc85a832197ad77608a31e5a3f2f6c66fed",
		"fbb1c028c25638e7575af485d5111db344d91277b7d21e082296f62791f9b2e1",
		"a6fa583c1149290c2fb16434b167c3845abf1b2fcd227065a7380dd5073de782",
		"8d0f52db311639036d531237bcafc9cb6a07e0e630f43ab9d984281deb49bbad",
		"43f4b8e677fd23aa0e667566b613d7869c725ac5ea68f5d7ce7b245bb768e1ae",
		"fragment_generation_replacement_authority_is_exact() IS DISTINCT FROM TRUE",
		"tgtype=7",
		"WHEN 'runtime_capability_selection' THEN station='coding_runtime_capability_selection'",
		"LANGUAGE SQL IMMUTABLE STRICT",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("runtime capability migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"UPDATE ", "DELETE ", "fallback"} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("runtime capability migration contains forbidden %q", forbidden)
		}
	}
}

func TestRuntimeCapabilitySelectionMigrationOwnsOnlyItsLeaf(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "185"),
	); err != nil {
		t.Fatal(err)
	}
	var before bool
	if err := pool.QueryRow(t.Context(), `
		SELECT station_owns_portable_work(
			'coding_runtime_capability_selection','runtime_capability_selection','{}'::jsonb
		)
	`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before {
		t.Fatal("migration 185 unexpectedly owns runtime capability selection")
	}
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "186"),
	); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, runtimeCapabilitySelectionMigration, 1)
	var direct, wrong, cross bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_runtime_capability_selection','runtime_capability_selection','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_capability_relation','runtime_capability_selection','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_runtime_capability_selection','skill_selection','{}'::jsonb
			)
	`).Scan(&direct, &wrong, &cross); err != nil {
		t.Fatal(err)
	}
	if !direct || wrong || cross {
		t.Fatalf("runtime capability ownership direct/wrong/cross=%v/%v/%v", direct, wrong, cross)
	}
}
