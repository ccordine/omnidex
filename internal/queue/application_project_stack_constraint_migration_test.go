package queue

import (
	"os"
	"strings"
	"testing"
)

const applicationProjectStackConstraintMigration = "133_application_project_stack_constraint_station.sql"

func TestApplicationProjectStackConstraintMigrationOwnsOnlyItsNarrowWork(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + applicationProjectStackConstraintMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := strings.Join(strings.Fields(string(raw)), " ")
	for _, required := range []string{
		"LOCK TABLE station_gap_openings IN ACCESS EXCLUSIVE MODE",
		"WHEN 'application_project_stack_constraint' THEN station='coding_project_stack_constraint'",
		"WHEN 'response_correction' THEN COALESCE(station_owns_portable_work(",
		"d941a57abf2c27bd33b7a6c04014ce5b491bbbee397ec5fa0214efdb9025b1f6",
		"db234d4b3d252d8bdf6050f9edbe262a85331e8b8acf67ca2c0c028182024a57",
		"LANGUAGE SQL IMMUTABLE STRICT",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("project-stack constraint migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{"ALTER TABLE", "UPDATE ", "DELETE ", "fallback"} {
		if strings.Contains(strings.ToUpper(source), strings.ToUpper(forbidden)) {
			t.Fatalf("project-stack constraint migration contains forbidden %q", forbidden)
		}
	}
}

func TestApplicationProjectStackConstraintMigrationPreservesExactOwnership(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "132")); err != nil {
		t.Fatal(err)
	}
	var before bool
	if err := pool.QueryRow(t.Context(), `
		SELECT station_owns_portable_work(
			'coding_project_stack_constraint','application_project_stack_constraint','{}'::jsonb
		)
	`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before {
		t.Fatal("migration 132 unexpectedly owns project-stack constraint work")
	}
	if err := repository.EnsureSchema(t.Context(), loadMigrationBundleThroughPrefix(t, "133")); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, applicationProjectStackConstraintMigration, 1)
	var direct, wrong, cross, correction, malformed bool
	if err := pool.QueryRow(t.Context(), `
		SELECT
			station_owns_portable_work(
				'coding_project_stack_constraint','application_project_stack_constraint','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_requirements','application_project_stack_constraint','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_project_stack_constraint','application_target_tree','{}'::jsonb
			),
			station_owns_portable_work(
				'coding_project_stack_constraint','response_correction',
				'{"original":{"kind":"application_project_stack_constraint","payload":{}}}'::jsonb
			),
			station_owns_portable_work(
				'coding_project_stack_constraint','response_correction','{}'::jsonb
			)
	`).Scan(&direct, &wrong, &cross, &correction, &malformed); err != nil {
		t.Fatal(err)
	}
	if !direct || wrong || cross || !correction || malformed {
		t.Fatalf(
			"project-stack ownership direct/wrong/cross/correction/malformed=%v/%v/%v/%v/%v",
			direct, wrong, cross, correction, malformed,
		)
	}
}
