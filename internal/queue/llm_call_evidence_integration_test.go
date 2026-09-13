package queue

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func TestFreshSchemaLLMCallRecordsResultsWithoutHashReceipts(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	pool, repository := freshEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "exercise exact LLM evidence", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "llm-evidence-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}

	accepted := exactLLMEvidenceFixture(
		t, assemblyline.WorkApplicationClassify, "Classify one exact value.", "A",
	)
	accepted.Authority = claim.Authority
	accepted.Prepared.ContextTokens = llm.MaxInferenceContextTokens
	acceptedEvidence, err := recordExactLLMEvidenceFixture(ctx, repository, accepted)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
		Authority: claim.Authority, CallEvidenceID: acceptedEvidence.ID,
		Candidate: accepted.Generation.Content,
	}); err != nil {
		t.Fatal(err)
	}
	if acceptedEvidence.ContextTokens != llm.MaxInferenceContextTokens {
		t.Fatalf("fresh schema context tokens=%d", acceptedEvidence.ContextTokens)
	}

	rejected := exactLLMEvidenceFixture(
		t, assemblyline.WorkFragmentGeneration, "Write one implementation body.", "invalid body (",
	)
	rejected.Authority = claim.Authority
	rejectedEvidence, err := recordExactLLMEvidenceFixture(ctx, repository, rejected)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
		Authority: claim.Authority, CallEvidenceID: rejectedEvidence.ID,
		Candidate:       rejected.Generation.Content,
		ValidationError: "implementation body parser rejected the exact response",
	}); err != nil {
		t.Fatal(err)
	}

	failed := exactLLMEvidenceFixture(
		t, assemblyline.WorkArtifactHandling, "Classify one artifact.", "source",
	)
	failed.Authority = claim.Authority
	failed.Generation.ProviderResponseDisposition = llm.ProviderResponseBodyReadError
	failed.Generation.ProviderResponseComplete = false
	failed.Generation.ProviderResponseBytesKnown = false
	failed.Generation.ProviderResponseBytes = 0
	failed.Generation.Content = ""
	failed.Generation.ProviderDonePresent = false
	failed.Generation.ProviderDone = false
	failed.Generation.ProviderDoneReason = ""
	failed.Generation.UsagePresent = false
	failed.Generation.Usage = llm.ProviderGenerationUsage{}
	failed.CallError = "provider body read stopped after the captured prefix"
	failedEvidence, err := recordExactLLMEvidenceFixture(ctx, repository, failed)
	if err != nil {
		t.Fatal(err)
	}
	if failedEvidence.Outcome == nil || failedEvidence.Outcome.Status != LLMCallProviderFailed ||
		string(failedEvidence.RawResponse) != string(failed.Generation.ProviderResponseCapture) ||
		failedEvidence.Error != failed.CallError ||
		failedEvidence.Outcome.ValidationError != failed.CallError {
		t.Fatalf("provider failure evidence=%#v", failedEvidence)
	}

	emptyResponse := exactLLMEvidenceFixture(
		t, assemblyline.WorkArtifactHandling, "Classify an empty provider response.", "unused",
	)
	emptyResponse.Authority = claim.Authority
	emptyResponse.Generation.ProviderHTTPStatus = 500
	emptyResponse.Generation.ProviderResponseDisposition = llm.ProviderResponseHTTPError
	emptyResponse.Generation.ProviderResponseCapture = []byte{}
	emptyResponse.Generation.ProviderResponseCapturedBytes = 0
	emptyResponse.Generation.ProviderResponseBytes = 0
	emptyResponse.Generation.Content = ""
	emptyResponse.Generation.ProviderDonePresent = false
	emptyResponse.Generation.ProviderDone = false
	emptyResponse.Generation.ProviderDoneReason = ""
	emptyResponse.Generation.UsagePresent = false
	emptyResponse.Generation.Usage = llm.ProviderGenerationUsage{}
	emptyResponse.CallError = "provider returned an empty HTTP error response"
	emptyEvidence, err := recordExactLLMEvidenceFixture(ctx, repository, emptyResponse)
	if err != nil {
		t.Fatal(err)
	}
	if !emptyEvidence.RawResponsePresent || emptyEvidence.RawResponse == nil ||
		len(emptyEvidence.RawResponse) != 0 || emptyEvidence.Outcome == nil {
		t.Fatalf("present empty provider response was not preserved: %#v", emptyEvidence)
	}

	if _, err := recordExactLLMEvidenceFixture(ctx, repository, accepted); err == nil {
		t.Fatal("duplicate station invocation evidence was accepted")
	}
	mismatched := exactLLMEvidenceFixture(
		t, assemblyline.WorkApplicationClassify, "Classify another value.", "B",
	)
	mismatched.Authority = claim.Authority
	mismatched.Authority.WorkerID = "different-worker"
	if _, err := recordExactLLMEvidenceFixture(ctx, repository, mismatched); err == nil {
		t.Fatal("attempt worker mismatch was accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE llm_call_receipts SET error='tampered' WHERE call_evidence_id=$1`, acceptedEvidence.ID); err == nil {
		t.Fatal("successful response accepted a contradictory error")
	}

	calls, err := listAllLLMCallEvidenceForJob(ctx, repository, job.ID)
	if err != nil || len(calls) != 4 {
		t.Fatalf("calls=%#v err=%v", calls, err)
	}
	for _, call := range calls {
		if !call.ProviderReceiptPresent || call.Outcome == nil {
			t.Fatalf("call %d is not terminal", call.ID)
		}
	}
	firstPage, err := repository.ListLLMCallEvidenceForJob(ctx, job.ID, 0, 1)
	if err != nil || len(firstPage) != 1 {
		t.Fatalf("first station-call page=%#v err=%v", firstPage, err)
	}
	secondPage, err := repository.ListLLMCallEvidenceForJob(ctx, job.ID, firstPage[0].ID, 1)
	if err != nil || len(secondPage) != 1 {
		t.Fatalf("second station-call page=%#v err=%v", secondPage, err)
	}
	completed := exactLLMEvidenceFixture(t, assemblyline.WorkApplicationClassify, "Classify the current value.", "B")
	completed.Authority = claim.Authority
	completedCall, err := recordExactLLMEvidenceFixture(ctx, repository, completed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
		Authority: claim.Authority, CallEvidenceID: completedCall.ID, Candidate: "unreturned value",
	}); err == nil {
		t.Fatal("outcome accepted a candidate different from the provider result")
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
		Authority: claim.Authority, CallEvidenceID: completedCall.ID, Candidate: completed.Generation.Content,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE job_step_attempts SET status='completed',finished_at=clock_timestamp()
		WHERE job_id=$1 AND generation=$2 AND step_id=$3 AND attempt=$4 AND worker_id=$5
	`, claim.Authority.JobID, claim.Authority.Generation, claim.Authority.StepID,
		claim.Authority.Attempt, claim.Authority.WorkerID); err != nil {
		t.Fatalf("earlier provider failure vetoed later code-owned completion: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM llm_call_evidence WHERE job_id=$1`, job.ID); err != nil {
		t.Fatalf("call history could not be discarded: %v", err)
	}
	remaining, err := repository.ListLLMCallEvidenceForJob(ctx, job.ID, 0, 1)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("discarded history remains: %#v, %v", remaining, err)
	}
}

