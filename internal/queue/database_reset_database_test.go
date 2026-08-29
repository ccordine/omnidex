package queue

import "testing"

func TestResetDatabaseAlwaysReplacesTheExactRuntimeSchema(t *testing.T) {
	pool := openIsolatedDatabasePool(t)
	repository := New(pool)
	setup := loadCurrentDatabaseSetup(t)
	if err := repository.ResetDatabase(t.Context(), setup); err != nil {
		t.Fatal(err)
	}
	if err := repository.ValidateRuntimeAuthority(t.Context()); err != nil {
		t.Fatalf("fresh database runtime authority: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `CREATE TABLE discarded_startup_state (id bigint)`); err != nil {
		t.Fatal(err)
	}
	if err := repository.ResetDatabase(t.Context(), setup); err != nil {
		t.Fatal(err)
	}

	var jobs, discarded, ledger *string
	if err := pool.QueryRow(t.Context(), `
		SELECT to_regclass(current_schema() || '.jobs')::text,
		       to_regclass(current_schema() || '.discarded_startup_state')::text,
		       to_regclass(current_schema() || '.schema_migrations')::text
	`).Scan(&jobs, &discarded, &ledger); err != nil {
		t.Fatal(err)
	}
	if jobs == nil || discarded != nil || ledger != nil {
		t.Fatalf("fresh setup relations jobs=%v discarded=%v ledger=%v", jobs, discarded, ledger)
	}

	var candidateKind, duplicateReplacement bool
	var rendererConstraint string
	if err := pool.QueryRow(t.Context(), `
		SELECT station_owns_portable_work(
		           'coding_requirements','application_requirement_candidate_kind','{}'::jsonb
		       ),
		       station_owns_portable_work(
		           'coding_requirements','application_requirement_candidate_duplicate_replacement','{}'::jsonb
		       ),
		       pg_get_constraintdef(oid)
		FROM pg_constraint
		WHERE conrelid='station_gap_openings'::regclass
		  AND conname='station_gap_openings_renderer_version_check'
	`).Scan(&candidateKind, &duplicateReplacement, &rendererConstraint); err != nil {
		t.Fatal(err)
	}
	if !candidateKind || !duplicateReplacement ||
		rendererConstraint != "CHECK ((renderer_version = 'omnidex.render-portable-job.v1'::text))" {
		t.Fatalf(
			"fresh renderer authority candidate_kind=%t duplicate_replacement=%t constraint=%q",
			candidateKind, duplicateReplacement, rendererConstraint,
		)
	}
}
