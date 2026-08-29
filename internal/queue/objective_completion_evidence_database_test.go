package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresObjectiveCompletionCommitsEvidenceAndStepAtomically(t *testing.T) {
	repository, pool, claim := currentObjectiveCompletionTestClaim(t, "objective-evidence-atomic")
	if _, err := pool.Exec(t.Context(), `
		CREATE FUNCTION reject_second_objective_evidence() RETURNS TRIGGER AS $$
		BEGIN
			IF NEW.source_ref='reject-second' THEN RAISE EXCEPTION 'injected second evidence failure'; END IF;
			RETURN NEW;
		END $$ LANGUAGE plpgsql;
		CREATE TRIGGER reject_second_objective_evidence
		BEFORE INSERT ON evidence FOR EACH ROW EXECUTE FUNCTION reject_second_objective_evidence();
	`); err != nil {
		t.Fatal(err)
	}
	command := objectiveEvidenceCompletionCommand(t, claim, []evidence.Record{
		objectiveCompletionEvidence(claim, "first"),
		objectiveCompletionEvidence(claim, "reject-second"),
	})
	if err := repository.CompleteStepWithEvidence(t.Context(), command); err == nil ||
		!strings.Contains(err.Error(), "injected second evidence failure") {
		t.Fatalf("completion error=%v", err)
	}
	assertObjectiveCompletionState(t, pool, claim, model.StepStatusRunning, 0)

	if _, err := pool.Exec(t.Context(), `DROP TRIGGER reject_second_objective_evidence ON evidence`); err != nil {
		t.Fatal(err)
	}
	command = objectiveEvidenceCompletionCommand(t, claim, []evidence.Record{
		objectiveCompletionEvidence(claim, "first"),
		objectiveCompletionEvidence(claim, "second"),
	})
	if err := repository.CompleteStepWithEvidence(t.Context(), command); err != nil {
		t.Fatal(err)
	}
	assertObjectiveCompletionState(t, pool, claim, model.StepStatusCompleted, 2)
	if err := repository.CompleteStepWithEvidence(t.Context(), command); err != nil {
		t.Fatalf("exact replay failed: %v", err)
	}
	changed := command
	changed.Evidence = append([]evidence.Record(nil), command.Evidence...)
	changed.Evidence[0].SourceRef = "changed"
	if err := repository.CompleteStepWithEvidence(t.Context(), changed); err == nil ||
		!strings.Contains(err.Error(), "evidence set") {
		t.Fatalf("changed replay error=%v", err)
	}
	assertObjectiveEvidenceImmutability(t, pool, claim, command)
}

