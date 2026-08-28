package queue

import (
	"context"
	"fmt"
	"os"
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
		ResponseFormat: "text",
		ContextTokens:  4096, MaxOutputTokens: 512,
		Status: LLMEvidenceSucceeded, Response: "exact response",
	}
	for name, mutate := range map[string]func(*LLMCallEvidenceRecord){
		"missing prompt": func(record *LLMCallEvidenceRecord) { record.SystemPrompt = "" },
		"fake success":   func(record *LLMCallEvidenceRecord) { record.Response = "" },
		"partial work":   func(record *LLMCallEvidenceRecord) { record.WorkID = strings.Repeat("a", 64) },
		"unbounded call": func(record *LLMCallEvidenceRecord) { record.ContextTokens = 0 },
		"JSON format":    func(record *LLMCallEvidenceRecord) { record.ResponseFormat = "json" },
		"empty format":   func(record *LLMCallEvidenceRecord) { record.ResponseFormat = "" },
		"normalized format": func(record *LLMCallEvidenceRecord) {
			record.ResponseFormat = " TEXT "
		},
		"response schema": func(record *LLMCallEvidenceRecord) {
			record.ResponseSchema = map[string]any{"type": "object"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			record := base
			mutate(&record)
			if _, err := (&Repository{}).RecordLLMCallEvidence(context.Background(), record); err == nil {
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
		Response: "\n  exact raw response  \n", ResponseFormat: " TEXT ",
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
	if record.ResponseFormat != " TEXT " {
		t.Fatalf("response format authority was normalized or defaulted: %q", record.ResponseFormat)
	}
}

func TestLLMCallEvidenceRejectsStructuredResponseAuthority(t *testing.T) {
	base := LLMCallEvidenceRecord{
		StepID: 1, Scope: "portable_semantic_worker",
		RequestedModel: "requested", Model: "effective", Attempt: 1,
		StationCallOpeningID: 1,
		SystemPrompt:         "system", UserPrompt: "user", ResponseFormat: "text",
		ContextTokens: 4096, MaxOutputTokens: 4096,
		Status: LLMEvidenceSucceeded, Response: "exact raw leaf",
	}
	if err := validateLLMCallEvidenceRecord(base); err != nil {
		t.Fatal(err)
	}
	withFormat := base
	withFormat.ResponseFormat = "json"
	if err := validateLLMCallEvidenceRecord(withFormat); err == nil {
		t.Fatal("LLM evidence accepted JSON response authority")
	}
	withSchema := base
	withSchema.ResponseSchema = map[string]any{"type": "object"}
	if err := validateLLMCallEvidenceRecord(withSchema); err == nil {
		t.Fatal("LLM evidence accepted a response schema")
	}
	if schema, _, err := validateAndHashLLMCallEvidenceRecord(base); err != nil {
		t.Fatal(err)
	} else if schema != nil {
		t.Fatalf("raw LLM evidence persisted response schema %#v", schema)
	}
}

func TestLLMCallEvidenceHashIncludesNativeThinkingMode(t *testing.T) {
	base := normalizeLLMCallEvidenceRecord(LLMCallEvidenceRecord{
		StepID: 1, Scope: "portable_advisory_worker", RequestedModel: "requested", Model: "effective", Attempt: 1,
		StationCallOpeningID: 1,
		SystemPrompt:         "system", UserPrompt: "user", ResponseFormat: "text",
		ContextTokens: 4096, MaxOutputTokens: 512,
		Status: LLMEvidenceSucceeded, Response: "exact memo",
	})
	_, directHash, err := validateAndHashLLMCallEvidenceRecord(base)
	if err != nil {
		t.Fatal(err)
	}
	base.ThinkingEnabled = true
	_, advisoryHash, err := validateAndHashLLMCallEvidenceRecord(base)
	if err != nil {
		t.Fatal(err)
	}
	if directHash == advisoryHash {
		t.Fatal("thinking-enabled request shared the direct request identity")
	}
}

func TestLLMCallEvidenceAllowsPartialOutputOnlyForGenerationFailure(t *testing.T) {
	t.Parallel()

	base := LLMCallEvidenceRecord{
		StepID: 1, Scope: "portable", RequestedModel: "requested", Model: "effective", Attempt: 1,
		StationCallOpeningID: 1,
		SystemPrompt:         "system", UserPrompt: "user", ResponseFormat: "text",
		ContextTokens: 4096, MaxOutputTokens: 512, Error: "stream ended", Response: "partial",
	}
	base.Status = LLMEvidenceGenerationFailed
	if err := validateLLMCallEvidenceRecord(base); err != nil {
		t.Fatalf("generation failure lost partial evidence: %v", err)
	}
	base.Status = LLMEvidencePreparationFailed
	if err := validateLLMCallEvidenceRecord(base); err == nil {
		t.Fatal("preparation failure accepted a response that could not have been generated")
	}
}

func TestLLMEvidenceMigrationIsImmutableAndExact(t *testing.T) {
	t.Parallel()

	raw, err := os.ReadFile("../../migrations/024_llm_evidence.sql")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	for _, required := range []string{
		"CREATE TABLE IF NOT EXISTS llm_call_evidence",
		"system_prompt", "user_prompt", "request_sha256", "response_sha256", "response_schema",
		"requested_model", "context_tokens", "max_output_tokens",
		"prevent_llm_call_evidence_mutation", "BEFORE UPDATE OR DELETE",
	} {
		if !strings.Contains(source, required) {
			t.Fatalf("LLM evidence migration omitted %q", required)
		}
	}
}

func TestLLMAdvisoryEvidenceMigrationAddsThinkingRequestIdentity(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/025_llm_advisory_evidence.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"thinking_enabled", "BOOLEAN", "NOT NULL"} {
		if !strings.Contains(string(raw), required) {
			t.Fatalf("advisory evidence migration omitted %q", required)
		}
	}
}

func TestPostgresLLMCallEvidenceRoundTripIsExactAndImmutable(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repository := New(pool)
	if err := repository.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
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
	first.Record.ThinkingEnabled = true
	created := persistPreparedStationEvidenceFixture(t, repository, first, "")
	if created.Response != response || created.SystemPrompt != first.Record.SystemPrompt ||
		created.UserPrompt != first.Record.UserPrompt {
		t.Fatalf("PostgreSQL changed exact model evidence: %#v", created)
	}
	if created.ResponseSHA256 != llmEvidenceSHA256(response) || len(created.RequestSHA256) != 64 {
		t.Fatalf("hashes request=%q response=%q", created.RequestSHA256, created.ResponseSHA256)
	}
	if !created.ThinkingEnabled {
		t.Fatal("PostgreSQL omitted native thinking state from exact request evidence")
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
