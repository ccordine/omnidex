package queue

import (
	"strings"
	"testing"
)

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

	var jobs, discarded *string
	if err := pool.QueryRow(t.Context(), `
		SELECT to_regclass(current_schema() || '.jobs')::text,
		       to_regclass(current_schema() || '.discarded_startup_state')::text
	`).Scan(&jobs, &discarded); err != nil {
		t.Fatal(err)
	}
	if jobs == nil || discarded != nil {
		t.Fatalf("fresh setup relations jobs=%v discarded=%v", jobs, discarded)
	}

	var inventory, candidateKind, outcomeRelation, resultRelation bool
	var resultRelationGrounding bool
	var resultRelationCorrection bool
	var rendererConstraint, semanticContractConstraint string
	if err := pool.QueryRow(t.Context(), `
		SELECT station_owns_portable_work(
		           'coding_requirements','application_requirement_inventory','{}'::jsonb
		       ),
		       station_owns_portable_work(
		           'coding_requirements','application_requirement_candidate_kind','{}'::jsonb
		       ),
		       station_owns_portable_work(
		           'coding_requirements','application_requirement_candidate_outcome_relation','{}'::jsonb
		       ),
		       station_owns_portable_work(
		           'coding_requirements','application_requirement_candidate_result_relation','{}'::jsonb
		       ),
		       station_owns_portable_work(
		           'coding_requirements','application_requirement_candidate_result_relation_grounding','{}'::jsonb
		       ),
		       station_owns_portable_work(
		           'coding_requirements','application_requirement_candidate_result_relation_correction','{}'::jsonb
		       ),
		       pg_get_constraintdef(oid),
		       (SELECT pg_get_constraintdef(semantic.oid)
		          FROM pg_constraint AS semantic
		         WHERE semantic.conrelid='station_gap_openings'::regclass
		           AND semantic.conname='station_gap_openings_semantic_uncertainty_shape')
		FROM pg_constraint
		WHERE conrelid='station_gap_openings'::regclass
		  AND conname='station_gap_openings_renderer_version_check'
	`).Scan(
		&inventory,
		&candidateKind, &outcomeRelation, &resultRelation, &resultRelationGrounding,
		&resultRelationCorrection,
		&rendererConstraint,
		&semanticContractConstraint,
	); err != nil {
		t.Fatal(err)
	}
	if !inventory || !candidateKind || !outcomeRelation || !resultRelation ||
		!resultRelationGrounding ||
		!resultRelationCorrection ||
		rendererConstraint != "CHECK ((renderer_version = 'omnidex.render-portable-job.v1'::text))" ||
		!strings.Contains(
			semanticContractConstraint,
			"(work_kind = 'application_requirement_inventory'::text) THEN '.v9'::text",
		) ||
		!strings.Contains(
			semanticContractConstraint,
			"(work_kind = 'application_requirement_candidate_authorization'::text) THEN '.v7'::text",
		) ||
		strings.Contains(
			semanticContractConstraint,
			"(work_kind = 'application_requirement_inventory_cardinality'::text)",
		) ||
		!strings.Contains(semanticContractConstraint, "ELSE '.v1'::text") {
		t.Fatalf(
			"fresh renderer authority inventory=%t candidate_kind=%t outcome_relation=%t result_relation=%t result_relation_grounding=%t result_relation_correction=%t renderer_constraint=%q semantic_contract_constraint=%q",
			inventory, candidateKind, outcomeRelation, resultRelation, resultRelationGrounding,
			resultRelationCorrection,
			rendererConstraint,
			semanticContractConstraint,
		)
	}
}
