package queue

import (
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

const llmEvidenceTransportIdentityCutoverMigration = "183_llm_evidence_transport_identity_cutover.sql"

func TestLLMEvidenceTransportIdentityCutoverMigrationIsExactAndFresh(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile("../../migrations/" + llmEvidenceTransportIdentityCutoverMigration)
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"DROP COLUMN request_sha256",
		"DROP COLUMN response_format",
		"status IN ('generation_failed','succeeded')",
		"requires a fresh reset: immutable evidence exists",
		"137f98e5c9262e6611a28b2ea2a46a96bdf1ae176b6c896b2ea0078529673c50",
		"142000be76074a80703ad0af90e1ee0826a239583519af274d71f712830e37c5",
	} {
		if !strings.Contains(source, required) {
			t.Errorf("LLM evidence identity migration omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"UPDATE llm_call_evidence", "DELETE FROM", "TRUNCATE ", "NOT VALID", " CASCADE",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("LLM evidence identity migration contains forbidden mutation %q", forbidden)
		}
	}
}

func TestPostgresLLMEvidenceUsesOnlyStationWireIdentity(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	assertAppliedMigrationCount(t, pool, llmEvidenceTransportIdentityCutoverMigration, 1)

	var retiredColumns int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='llm_call_evidence'
		  AND column_name IN ('request_sha256','response_format')
	`).Scan(&retiredColumns); err != nil {
		t.Fatal(err)
	}
	if retiredColumns != 0 {
		t.Fatalf("retired LLM evidence column count=%d want 0", retiredColumns)
	}
	var statusHash string
	if err := pool.QueryRow(t.Context(), `
		SELECT encode(digest(convert_to(pg_get_constraintdef(oid),'UTF8'),'sha256'),'hex')
		FROM pg_constraint
		WHERE conrelid='llm_call_evidence'::regclass
		  AND conname='llm_call_evidence_status_check' AND convalidated
	`).Scan(&statusHash); err != nil {
		t.Fatal(err)
	}
	if statusHash !=
		"142000be76074a80703ad0af90e1ee0826a239583519af274d71f712830e37c5" {
		t.Fatalf("current LLM evidence status constraint=%q", statusHash)
	}
}

func TestPostgresLLMEvidenceIdentityCutoverRejectsChangedPriorAuthority(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "182"),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE llm_call_evidence
		DROP CONSTRAINT llm_call_evidence_status_check,
		ADD CONSTRAINT llm_call_evidence_status_check CHECK (TRUE)
	`); err != nil {
		t.Fatal(err)
	}
	err := repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(err.Error(), "requires the exact prior constraints") {
		t.Fatalf("changed prior LLM evidence authority error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, llmEvidenceTransportIdentityCutoverMigration, 0)
}

func TestPostgresLLMEvidenceIdentityCutoverRejectsImmutableHistory(t *testing.T) {
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(
		t.Context(), loadMigrationBundleThroughPrefix(t, "182"),
	); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(
		t.Context(), "llm-evidence-identity-history", model.PipelineCoding, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), "llm-evidence-identity-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("history claim=%+v err=%v", claim, err)
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE llm_call_evidence
			DISABLE TRIGGER llm_call_evidence_require_station_gap,
			DISABLE TRIGGER llm_call_evidence_validate_station_identity
	`); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO llm_call_evidence (
			job_id,job_generation,step_id,step_attempt,worker_id,scope,
			requested_model,model,attempt,system_prompt,user_prompt,request_sha256,
			response_format,context_tokens,max_output_tokens,response,response_sha256,
			status,error,latency_ms
		) VALUES (
			$1,$2,$3,$4,$5,'portable_semantic_worker','model','model',1,
			'system','user',repeat('a',64),'text',8192,8192,'answer',
			encode(digest('answer','sha256'),'hex'),'succeeded',NULL,1
		)
	`, claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID,
		claim.Authority.Attempt, claim.Authority.WorkerID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE llm_call_evidence
			ENABLE TRIGGER llm_call_evidence_require_station_gap,
			ENABLE TRIGGER llm_call_evidence_validate_station_identity
	`); err != nil {
		t.Fatal(err)
	}
	err = repository.EnsureSchema(t.Context(), loadCheckedMigrationBundle(t))
	if err == nil || !strings.Contains(err.Error(), "requires a fresh reset: immutable evidence exists") {
		t.Fatalf("immutable LLM evidence history error=%v", err)
	}
	assertAppliedMigrationCount(t, pool, llmEvidenceTransportIdentityCutoverMigration, 0)
}
