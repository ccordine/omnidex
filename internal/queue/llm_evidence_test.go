package queue

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLLMCallEvidenceRejectsIncompleteOrFakeSuccess(t *testing.T) {
	t.Parallel()

	base := LLMCallEvidenceRecord{
		StepID: 1, Scope: "portable", RequestedModel: "requested", Model: "effective", Attempt: 1,
		SystemPrompt: "exact system prompt", UserPrompt: "exact user prompt",
		ResponseFormat: "json", ResponseSchema: map[string]any{"type": "object"},
		ContextTokens: 4096, MaxOutputTokens: 512,
		Status: LLMEvidenceSucceeded, Response: "exact response",
	}
	for name, mutate := range map[string]func(*LLMCallEvidenceRecord){
		"missing prompt":   func(record *LLMCallEvidenceRecord) { record.SystemPrompt = "" },
		"fake success":     func(record *LLMCallEvidenceRecord) { record.Response = "" },
		"partial work":     func(record *LLMCallEvidenceRecord) { record.WorkID = strings.Repeat("a", 64) },
		"unbounded call":   func(record *LLMCallEvidenceRecord) { record.ContextTokens = 0 },
		"schema with text": func(record *LLMCallEvidenceRecord) { record.ResponseFormat = "text" },
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
		Response: "\n  exact raw response  \n", ResponseFormat: " JSON ",
	})
	if record.SystemPrompt != "\nexact system prompt\n" || record.UserPrompt != " exact user prompt " {
		t.Fatalf("prompt content was normalized: system=%q user=%q", record.SystemPrompt, record.UserPrompt)
	}
	if record.Response != "\n  exact raw response  \n" {
		t.Fatalf("response content was normalized: %q", record.Response)
	}
	if record.Scope != "portable" || record.RequestedModel != "requested" || record.Model != "effective" || record.ResponseFormat != "json" {
		t.Fatalf("routing metadata was not normalized: %#v", record)
	}
}

func TestLLMCallEvidenceHashIncludesNativeThinkingMode(t *testing.T) {
	base := normalizeLLMCallEvidenceRecord(LLMCallEvidenceRecord{
		StepID: 1, Scope: "portable_advisory_worker", RequestedModel: "requested", Model: "effective", Attempt: 1,
		SystemPrompt: "system", UserPrompt: "user", ResponseFormat: "text",
		ContextTokens: 4096, MaxOutputTokens: 512,
		Status: LLMEvidenceSucceeded, Response: `{"thinking":"trace","content":"memo"}`,
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
		SystemPrompt: "system", UserPrompt: "user", ResponseFormat: "text",
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
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set OMNI_TEST_DATABASE_URL to run PostgreSQL LLM evidence tests")
	}
	t.Setenv("MIGRATIONS_DIR", filepath.Join("..", "..", "migrations"))
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	repository := New(pool)
	if err := repository.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}

	marker := fmt.Sprintf("immutable LLM evidence test %d", time.Now().UnixNano())
	var jobID, stepID int64
	seedTx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer seedTx.Rollback(context.Background())
	if err := seedTx.QueryRow(ctx, `
		INSERT INTO jobs (instruction, pipeline, status, metadata)
		VALUES ($1, 'agent', 'completed', '{}'::jsonb)
		RETURNING id
	`, marker).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := seedTx.Exec(ctx, `
		INSERT INTO job_generations (job_id, generation, purpose) VALUES ($1, 1, 'initial')
	`, jobID); err != nil {
		t.Fatal(err)
	}
	if err := seedTx.QueryRow(ctx, `
		INSERT INTO job_steps (job_id, action, sort_index, status, generation)
		VALUES ($1, 'evidence_contract', 0, 'completed', 1)
		RETURNING id
	`, jobID).Scan(&stepID); err != nil {
		t.Fatal(err)
	}
	if err := seedTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	response := "\n  exact raw provider response  \n"
	created, err := repository.RecordLLMCallEvidence(ctx, LLMCallEvidenceRecord{
		StepID: stepID, Scope: "portable_semantic_worker",
		WorkID: strings.Repeat("b", 64), WorkKind: "application_classification",
		RequestedModel: "requested-model", Model: "effective-model", Attempt: 1,
		SystemPrompt: "\nexact system prompt\n", UserPrompt: " exact user prompt ",
		ResponseFormat: "json", ResponseSchema: map[string]any{"type": "object"},
		ContextTokens: 8192, MaxOutputTokens: 1024, ThinkingEnabled: true,
		Response: response, Status: LLMEvidenceSucceeded, LatencyMS: 17,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Response != response || created.SystemPrompt != "\nexact system prompt\n" || created.UserPrompt != " exact user prompt " {
		t.Fatalf("PostgreSQL changed exact model evidence: %#v", created)
	}
	if created.ResponseSHA256 != llmEvidenceSHA256(response) || len(created.RequestSHA256) != 64 {
		t.Fatalf("hashes request=%q response=%q", created.RequestSHA256, created.ResponseSHA256)
	}
	if !created.ThinkingEnabled {
		t.Fatal("PostgreSQL omitted native thinking state from exact request evidence")
	}
	if created.JobGeneration != 1 || created.ContextProjectionID != "" {
		t.Fatalf("legacy shadow call generation/projection authority=%+v", created)
	}
	if _, err := repository.RecordLLMCallEvidence(ctx, LLMCallEvidenceRecord{
		StepID: stepID, Scope: "portable_fragment_worker",
		RequestedModel: "requested-model", Model: "effective-model", Attempt: 1,
		SystemPrompt: "system", UserPrompt: "user", ResponseFormat: "text",
		ContextTokens: 4096, MaxOutputTokens: 512,
		Response: "partial output", Status: LLMEvidenceGenerationFailed,
		Error: "stream terminated", LatencyMS: 9,
	}); err != nil {
		t.Fatal(err)
	}
	page, err := repository.ReadJobHistoryPage(ctx, jobID, JobHistoryRequest{
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
