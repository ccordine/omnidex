package queue

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestFreshSchemaRejectsStepCompletionWithUnterminatedLLMCall(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	pool, repository := freshEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "exercise terminal evidence guard", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "terminal-evidence-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	record := exactLLMEvidenceFixture(
		t, assemblyline.WorkApplicationClassify, "Classify a delivery.", "browser",
	)
	record.Authority = claim.Authority
	evidence, err := recordExactLLMEvidenceFixture(ctx, repository, record)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := setEvidenceAttemptStatus(ctx, pool, claim.Authority, "completed"); err == nil {
		t.Fatal("step completion accepted an LLM call without a terminal outcome")
	}
	projection, err := assemblyline.NewExactPortableResultProjection(record.Generation.Content)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
		Authority: claim.Authority, CallEvidenceID: evidence.ID,
		Candidate: record.Generation.Content, Projection: &projection,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := setEvidenceAttemptStatus(ctx, pool, claim.Authority, "completed"); err != nil {
		t.Fatalf("completed exact terminal evidence chain: %v", err)
	}
}

func TestFreshSchemaAbandonedLLMOpeningBlocksCompletionAndExpiresInterrupted(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	pool, repository := freshEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "exercise abandoned provider call", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "abandoned-call-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	record := exactLLMEvidenceFixture(
		t, assemblyline.WorkApplicationClassify, "Classify before process death.", "browser",
	)
	record.Authority = claim.Authority
	opening, err := repository.ReserveLLMCallEvidence(ctx, record.LLMCallOpeningRecord)
	if err != nil || opening.ProviderReceiptPresent || opening.Outcome != nil {
		t.Fatalf("opening=%#v err=%v", opening, err)
	}
	if _, err := setEvidenceAttemptStatus(ctx, pool, claim.Authority, "completed"); err == nil {
		t.Fatal("step completion accepted an abandoned LLM opening")
	}
	if _, err := setEvidenceAttemptStatus(ctx, pool, claim.Authority, "expired"); err != nil {
		t.Fatalf("expire abandoned LLM opening: %v", err)
	}
	calls, err := listAllLLMCallEvidenceForJob(ctx, repository, job.ID)
	if err != nil || len(calls) != 1 || calls[0].ProviderReceiptPresent ||
		calls[0].Outcome == nil || calls[0].Outcome.Status != LLMCallInterrupted {
		t.Fatalf("abandoned call=%#v err=%v", calls, err)
	}
}

func TestFreshSchemaTerminalAttemptPreservesBothLLMCallRaceOrderings(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	pool, repository := freshEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "exercise terminal evidence races", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "terminal-race-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}

	receiptFirst := exactLLMEvidenceFixture(
		t, assemblyline.WorkApplicationClassify, "Classify before cancellation.", "browser",
	)
	receiptFirst.Authority = claim.Authority
	receiptEvidence, err := recordExactLLMEvidenceFixture(ctx, repository, receiptFirst)
	if err != nil || receiptEvidence.Outcome != nil {
		t.Fatalf("receipt-first evidence=%#v err=%v", receiptEvidence, err)
	}
	terminalFirst := exactLLMEvidenceFixture(
		t, assemblyline.WorkArtifactHandling, "Classify ARTIFACT_1 after cancellation.", "must_exist",
	)
	terminalFirst.Authority = claim.Authority
	terminalFirst.WorkID = strings.Repeat("b", 64)
	terminalOpening, err := repository.ReserveLLMCallEvidence(ctx, terminalFirst.LLMCallOpeningRecord)
	if err != nil {
		t.Fatalf("reserve provider-in-flight call: %v", err)
	}
	if _, err := setEvidenceAttemptStatus(ctx, pool, claim.Authority, "canceled"); err != nil {
		t.Fatal(err)
	}

	projection, err := assemblyline.NewExactPortableResultProjection(receiptFirst.Generation.Content)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
		Authority: claim.Authority, CallEvidenceID: receiptEvidence.ID,
		Candidate: receiptFirst.Generation.Content, Projection: &projection,
	}); !errors.Is(err, ErrLLMCallTerminalizedByAttempt) {
		t.Fatalf("terminalized call outcome err=%v", err)
	}
	calls, err := listAllLLMCallEvidenceForJob(ctx, repository, job.ID)
	if err != nil || len(calls) != 2 || calls[0].Outcome == nil ||
		calls[0].Outcome.Status != LLMCallRejected || calls[1].Outcome == nil ||
		calls[1].Outcome.Status != LLMCallInterrupted || calls[1].ProviderReceiptPresent {
		t.Fatalf("cancellation outcomes=%#v err=%v", calls, err)
	}
	lateEvidence, err := repository.FinalizeLLMCallEvidence(
		ctx, terminalFirst.receipt(terminalOpening.ID),
	)
	if !errors.Is(err, ErrLLMCallTerminalizedByAttempt) ||
		!lateEvidence.ProviderReceiptPresent || lateEvidence.Outcome == nil ||
		lateEvidence.Outcome.Status != LLMCallInterrupted ||
		string(lateEvidence.RawResponse) != string(terminalFirst.Generation.ProviderResponseCapture) {
		t.Fatalf("late provider receipt=%#v err=%v", lateEvidence, err)
	}
}

func TestFreshSchemaCompletedAttemptRejectsNewEvidence(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	pool, repository := freshEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "reject evidence after completion", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "completed-race-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	if _, err := setEvidenceAttemptStatus(ctx, pool, claim.Authority, "completed"); err != nil {
		t.Fatal(err)
	}
	record := exactLLMEvidenceFixture(
		t, assemblyline.WorkArtifactHandling, "Classify after completion.", "must_exist",
	)
	record.Authority = claim.Authority
	if _, err := repository.ReserveLLMCallEvidence(ctx, record.LLMCallOpeningRecord); err == nil {
		t.Fatal("completed attempt accepted late LLM evidence")
	}
	zero := 0
	command := verificationIntegrationCommand(
		claim.Authority, 1, time.Date(2026, 8, 31, 13, 0, 0, 0, time.UTC), &zero, "",
	)
	if err := repository.AppendVerificationCommandEvidence(ctx, command); err == nil {
		t.Fatal("completed attempt accepted late verification command evidence")
	}
}

func setEvidenceAttemptStatus(
	ctx context.Context,
	pool interface {
		Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	},
	authority model.StepAttemptAuthority,
	status string,
) (pgconn.CommandTag, error) {
	return pool.Exec(ctx, `
		UPDATE job_step_attempts SET status=$6,finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5
	`, authority.JobID, authority.Generation, authority.StepID,
		authority.Attempt, authority.WorkerID, status)
}
