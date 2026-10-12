package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/database"
	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/db"
	"github.com/gryph/omnidex/internal/llm"
)

func nativeCodexEvidenceFixture(t *testing.T, candidate, turn string) exactLLMEvidenceFixtureRecord {
	t.Helper()
	record := exactLLMEvidenceFixture(t, assemblyline.WorkFragmentGeneration, "Return one implementation body.", candidate)
	record.Prepared.Protocol = llm.ExactPreparedProtocolCodexV1
	record.Prepared.Temperature = nil
	record.Prepared.RetainProviderContext = true
	capture := llm.CodexResponseCapture{
		Thread:     json.RawMessage(`{"id":3,"result":{"model":"fixture-model","thread":{"id":"native-thread"}}}`),
		Turn:       json.RawMessage(fmt.Sprintf(`{"id":4,"result":{"turn":{"id":%q}}}`, turn)),
		Message:    json.RawMessage(fmt.Sprintf(`{"method":"item/completed","params":{"threadId":"native-thread","turnId":%q,"item":{"type":"agentMessage","phase":"final_answer","text":%q}}}`, turn, candidate)),
		Usage:      json.RawMessage(fmt.Sprintf(`{"method":"thread/tokenUsage/updated","params":{"threadId":"native-thread","turnId":%q,"tokenUsage":{"last":{"inputTokens":40,"outputTokens":12,"reasoningOutputTokens":5,"totalTokens":52}}}}`, turn)),
		Completion: json.RawMessage(fmt.Sprintf(`{"method":"turn/completed","params":{"threadId":"native-thread","turn":{"id":%q,"status":"completed"}}}`, turn)),
	}
	raw, err := json.Marshal(capture)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := llm.DecodeExactPreparedResponseForProtocol(record.Prepared.Protocol, 0, raw)
	if err != nil {
		t.Fatal(err)
	}
	record.Generation.Protocol = record.Prepared.Protocol
	record.Generation.ProviderHTTPStatus = 0
	record.Generation.ProviderResponseCapture = raw
	record.Generation.ProviderResponseBytes = int64(len(raw))
	record.Generation.ProviderResponseCapturedBytes = len(raw)
	record.Generation.ProviderDoneReason = decoded.DoneReason
	record.Generation.Usage = decoded.Usage
	return record
}

func TestCodexDatabaseRestartPreservesAndBindsCorrectionContext(t *testing.T) {
	databaseURL := evidenceDatabaseURL(t)
	pool, repository := freshEvidenceRepository(t, databaseURL)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "qualify native correction lineage", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "codex-lineage-worker")
	if err != nil || claim == nil {
		t.Fatalf("claim=%v err=%v", claim, err)
	}
	initial := nativeCodexEvidenceFixture(t, "return missing;", "native-parent-turn")
	initial.Authority = claim.Authority
	parent, err := recordExactLLMEvidenceFixture(ctx, repository, initial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{Authority: claim.Authority,
		CallEvidenceID: parent.ID, Candidate: initial.Generation.Content, ValidationError: "undeclared value"}); err != nil {
		t.Fatal(err)
	}
	var schema string
	if err := pool.QueryRow(ctx, `SELECT current_schema()`).Scan(&schema); err != nil {
		t.Fatal(err)
	}
	resumed, err := db.ConnectRuntime(ctx, databaseURL, schema, database.SetupSQL())
	if err != nil {
		t.Fatal(err)
	}
	defer resumed.Close()
	repository = New(resumed, repository.modelAuthority)
	retained, err := repository.GetLLMCallEvidence(ctx, parent.ID)
	if err != nil || retained.Outcome == nil || retained.Outcome.Status != LLMCallRejected {
		t.Fatalf("retained=%+v err=%v", retained, err)
	}
	child := nativeCodexCorrectionFixture(t, initial, retained)
	normalized, err := normalizeLLMCallOpening(child.LLMCallOpeningRecord)
	if err != nil {
		t.Fatal(err)
	}
	var request map[string]json.RawMessage
	if err := json.Unmarshal(normalized.providerRequest, &request); err != nil {
		t.Fatal(err)
	}
	request["continuation"] = json.RawMessage(`{"threadId":"foreign-thread","turnId":"native-parent-turn","model":"fixture-model"}`)
	forged := normalized
	forged.providerRequest, err = json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := insertLLMCallOpening(ctx, resumed, forged); err == nil {
		t.Fatal("SQL admitted foreign native context after restart")
	}
	if _, err := recordExactLLMEvidenceFixture(ctx, repository, child); err != nil {
		t.Fatal(err)
	}
	calls, err := listAllLLMCallEvidenceForJob(ctx, repository, job.ID)
	if err != nil || len(calls) != 2 || calls[1].ParentCallEvidenceID != parent.ID {
		t.Fatalf("retained native lineage: calls=%+v err=%v", calls, err)
	}
	var receipt llmCallGenerationReceipt
	if err := json.Unmarshal(calls[1].GenerationReceipt, &receipt); err != nil || receipt.Usage.ReasoningEvalCount != 5 {
		t.Fatalf("retained native usage: receipt=%+v err=%v", receipt, err)
	}
}

func nativeCodexCorrectionFixture(t *testing.T, initial exactLLMEvidenceFixtureRecord, parent LLMCallEvidence) exactLLMEvidenceFixtureRecord {
	t.Helper()
	defect, err := assemblyline.NewSourceBodyDefect(initial.Generation.Content, 7, 14, "Which expression returns the required value?", fmt.Errorf("undeclared value"))
	if err != nil {
		t.Fatal(err)
	}
	correction, err := defect.Correction(initial.Generation.Content)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := correction.ModelInput()
	if err != nil {
		t.Fatal(err)
	}
	evidence, err := correction.Evidence()
	if err != nil {
		t.Fatal(err)
	}
	child := nativeCodexEvidenceFixture(t, "7", "native-child-turn")
	child.Authority, child.Iteration, child.ParentCallEvidenceID = initial.Authority, 2, parent.ID
	child.WorkInput, child.SourceCorrection, child.Prepared.Prompt = nil, &evidence, prompt
	child.Prepared.RetainedProviderContext, err = llm.DecodeCodexRetainedContext(parent.RawResponse)
	if err != nil {
		t.Fatal(err)
	}
	return child
}