func TestPostgresObjectiveCompletionRejectsNonAtomicCitationWrite(t *testing.T) {
	repository, pool, claim := currentObjectiveCompletionTestClaim(t, "objective-evidence-sidecar")
	record := objectiveCompletionEvidence(claim, "sidecar")
	if err := repository.WriteEvidence(t.Context(), claim.Authority, record); err == nil ||
		!strings.Contains(err.Error(), "completion") {
		t.Fatalf("sidecar objective citation error=%v", err)
	}
	payload, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO evidence (job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb)
	`, record.JobID, record.StepID, record.Kind, record.SourceType, record.SourceRef,
		string(payload)); err == nil {
		t.Fatal("raw sidecar objective citation was accepted")
	}
	assertObjectiveCompletionState(t, pool, claim, model.StepStatusRunning, 0)
}

func currentObjectiveCompletionTestClaim(
	t *testing.T,
	marker string,
) (*Repository, *pgxpool.Pool, *model.ClaimedStep) {
	t.Helper()
	pool := openIsolatedDatabasePool(t)
	repository := New(pool)
	if err := repository.ResetDatabase(t.Context(), loadCurrentDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	job, err := repository.EnqueueJob(t.Context(), marker, model.PipelineCoding, json.RawMessage(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(t.Context(), marker+"-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("objective completion claim=%+v want job %d", claim, job.ID)
	}
	return repository, pool, claim
}

func assertObjectiveEvidenceImmutability(
	t *testing.T,
	pool *pgxpool.Pool,
	claim *model.ClaimedStep,
	command CompleteStepEvidenceCommand,
) {
	t.Helper()
	forgedPayload, err := json.Marshal(command.Evidence[0])
	if err != nil {
		t.Fatal(err)
	}
	var nonObjectiveID int64
	if err := pool.QueryRow(t.Context(), `
		INSERT INTO evidence (job_id,step_id,kind,source_type,source_ref,payload_json)
		VALUES ($1,$2,$3,$4,$5,$6::jsonb)
		RETURNING id
	`, claim.Job.ID, claim.Step.ID, evidence.KindCommandOutput,
		command.Evidence[0].SourceType, "preexisting-non-objective",
		string(forgedPayload)).Scan(&nonObjectiveID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		UPDATE evidence
		SET kind='objective_citation',completion_operation_id=$2,
			completion_evidence_index=2
		WHERE id=$1
	`, nonObjectiveID, command.OperationID); err == nil {
		t.Fatal("non-objective evidence was converted into completion authority")
	}
	var evidenceID int64
	if err := pool.QueryRow(t.Context(), `
		SELECT id FROM evidence WHERE completion_operation_id=$1
		ORDER BY completion_evidence_index LIMIT 1
	`, command.OperationID).Scan(&evidenceID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `UPDATE evidence SET source_ref='forged' WHERE id=$1`, evidenceID); err == nil {
		t.Fatal("objective completion evidence allowed an update")
	}
	if _, err := pool.Exec(t.Context(), `DELETE FROM evidence WHERE id=$1`, evidenceID); err == nil {
		t.Fatal("objective completion evidence allowed a delete")
	}
	payload, err := json.Marshal(command.Evidence[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		INSERT INTO evidence (
			job_id,step_id,kind,source_type,source_ref,payload_json,
			completion_operation_id,completion_evidence_index
		) VALUES ($1,$2,$3,$4,$5,$6::jsonb,$7,2)
	`, claim.Job.ID, claim.Step.ID, evidence.KindObjectiveCitation,
		command.Evidence[0].SourceType, command.Evidence[0].SourceRef,
		string(payload), command.OperationID); err == nil {
		t.Fatal("objective completion evidence set accepted an extra row")
	}
	assertObjectiveCompletionState(t, pool, claim, model.StepStatusCompleted, 2)
}

func objectiveEvidenceCompletionCommand(
	t *testing.T,
	claim *model.ClaimedStep,
	records []evidence.Record,
) CompleteStepEvidenceCommand {
	t.Helper()
	id, err := NewLifecycleOperationID(
		"objective-evidence", strconv.FormatInt(claim.Job.ID, 10),
		strconv.FormatInt(claim.Authority.Generation, 10),
		strconv.FormatInt(claim.Step.ID, 10), strconv.FormatInt(claim.Authority.Attempt, 10),
		claim.Authority.WorkerID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return CompleteStepEvidenceCommand{CompleteStepCommand: CompleteStepCommand{
		OperationID: id, Authority: claim.Authority, StepID: claim.Step.ID,
		Output: "grounded result", ContextKey: "objective_result", ContextValue: "objective-test",
	}, Evidence: records}
}

func objectiveCompletionEvidence(claim *model.ClaimedStep, sourceRef string) evidence.Record {
	excerpt := "Exact evidence " + sourceRef
	projection := sha256.Sum256([]byte(excerpt))
	sourceSHA := strings.Repeat("a", 64)
	return evidence.Record{
		JobID: claim.Job.ID, StepID: claim.Step.ID, Kind: evidence.KindObjectiveCitation,
		SourceType: "fixture", SourceRef: sourceRef, Excerpt: excerpt,
		Summary: "Exact objective citation.", Hash: sourceSHA, Confidence: 1,
		RequirementAuthorityBindings: []string{"requirement-test"},
		Metadata: map[string]any{
			"capsule_id": sourceRef, "instruction_sha256": strings.Repeat("b", 64),
			"objective_id": "objective-test", "objective_kind": "repository_read",
			"requirement_id":    "requirement-test",
			"projection_sha256": hex.EncodeToString(projection[:]),
			"source_sha256":     sourceSHA,
		},
	}
}

func assertObjectiveCompletionState(
	t *testing.T,
	pool *pgxpool.Pool,
	claim *model.ClaimedStep,
	wantStatus string,
	wantEvidence int,
) {
	t.Helper()
	var status string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM job_steps WHERE id=$1`, claim.Step.ID).Scan(&status); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(t.Context(), `
		SELECT COUNT(*) FROM evidence WHERE job_id=$1 AND step_id=$2 AND kind=$3
	`, claim.Job.ID, claim.Step.ID, evidence.KindObjectiveCitation).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if status != wantStatus || count != wantEvidence {
		t.Fatalf("step status=%q evidence=%d want %q/%d", status, count, wantStatus, wantEvidence)
	}
}
