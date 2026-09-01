package queue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func TestFreshSchemaLLMCallEvidenceIsExactTerminalAndImmutable(t *testing.T) {
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
	accepted.WorkID = strings.Repeat("a", 64)
	accepted.Prepared.ContextTokens = llm.MaxInferenceContextTokens
	accepted.Generation.ProviderRequestSHA256, err = llm.ExactPreparedRequestSHA256(
		accepted.Prepared,
	)
	if err != nil {
		t.Fatal(err)
	}
	acceptedEvidence, err := recordExactLLMEvidenceFixture(ctx, repository, accepted)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := assemblyline.NewExactPortableResultProjection(accepted.Generation.Content)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
		Authority: claim.Authority, CallEvidenceID: acceptedEvidence.ID,
		Candidate: accepted.Generation.Content, Projection: &projection,
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
	rejected.WorkID = strings.Repeat("b", 64)
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
	failed.WorkID = strings.Repeat("c", 64)
	failed.Generation.ProviderResponseDisposition = llm.ProviderResponseBodyReadError
	failed.Generation.ProviderResponseComplete = false
	failed.Generation.ProviderResponseBytesKnown = false
	failed.Generation.ProviderResponseSHA256 = ""
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
		failedEvidence.ErrorSHA256 != llmEvidenceSHA256([]byte(failed.CallError)) ||
		failedEvidence.Outcome.ValidationErrorSHA256 != failedEvidence.ErrorSHA256 {
		t.Fatalf("provider failure evidence=%#v", failedEvidence)
	}

	emptyResponse := exactLLMEvidenceFixture(
		t, assemblyline.WorkArtifactHandling, "Classify an empty provider response.", "unused",
	)
	emptyResponse.Authority = claim.Authority
	emptyResponse.WorkID = strings.Repeat("d", 64)
	emptyDigest := sha256.Sum256(nil)
	emptyResponse.Generation.ProviderHTTPStatus = 500
	emptyResponse.Generation.ProviderResponseDisposition = llm.ProviderResponseHTTPError
	emptyResponse.Generation.ProviderResponseCapture = []byte{}
	emptyResponse.Generation.ProviderResponseCapturedBytes = 0
	emptyResponse.Generation.ProviderResponseCaptureSHA256 = hex.EncodeToString(emptyDigest[:])
	emptyResponse.Generation.ProviderResponseBytes = 0
	emptyResponse.Generation.ProviderResponseSHA256 = hex.EncodeToString(emptyDigest[:])
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
	mismatched.WorkID = strings.Repeat("e", 64)
	if _, err := recordExactLLMEvidenceFixture(ctx, repository, mismatched); err == nil {
		t.Fatal("attempt worker mismatch was accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE llm_call_evidence SET system_envelope='tampered' WHERE id=$1`, acceptedEvidence.ID); err == nil {
		t.Fatal("LLM call opening update was accepted")
	}
	if _, err := pool.Exec(ctx, `UPDATE llm_call_receipts SET error='tampered' WHERE call_evidence_id=$1`, acceptedEvidence.ID); err == nil {
		t.Fatal("LLM call receipt update was accepted")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM llm_call_receipts WHERE call_evidence_id=$1`, acceptedEvidence.ID); err == nil {
		t.Fatal("LLM call receipt delete was accepted")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM llm_call_outcomes WHERE call_evidence_id=$1`, acceptedEvidence.ID); err == nil {
		t.Fatal("LLM call outcome delete was accepted")
	}

	calls, err := listAllLLMCallEvidenceForJob(ctx, repository, job.ID)
	if err != nil || len(calls) != 4 {
		t.Fatalf("calls=%#v err=%v", calls, err)
	}
	for _, call := range calls {
		if !call.ProviderReceiptPresent || call.Outcome == nil {
			t.Fatalf("call %d is not terminal", call.ID)
		}
		if call.Outcome.ValidationError != "" &&
			call.Outcome.ValidationErrorSHA256 != llmEvidenceSHA256([]byte(call.Outcome.ValidationError)) {
			t.Fatalf("call %d terminal error hash is not exact: %#v", call.ID, call.Outcome)
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
	initial.Authority = claim.Authority
	initial.WorkID = strings.Repeat("f", 64)
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
	correction.WorkID = initial.WorkID
	correction.Iteration = 2
	correction.ParentCallEvidenceID = initialEvidence.ID
	correction.SourceCorrection = &correctionEvidence
	correctedEvidence, err := recordExactLLMEvidenceFixture(ctx, repository, correction)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := assemblyline.NewExactPortableResultProjection(
		correction.Generation.Content,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
		Authority: claim.Authority, CallEvidenceID: correctedEvidence.ID,
		Candidate: correction.Generation.Content, Projection: &projection,
	}); err != nil {
		t.Fatal(err)
	}
	if correctedEvidence.Iteration != 2 ||
		correctedEvidence.ParentCallEvidenceID != initialEvidence.ID ||
		correctedEvidence.WorkID != initialEvidence.WorkID ||
		correctedEvidence.Model != initialEvidence.Model ||
		correctedEvidence.SystemEnvelope != correctionPrompt ||
		correctedEvidence.SourceBaseCandidate != initial.Generation.Content ||
		correctedEvidence.SourceStartByte != startByte ||
		correctedEvidence.SourceEndByte != startByte+len("missingValue") {
		t.Fatalf("correction lineage=%#v initial=%#v", correctedEvidence, initialEvidence)
	}

	unrejected := exactLLMEvidenceFixture(
		t, assemblyline.WorkFragmentGeneration,
		"Write another implementation body.", "return unknown;",
	)
	unrejected.Authority = claim.Authority
	unrejected.WorkID = strings.Repeat("e", 64)
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
	illegal.WorkID = unrejected.WorkID
	illegal.Iteration = 2
	illegal.ParentCallEvidenceID = unrejectedEvidence.ID
	illegal.SourceCorrection = &illegalEvidence
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
