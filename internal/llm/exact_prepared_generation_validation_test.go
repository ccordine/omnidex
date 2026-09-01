package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
)

func TestExactPreparedGenerationAcceptsOnlyCompleteStopWithinBudget(t *testing.T) {
	t.Parallel()
	prepared := exactPreparedRequestFixture()

	accepted := exactPreparedGenerationFixture(t, prepared, "stop", 40, 12)
	if err := ValidateExactPreparedGenerationForRequest(prepared, accepted); err != nil {
		t.Fatalf("complete bounded response was rejected: %v", err)
	}

	overBudget := exactPreparedGenerationFixture(
		t, prepared, "stop", 40, prepared.MaxOutputTokens+1,
	)
	if err := ValidateExactPreparedGenerationForRequest(prepared, overBudget); err == nil {
		t.Fatal("response exceeding provider output authority was accepted")
	}
}

func TestExactPreparedGenerationRejectsLengthAsIncompleteEvidence(t *testing.T) {
	t.Parallel()
	prepared := exactPreparedRequestFixture()
	generation := exactPreparedGenerationFixture(
		t, prepared, "length", 40, prepared.MaxOutputTokens,
	)
	err := ValidateExactPreparedGenerationForRequest(prepared, generation)
	limit, ok := err.(*ExactPreparedOutputLimitReachedError)
	if !ok {
		t.Fatalf("length completion error = %T %v", err, err)
	}
	if limit.OutputTokens != prepared.MaxOutputTokens || limit.ContentBytes < 1 {
		t.Fatalf("length evidence = %#v", limit)
	}
}

func TestExactPreparedGenerationRejectsAggregateNativeContextOverflow(t *testing.T) {
	t.Parallel()
	prepared := exactPreparedRequestFixture()
	generation := exactPreparedGenerationFixture(
		t, prepared, "stop", prepared.ContextTokens-11, 12,
	)
	err := ValidateExactPreparedGenerationForRequest(prepared, generation)
	if err == nil || !strings.Contains(err.Error(), "aggregate native context exceeded") {
		t.Fatalf("aggregate native overflow error = %v", err)
	}
}

func TestExactPreparedGenerationRejectsUnknownCompletionReason(t *testing.T) {
	t.Parallel()
	prepared := exactPreparedRequestFixture()
	generation := exactPreparedGenerationFixture(t, prepared, "cancelled", 40, 12)
	if err := ValidateExactPreparedGenerationForRequest(prepared, generation); err == nil {
		t.Fatal("unknown provider completion reason was accepted")
	}
}

func exactPreparedGenerationFixture(
	t *testing.T,
	prepared PreparedModel,
	doneReason string,
	promptTokens int,
	outputTokens int,
) PreparedGeneration {
	t.Helper()
	raw := []byte(fmt.Sprintf(
		`{"created_at":"2026-09-01T12:00:00Z","response":"candidate","done":true,"done_reason":%q,"total_duration":19,"load_duration":2,"prompt_eval_count":%d,"prompt_eval_duration":3,"eval_count":%d,"eval_duration":5}`,
		doneReason, promptTokens, outputTokens,
	))
	decoded, err := DecodeExactPreparedResponseForProtocol(prepared.Protocol, 200, raw)
	if err != nil {
		t.Fatal(err)
	}
	requestSHA, err := ExactPreparedRequestSHA256(prepared)
	if err != nil {
		t.Fatal(err)
	}
	responseDigest := sha256.Sum256(raw)
	responseSHA := hex.EncodeToString(responseDigest[:])
	return PreparedGeneration{
		Schema:                        PreparedGenerationSchemaV1,
		Protocol:                      prepared.Protocol,
		ProviderRequestDisposition:    ProviderRequestDispatched,
		Content:                       decoded.Content,
		ProviderRequestSHA256:         requestSHA,
		ProviderHTTPStatus:            200,
		ProviderResponseDisposition:   decoded.Disposition,
		ProviderResponseComplete:      true,
		ProviderContentEncoding:       NewProviderContentEncodingEvidence(nil, false),
		ProviderResponseBytesKnown:    true,
		ProviderResponseSHA256:        responseSHA,
		ProviderResponseBytes:         int64(len(raw)),
		ProviderResponseCaptureSHA256: responseSHA,
		ProviderResponseCapturedBytes: len(raw),
		ProviderResponseCapture:       raw,
		ProviderDonePresent:           decoded.DonePresent,
		ProviderDone:                  decoded.Done,
		ProviderDoneReason:            decoded.DoneReason,
		UsagePresent:                  decoded.UsagePresent,
		Usage:                         decoded.Usage,
	}
}
