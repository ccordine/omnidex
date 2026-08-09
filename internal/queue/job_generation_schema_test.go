package queue

import (
	"os"
	"strings"
	"testing"
)

func TestJobGenerationMigrationDefinesOneAuthoritativeBoundary(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/028_job_generations.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"current_generation BIGINT NOT NULL DEFAULT 1",
		"IN SHARE ROW EXCLUSIVE MODE",
		"CREATE TABLE job_generations",
		"purpose TEXT NOT NULL",
		"purpose IN ('initial', 'replan')",
		"predecessor_generation = generation - 1",
		"boundary_action IN ('v3_coding', 'v3_planning')",
		"feedback_sha256 = encode(digest(feedback, 'sha256'), 'hex')",
		"generation BIGINT NOT NULL DEFAULT 1",
		"superseded_at_generation BIGINT",
		"superseded_at_generation > generation",
		"prevent_job_step_generation_identity_mutation",
		"job step execution identity cannot return to pending",
		"prevent_job_step_history_delete",
		"job_steps_history_delete_immutable",
		"job_steps_history_truncate_immutable",
		"ALTER COLUMN generation DROP DEFAULT",
		"BEFORE UPDATE OR DELETE ON job_generations",
		"BEFORE TRUNCATE ON job_generations",
		"idx_job_steps_current_generation_sort",
		"idx_job_steps_current_generation_action",
		"task_events_job_generation_fkey",
		"ALTER COLUMN job_generation DROP DEFAULT",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("job generation migration omitted %q", required)
		}
	}
}

func TestJobGenerationMigrationBindsDerivedRecordsToTheirJob(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/028_job_generations.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"artifacts_job_step_fkey",
		"evidence_job_step_fkey",
		"claims_job_step_fkey",
		"llm_call_evidence_job_step_fkey",
		"claim_support_job_claim_fkey",
		"claim_support_job_evidence_fkey",
		"memory_candidates_job_generation_fkey",
		"memory_candidates_job_id_fkey",
		"job step generation history is immutable",
		"claims_status_registered",
		"claims_confidence_bounded",
		"claim_support_score_bounded",
		"claim_support_rationale_exact",
		"memory_candidates_status_registered",
		"memory_candidates_confidence_bounded",
		"ON DELETE RESTRICT",
		"(job_id IS NULL AND step_id IS NULL)",
		"(job_id IS NOT NULL AND step_id IS NOT NULL)",
		"generation IS NULL",
		"generation IS NOT NULL",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("job generation ownership migration omitted %q", required)
		}
	}
}

func TestJobGenerationMigrationRejectsAmbiguousLegacyStateBeforeBackfill(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/028_job_generations.sql")
	if err != nil {
		t.Fatal(err)
	}
	schema := string(raw)
	for _, required := range []string{
		"replan_feedback",
		"nonterminal legacy-replanned job",
		"cross-job or orphan artifact",
		"cross-job or orphan evidence",
		"cross-job or orphan claim",
		"cross-job LLM call evidence",
		"cross-job claim support",
	} {
		if !strings.Contains(schema, required) {
			t.Fatalf("job generation preflight omitted %q", required)
		}
	}
}
