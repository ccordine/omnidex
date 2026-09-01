package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func TestNormalizeLLMCallEvidencePreservesUnrelatedStationEnvelopes(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name      string
		kind      assemblyline.WorkKind
		prompt    string
		candidate string
	}{
		{
			name: "semantic classification", kind: assemblyline.WorkApplicationClassify,
			prompt: "Classify one delivery surface exactly.\n", candidate: "A",
		},
		{
			name: "source fragment", kind: assemblyline.WorkFragmentGeneration,
			prompt: "Return one exact declaration.\n", candidate: "func value() int { return 7 }",
		},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			record := exactLLMEvidenceFixture(t, fixture.kind, fixture.prompt, fixture.candidate)
			_, normalized, err := normalizeExactLLMEvidenceFixture(record)
			if err != nil {
				t.Fatal(err)
			}
			if normalized.record.Prepared.Prompt != fixture.prompt ||
				normalized.record.Generation.Content != fixture.candidate {
				t.Fatalf("evidence changed exact bytes: %#v", normalized)
			}
			if string(normalized.rawResponse) != string(record.Generation.ProviderResponseCapture) ||
				!normalized.rawResponsePresent {
				t.Fatalf("raw provider capture was not preserved: %#v", normalized)
			}
			if normalized.status != LLMCallSucceeded ||
				normalized.candidateSHA256 != llmEvidenceSHA256([]byte(fixture.candidate)) {
				t.Fatalf("terminal evidence identity is wrong: %#v", normalized)
			}
		})
	}
}

func TestNormalizeLLMCallEvidencePreservesFailedPartialProviderCapture(t *testing.T) {
	t.Parallel()
	record := exactLLMEvidenceFixture(
		t, assemblyline.WorkApplicationClassify, "Classify one value.", "A",
	)
	record.Generation.ProviderResponseDisposition = llm.ProviderResponseBodyReadError
	record.Generation.ProviderResponseComplete = false
	record.Generation.ProviderResponseBytesKnown = false
	record.Generation.ProviderResponseSHA256 = ""
	record.Generation.ProviderResponseBytes = 0
	record.Generation.Content = ""
	record.Generation.ProviderDonePresent = false
	record.Generation.ProviderDone = false
	record.Generation.ProviderDoneReason = ""
	record.Generation.UsagePresent = false
	record.Generation.Usage = llm.ProviderGenerationUsage{}
	record.CallError = "provider response ended before completion"

	_, normalized, err := normalizeExactLLMEvidenceFixture(record)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.status != LLMCallFailed || !normalized.rawResponsePresent ||
		string(normalized.rawResponse) != string(record.Generation.ProviderResponseCapture) {
		t.Fatalf("failed response evidence=%#v", normalized)
	}
}

func TestNormalizeLLMCallEvidenceClassifiesOnlyValidatedOutputLimit(t *testing.T) {
	t.Parallel()
	record := exactLLMEvidenceFixture(
		t, assemblyline.WorkApplicationClassify, "Classify one value.", "unfinished",
	)
	raw := []byte(fmt.Sprintf(
		`{"created_at":"2026-08-31T12:00:00Z","response":%q,"done":true,"done_reason":"length","total_duration":19,"load_duration":2,"prompt_eval_count":11,"prompt_eval_duration":3,"eval_count":512,"eval_duration":5}`,
		record.Generation.Content,
	))
	decoded, err := llm.DecodeExactPreparedResponseForProtocol(
		record.Prepared.Protocol, 200, raw,
	)
	if err != nil {
		t.Fatal(err)
	}
	responseDigest := sha256.Sum256(raw)
	record.Generation.ProviderResponseDisposition = decoded.Disposition
	record.Generation.ProviderResponseSHA256 = hex.EncodeToString(responseDigest[:])
	record.Generation.ProviderResponseBytes = int64(len(raw))
	record.Generation.ProviderResponseCaptureSHA256 = hex.EncodeToString(responseDigest[:])
	record.Generation.ProviderResponseCapturedBytes = len(raw)
	record.Generation.ProviderResponseCapture = raw
	record.Generation.ProviderDonePresent = decoded.DonePresent
	record.Generation.ProviderDone = decoded.Done
	record.Generation.ProviderDoneReason = decoded.DoneReason
	record.Generation.UsagePresent = decoded.UsagePresent
	record.Generation.Usage = decoded.Usage
	record.OutputLimitReached = true
	record.CallError = (&llm.ExactPreparedOutputLimitReachedError{
		DoneReason: "length", PromptTokens: 11, OutputTokens: 512,
		ContextTokens:   record.Prepared.ContextTokens,
		MaxOutputTokens: record.Prepared.MaxOutputTokens,
		ContentBytes:    len(record.Generation.Content),
	}).Error()

	_, normalized, err := normalizeExactLLMEvidenceFixture(record)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.status != LLMCallFailed || !normalized.record.OutputLimitReached ||
		normalized.record.Generation.Content != "unfinished" {
		t.Fatalf("output-limit evidence=%#v", normalized)
	}

	missingClassification := record
	missingClassification.OutputLimitReached = false
	if _, _, err := normalizeExactLLMEvidenceFixture(missingClassification); err == nil {
		t.Fatal("validated length completion was accepted without its exact classification")
	}
	forgedClassification := exactLLMEvidenceFixture(
		t, assemblyline.WorkApplicationClassify, "Classify one value.", "A",
	)
	forgedClassification.OutputLimitReached = true
	forgedClassification.CallError = "forged output limit"
	if _, _, err := normalizeExactLLMEvidenceFixture(forgedClassification); err == nil {
		t.Fatal("ordinary stop completion was accepted as an output-limit result")
	}
}

