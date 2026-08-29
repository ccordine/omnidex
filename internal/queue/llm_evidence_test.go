package queue

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func TestLLMCallEvidenceRejectsIncompleteOrFakeSuccess(t *testing.T) {
	t.Parallel()

	base := LLMCallEvidenceRecord{
		StepID: 1, Scope: "portable", RequestedModel: "requested", Model: "effective", Attempt: 1,
		StationCallOpeningID: 1,
		SystemPrompt:         "exact system prompt", UserPrompt: "exact user prompt",
		ContextTokens: 4096, MaxOutputTokens: 512,
		Status: LLMEvidenceSucceeded, Response: "exact response",
	}
	for name, mutate := range map[string]func(*LLMCallEvidenceRecord){
		"missing prompt": func(record *LLMCallEvidenceRecord) { record.SystemPrompt = "" },
		"fake success":   func(record *LLMCallEvidenceRecord) { record.Response = "" },
		"partial work":   func(record *LLMCallEvidenceRecord) { record.WorkID = strings.Repeat("a", 64) },
		"unbounded call": func(record *LLMCallEvidenceRecord) { record.ContextTokens = 0 },
	} {
		t.Run(name, func(t *testing.T) {
			record := base
			mutate(&record)
			if err := validateLLMCallEvidenceRecord(record); err == nil {
				t.Fatalf("accepted invalid evidence %#v", record)
			}
		})
	}
}

func TestLLMCallEvidencePreservesExactPromptsAndRawResponse(t *testing.T) {
	t.Parallel()

	record := normalizeLLMCallEvidenceRecord(LLMCallEvidenceRecord{
		Scope: " portable ", RequestedModel: " requested ", Model: " effective ",
		SystemPrompt: "\nexact system prompt\n", UserPrompt: " exact user prompt ",
		Response: "\n  exact raw response  \n",
	})
	if record.SystemPrompt != "\nexact system prompt\n" || record.UserPrompt != " exact user prompt " {
		t.Fatalf("prompt content was normalized: system=%q user=%q", record.SystemPrompt, record.UserPrompt)
	}
	if record.Response != "\n  exact raw response  \n" {
		t.Fatalf("response content was normalized: %q", record.Response)
	}
	if record.Scope != "portable" || record.RequestedModel != "requested" || record.Model != "effective" {
		t.Fatalf("routing metadata was not normalized: %#v", record)
	}
}

func TestLLMCallEvidenceAcceptsCurrentTransportRecord(t *testing.T) {
	base := LLMCallEvidenceRecord{
		StepID: 1, Scope: "portable_semantic_worker",
		RequestedModel: "requested", Model: "effective", Attempt: 1,
		StationCallOpeningID: 1,
		SystemPrompt:         "system", UserPrompt: "user",
		ContextTokens: 4096, MaxOutputTokens: 4096,
		Status: LLMEvidenceSucceeded, Response: "exact raw leaf",
	}
	if err := validateLLMCallEvidenceRecord(base); err != nil {
		t.Fatal(err)
	}
	unsupported := base
	unsupported.Status = LLMCallEvidenceStatus("retired_status")
	if err := validateLLMCallEvidenceRecord(unsupported); err == nil {
		t.Fatal("LLM evidence accepted a retired transport status")
	}
}

func TestLLMCallEvidenceAllowsPartialOutputOnlyForGenerationFailure(t *testing.T) {
	t.Parallel()

	base := LLMCallEvidenceRecord{
		StepID: 1, Scope: "portable", RequestedModel: "requested", Model: "effective", Attempt: 1,
		StationCallOpeningID: 1,
		SystemPrompt:         "system", UserPrompt: "user",
		ContextTokens: 4096, MaxOutputTokens: 512, Error: "stream ended", Response: "partial",
	}
	base.Status = LLMEvidenceGenerationFailed
	if err := validateLLMCallEvidenceRecord(base); err != nil {
		t.Fatalf("generation failure lost partial evidence: %v", err)
	}
	base.Status = LLMCallEvidenceStatus("unsupported")
	if err := validateLLMCallEvidenceRecord(base); err == nil {
		t.Fatal("unsupported failure status accepted partial evidence")
	}
}

func TestPostgresLLMCallEvidenceRoundTripIsExactAndImmutable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := openIsolatedDatabasePool(t)
	repository := New(pool)
	if err := repository.ResetDatabase(ctx, loadCurrentDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}

	marker := fmt.Sprintf("immutable LLM evidence test %d", time.Now().UnixNano())
	job, err := repository.EnqueueJob(ctx, marker, model.PipelineCoding, []byte(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "llm-evidence-worker")
	if err != nil {
		t.Fatal(err)
	}
	if claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%+v want job %d", claim, job.ID)
	}
	authority, stepID := claim.Authority, claim.Step.ID

	response := "\n  exact response  \n"
	first := prepareSuccessfulStationEvidenceFixture(
		t, repository, authority,
		newStationEvidenceJobForTest(t, marker+"-success"), response,
	)
	created := persistPreparedStationEvidenceFixture(t, repository, first, "")
	if created.Response != response || created.SystemPrompt != first.Record.SystemPrompt ||
		created.UserPrompt != first.Record.UserPrompt {
		t.Fatalf("PostgreSQL changed exact model evidence: %#v", created)
	}
	if created.ResponseSHA256 != llmEvidenceSHA256(response) ||
		created.WireRequestSHA256 != first.Call.WireRequestSHA256 {
		t.Fatalf("wire request hash=%q want %q; response hash=%q",
			created.WireRequestSHA256, first.Call.WireRequestSHA256, created.ResponseSHA256)
	}
	if created.JobGeneration != 1 || created.StepAttempt != 1 ||
		created.WorkerID != authority.WorkerID || created.ContextProjectionID != "" {
		t.Fatalf("exact call generation/projection authority=%+v", created)
	}
	failed := prepareFailedStationEvidenceFixture(
		t, repository, authority,
		newStationEvidenceJobForTest(t, marker+"-failure"),
		"partial output", "stream terminated",
	)
	persistPreparedStationEvidenceFixture(t, repository, failed, "")
	completeStepAttemptForTest(t, ctx, pool, authority)
	page, err := repository.ReadJobHistoryPage(ctx, job.ID, JobHistoryRequest{
		Stream: JobHistoryLLMCalls, Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.LLMCalls) != 2 || page.LLMCalls[0].Call.ID != created.ID ||
		page.LLMCalls[0].Call.WireRequestSHA256 != first.Call.WireRequestSHA256 ||
		page.LLMCalls[1].Call.WireRequestSHA256 != failed.Call.WireRequestSHA256 ||
		page.LLMCalls[1].Call.Response != "partial output" {
		t.Fatalf("evidence page=%#v", page.LLMCalls)
	}
	for _, item := range page.LLMCalls {
		if item.Step.StepID != stepID || item.Step.Generation != 1 || item.Step.SupersededAtGeneration != nil {
			t.Fatalf("LLM evidence step authority=%+v", item.Step)
		}
	}
	if _, err := pool.Exec(ctx, `UPDATE llm_call_evidence SET system_prompt='tampered' WHERE id=$1`, created.ID); err == nil {
		t.Fatal("database allowed exact model evidence to be edited")
	}
	if _, err := pool.Exec(ctx, `DELETE FROM llm_call_evidence WHERE id=$1`, created.ID); err == nil {
		t.Fatal("database allowed exact model evidence to be deleted")
	}
}
