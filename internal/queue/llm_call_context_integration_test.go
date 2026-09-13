package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func TestFreshSchemaSourceContextMustMatchParentEvenWhenRequestNormalizationIsBypassed(t *testing.T) {
	pool, repository := freshEvidenceRepository(t, evidenceDatabaseURL(t))
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "exercise retained context binding", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "context-binding-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	initial := exactLLMEvidenceFixture(t, assemblyline.WorkFragmentGeneration, "Return one implementation body.", "return missing;")
	initial.Authority = claim.Authority
	initial = exactLLMEvidenceWithNativeContext(initial, "[11,22,33]")
	parent, err := recordExactLLMEvidenceFixture(ctx, repository, initial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
		Authority: claim.Authority, CallEvidenceID: parent.ID,
		Candidate: initial.Generation.Content, ValidationError: "undeclared local value",
	}); err != nil {
		t.Fatal(err)
	}
	defect, err := assemblyline.NewSourceBodyDefect(initial.Generation.Content, len("return "), len("return missing"),
		"Which expression returns the required value?", fmt.Errorf("undeclared local value"))
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
	child := exactLLMEvidenceFixture(t, assemblyline.WorkFragmentGeneration, prompt, "7")
	child.Authority = claim.Authority
	child.Iteration = 2
	child.WorkInput = nil
	child.ParentCallEvidenceID = parent.ID
	child.SourceCorrection = &evidence
	child.Prepared.RetainedContext = []int{11, 22, 33}
	normalized, err := normalizeLLMCallOpening(child.LLMCallOpeningRecord)
	if err != nil {
		t.Fatal(err)
	}
	for name, nativeContext := range map[string]string{
		"missing": "", "null": "null", "empty": "[]", "object": "{}",
		"different parent": "[70,80]", "reordered": "[33,22,11]", "changed token": "[11,22,34]",
	} {
		t.Run(name, func(t *testing.T) {
			var request map[string]json.RawMessage
			if err := json.Unmarshal(normalized.providerRequest, &request); err != nil {
				t.Fatal(err)
			}
			if nativeContext == "" {
				delete(request, "context")
			} else {
				request["context"] = json.RawMessage(nativeContext)
			}
			forged := normalized
			forged.providerRequest, err = json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := insertLLMCallOpening(ctx, pool, forged); err == nil || !strings.Contains(err.Error(), "model context") {
				t.Fatalf("database admitted altered parent context: %v", err)
			}
		})
	}
	if _, err := repository.ReserveLLMCallEvidence(ctx, child.LLMCallOpeningRecord); err != nil {
		t.Fatalf("exact retained context was rejected: %v", err)
	}
}

func TestFreshSchemaRetainedContextRequestMayExceedFormerRequestByteLimit(t *testing.T) {
	_, repository := freshEvidenceRepository(t, evidenceDatabaseURL(t))
	ctx := context.Background()
	job, err := repository.EnqueueCodingJob(ctx, "exercise native context request capacity", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "context-capacity-worker")
	if err != nil || claim == nil || claim.Job.ID != job.ID {
		t.Fatalf("claim=%#v err=%v", claim, err)
	}
	initial := exactLLMEvidenceFixture(t, assemblyline.WorkFragmentGeneration, "Return one implementation body.", "return missing;")
	initial.Authority = claim.Authority
	initial.Prepared.ContextTokens = llm.MaxInferenceContextTokens
	retained := make([]int, 160_000)
	for index := range retained {
		retained[index] = 200_000
	}
	encoded, err := json.Marshal(retained)
	if err != nil {
		t.Fatal(err)
	}
	initial.Generation.ProviderResponseCapture = []byte(strings.Replace(
		string(initial.Generation.ProviderResponseCapture), `"prompt_eval_count":11`, `"prompt_eval_count":159993`, 1))
	initial.Generation.Usage.PromptEvalCount = 159993
	initial = exactLLMEvidenceWithNativeContext(initial, string(encoded))
	parent, err := recordExactLLMEvidenceFixture(ctx, repository, initial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.RecordLLMCallOutcome(ctx, LLMCallOutcomeRecord{
		Authority: claim.Authority, CallEvidenceID: parent.ID,
		Candidate: initial.Generation.Content, ValidationError: "undeclared local value",
	}); err != nil {
		t.Fatal(err)
	}
	question := "Which expression returns the required value?"
	child := exactLLMEvidenceFixture(t, assemblyline.WorkFragmentGeneration, question+"\n\nmissing", "7")
	child.Authority = claim.Authority
	child.Iteration, child.ParentCallEvidenceID = 2, parent.ID
	child.WorkInput = nil
	child.SourceCorrection = &assemblyline.SourceBodyCorrectionEvidence{
		BaseCandidate: initial.Generation.Content, StartByte: len("return "), EndByte: len("return missing"), Question: question}
	child.Prepared.ContextTokens = initial.Prepared.ContextTokens
	child.Prepared.RetainedContext = retained
	call, err := repository.ReserveLLMCallEvidence(ctx, child.LLMCallOpeningRecord)
	if err != nil {
		t.Fatal(err)
	}
	if call.ProviderRequestBytes <= 1024*1024 || call.ProviderRequestBytes > llm.MaxExactPreparedProviderRequestBytes {
		t.Fatalf("persisted request bytes=%d", call.ProviderRequestBytes)
	}
}