func TestCompactLLMCallGenerationReceiptDoesNotDuplicateLargeModelContent(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("\"", llm.MaxExactPreparedProviderResponseBytes)
	receipt, err := encodeLLMCallGenerationReceipt(llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, Content: content,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(receipt) > maxLLMCallGenerationReceiptBytes ||
		strings.Contains(string(receipt), strings.Repeat("\\\"", 64)) {
		t.Fatalf("compact receipt duplicated escaped model content: bytes=%d", len(receipt))
	}
	var decoded llmCallGenerationReceipt
	if err := json.Unmarshal(receipt, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ContentBytes != len(content) ||
		decoded.ContentSHA256 != llmEvidenceSHA256([]byte(content)) {
		t.Fatalf("compact content identity=%#v", decoded)
	}
}

func TestNormalizeLLMCallEvidenceRejectsUnboundOrInexactRecords(t *testing.T) {
	t.Parallel()
	base := exactLLMEvidenceFixture(
		t, assemblyline.WorkApplicationClassify, "Classify one value.", "A",
	)
	for name, mutate := range map[string]func(*exactLLMEvidenceFixtureRecord){
		"wrong scope":    func(record *exactLLMEvidenceFixtureRecord) { record.Scope = assemblyline.PortableFragmentWorkerScope },
		"forged work":    func(record *exactLLMEvidenceFixtureRecord) { record.WorkID = "forged" },
		"model mismatch": func(record *exactLLMEvidenceFixtureRecord) { record.RequestedModel = "other" },
		"zero iteration": func(record *exactLLMEvidenceFixtureRecord) { record.Iteration = 0 },
		"parent on initial": func(record *exactLLMEvidenceFixtureRecord) {
			record.ParentCallEvidenceID = 7
		},
		"second output continuation": func(record *exactLLMEvidenceFixtureRecord) {
			record.OutputContinuation = 2
			record.ParentCallEvidenceID = 7
		},
		"semantic correction": func(record *exactLLMEvidenceFixtureRecord) {
			record.Iteration = 2
			record.ParentCallEvidenceID = 7
		},
		"fake success": func(record *exactLLMEvidenceFixtureRecord) { record.Generation.Content = "" },
		"unbounded error": func(record *exactLLMEvidenceFixtureRecord) {
			record.CallError = strings.Repeat("x", 8193)
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			record := base
			mutate(&record)
			if _, _, err := normalizeExactLLMEvidenceFixture(record); err == nil {
				t.Fatalf("accepted invalid evidence: %#v", record)
			}
		})
	}
}

func TestNormalizeLLMCallOpeningAcceptsOneSameIterationOutputContinuation(t *testing.T) {
	t.Parallel()
	record := exactLLMEvidenceFixture(
		t, assemblyline.WorkApplicationClassify, "Classify one value.", "A",
	)
	opening := record.LLMCallOpeningRecord
	opening.OutputContinuation = 1
	opening.ParentCallEvidenceID = 7
	if _, err := normalizeLLMCallOpening(opening); err != nil {
		t.Fatal(err)
	}
}

func TestEncodeLLMCallProjectionStoresOnlyExactSpanIdentity(t *testing.T) {
	t.Parallel()
	candidate := strings.Repeat("declaration-byte-", 4096)
	projection, err := assemblyline.NewExactPortableResultProjection(candidate)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := encodeLLMCallProjection(candidate, &projection)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) >= 8192 || strings.Contains(string(raw), candidate[:128]) {
		t.Fatalf("accepted projection duplicated source bytes: encoded=%d", len(raw))
	}
	var evidence LLMCallProjectionEvidence
	if err := json.Unmarshal(raw, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.SourceResponseSHA256 != projection.SourceResponseSHA256 ||
		evidence.SourceSHA256 != projection.SourceSHA256 ||
		evidence.StartByte != 0 || evidence.EndByte != len(candidate) ||
		evidence.RawBytes != len(candidate) {
		t.Fatalf("projection identity=%#v", evidence)
	}
}

func exactLLMEvidenceFixture(
	t *testing.T,
	kind assemblyline.WorkKind,
	prompt, candidate string,
) exactLLMEvidenceFixtureRecord {
	t.Helper()
	temperature := llm.ExactPreparedTemperature(0)
	prepared := llm.PreparedModel{
		Protocol:  llm.ExactPreparedProtocolPlainCompletionV4,
		BaseModel: "fixture-model", ContextModel: "fixture-model",
		Prompt:          prompt,
		MaxOutputTokens: 512, OutputLimitMode: llm.ExactPreparedOutputLimitExplicit,
		ContextTokens: 8192, Temperature: &temperature,
	}
	raw := []byte(fmt.Sprintf(
		`{"created_at":"2026-08-31T12:00:00Z","response":%q,"done":true,"done_reason":"stop","total_duration":19,"load_duration":2,"prompt_eval_count":11,"prompt_eval_duration":3,"eval_count":7,"eval_duration":5}`,
		candidate,
	))
	decoded, err := llm.DecodeExactPreparedResponseForProtocol(prepared.Protocol, 200, raw)
	if err != nil {
		t.Fatal(err)
	}
	request, err := llm.ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	requestDigest := sha256.Sum256(request)
	responseDigest := sha256.Sum256(raw)
	return exactLLMEvidenceFixtureRecord{
		LLMCallOpeningRecord: LLMCallOpeningRecord{Authority: model.StepAttemptAuthority{
			JobID: 1, Generation: 1, StepID: 1, Attempt: 1, WorkerID: "fixture-worker",
		},
			Scope: mustLLMEvidenceScope(t, kind), WorkID: strings.Repeat("a", 64),
			WorkKind: kind, Iteration: 1,
			RequestedModel: prepared.BaseModel, Prepared: prepared,
		},
		Generation: llm.PreparedGeneration{
			Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
			ProviderRequestDisposition:  llm.ProviderRequestDispatched,
			Content:                     candidate,
			ProviderRequestSHA256:       hex.EncodeToString(requestDigest[:]),
			ProviderHTTPStatus:          200,
			ProviderResponseDisposition: decoded.Disposition,
			ProviderResponseComplete:    true, ProviderResponseBytesKnown: true,
			ProviderContentEncoding:       llm.NewProviderContentEncodingEvidence(nil, false),
			ProviderResponseSHA256:        hex.EncodeToString(responseDigest[:]),
			ProviderResponseBytes:         int64(len(raw)),
			ProviderResponseCaptureSHA256: hex.EncodeToString(responseDigest[:]),
			ProviderResponseCapturedBytes: len(raw), ProviderResponseCapture: raw,
			ProviderDonePresent: decoded.DonePresent, ProviderDone: decoded.Done,
			ProviderDoneReason: decoded.DoneReason,
			UsagePresent:       decoded.UsagePresent, Usage: decoded.Usage,
		},
		Elapsed: 23 * time.Millisecond,
	}
}

type exactLLMEvidenceFixtureRecord struct {
	LLMCallOpeningRecord
	Generation         llm.PreparedGeneration
	OutputLimitReached bool
	CallError          string
	Elapsed            time.Duration
}

func (record exactLLMEvidenceFixtureRecord) receipt(callID int64) LLMCallReceiptRecord {
	return LLMCallReceiptRecord{
		Authority: record.Authority, CallEvidenceID: callID,
		Prepared: record.Prepared, Generation: record.Generation,
		OutputLimitReached: record.OutputLimitReached,
		CallError:          record.CallError,
		Elapsed:            record.Elapsed,
	}
}

func normalizeExactLLMEvidenceFixture(
	record exactLLMEvidenceFixtureRecord,
) (normalizedLLMCallOpening, normalizedLLMCallReceipt, error) {
	opening, err := normalizeLLMCallOpening(record.LLMCallOpeningRecord)
	if err != nil {
		return normalizedLLMCallOpening{}, normalizedLLMCallReceipt{}, err
	}
	receipt, err := normalizeLLMCallReceipt(record.receipt(1))
	return opening, receipt, err
}

func mustLLMEvidenceScope(t *testing.T, kind assemblyline.WorkKind) string {
	t.Helper()
	scope, err := assemblyline.PortableWorkerScopeForWorkKind(kind)
	if err != nil {
		t.Fatal(err)
	}
	return scope
}