func TestFreshSchemaSourceBodyCorrectionRequiresRejectedSameJobAndModelParent(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	_, repository := freshEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "exercise source-body continuation", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "source-body-continuation-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}

	initial := exactLLMEvidenceFixture(
		t, assemblyline.WorkFragmentGeneration,
		"Write one implementation body.", "return missingValue;",
	)
	initial = exactLLMEvidenceWithNativeContext(initial, "[17,28,39]")
	initial.Authority = claim.Authority
	initialEvidence, err := recordExactLLMEvidenceFixture(ctx, repository, initial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
		Authority: claim.Authority, CallEvidenceID: initialEvidence.ID,
		Candidate:       initial.Generation.Content,
		ValidationError: "implementation body uses undeclared identifier missingValue",
	}); err != nil {
		t.Fatal(err)
	}

	startByte := strings.Index(initial.Generation.Content, "missingValue")
	defect, err := assemblyline.NewSourceBodyDefect(
		initial.Generation.Content,
		startByte,
		startByte+len("missingValue"),
		"Which expression returns the required value without the undeclared symbol?",
		fmt.Errorf("implementation body uses undeclared identifier missingValue"),
	)
	if err != nil {
		t.Fatal(err)
	}
	correctionState, err := defect.Correction(initial.Generation.Content)
	if err != nil {
		t.Fatal(err)
	}
	correctionPrompt, err := correctionState.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	correctionEvidence, err := correctionState.Evidence()
	if err != nil {
		t.Fatal(err)
	}
	correction := exactLLMEvidenceFixture(
		t, assemblyline.WorkFragmentGeneration, correctionPrompt, "7",
	)
	correction.Authority = claim.Authority
	correction.WorkInput = nil
	correction.Iteration = 2
	correction.ParentCallEvidenceID = initialEvidence.ID
	correction.SourceCorrection = &correctionEvidence
	correction.Prepared.RetainedContext = []int{17, 28, 39}
	correctedEvidence, err := recordExactLLMEvidenceFixture(ctx, repository, correction)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
		Authority: claim.Authority, CallEvidenceID: correctedEvidence.ID,
		Candidate: correction.Generation.Content,
	}); err != nil {
		t.Fatal(err)
	}
	if correctedEvidence.Iteration != 2 ||
		correctedEvidence.ParentCallEvidenceID != initialEvidence.ID ||
		correctedEvidence.WorkInput != nil ||
		correctedEvidence.Model != initialEvidence.Model ||
		correctedEvidence.ModelInput != correctionPrompt ||
		correctedEvidence.SourceBaseCandidate != initial.Generation.Content ||
		correctedEvidence.SourceStartByte != startByte ||
		correctedEvidence.SourceEndByte != startByte+len("missingValue") {
		t.Fatalf("correction lineage=%#v initial=%#v", correctedEvidence, initialEvidence)
	}

	unrejected := exactLLMEvidenceFixture(
		t, assemblyline.WorkFragmentGeneration,
		"Write another implementation body.", "return unknown;",
	)
	unrejected = exactLLMEvidenceWithNativeContext(unrejected, "[50,61]")
	unrejected.Authority = claim.Authority
	unrejectedEvidence, err := recordExactLLMEvidenceFixture(ctx, repository, unrejected)
	if err != nil {
		t.Fatal(err)
	}
	illegalStart := strings.Index(unrejected.Generation.Content, "unknown")
	illegalDefect, err := assemblyline.NewSourceBodyDefect(
		unrejected.Generation.Content,
		illegalStart,
		illegalStart+len("unknown"),
		"Which expression should replace this undeclared symbol?",
		fmt.Errorf("implementation body uses undeclared identifier unknown"),
	)
	if err != nil {
		t.Fatal(err)
	}
	illegalState, err := illegalDefect.Correction(unrejected.Generation.Content)
	if err != nil {
		t.Fatal(err)
	}
	illegalPrompt, err := illegalState.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	illegalEvidence, err := illegalState.Evidence()
	if err != nil {
		t.Fatal(err)
	}
	illegal := exactLLMEvidenceFixture(
		t, assemblyline.WorkFragmentGeneration,
		illegalPrompt, "9",
	)
	illegal.Authority = claim.Authority
	illegal.WorkInput = nil
	illegal.Iteration = 2
	illegal.ParentCallEvidenceID = unrejectedEvidence.ID
	illegal.SourceCorrection = &illegalEvidence
	illegal.Prepared.RetainedContext = []int{50, 61}
	if _, err := repository.ReserveLLMCallEvidence(
		ctx, illegal.LLMCallOpeningRecord,
	); err == nil {
		t.Fatal("source-body correction without a rejected parent was accepted")
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
		Authority: claim.Authority, CallEvidenceID: unrejectedEvidence.ID,
		Candidate:       unrejected.Generation.Content,
		ValidationError: "implementation body uses undeclared identifier unknown",
	}); err != nil {
		t.Fatal(err)
	}
	illegal.RequestedModel = "different-model"
	illegal.Prepared.BaseModel = "different-model"
	illegal.Prepared.ContextModel = "different-model"
	if _, err := repository.ReserveLLMCallEvidence(
		ctx, illegal.LLMCallOpeningRecord,
	); err == nil {
		t.Fatal("source-body correction with a different model was accepted")
	}
}
