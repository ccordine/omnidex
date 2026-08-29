package llm

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExactPreparedGenerationRejectsRawChatMLControlLeakage(t *testing.T) {
	t.Parallel()
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
	valid := exactPreparedGenerationForRequestTest(t, prepared, "semantic leaf")
	if err := ValidateExactPreparedGenerationForRequest(prepared, valid); err != nil {
		t.Fatalf("valid request-bound raw generation was rejected: %v", err)
	}

	for _, control := range []string{
		"<|im_start|>", ExactPreparedRawChatEndV1,
	} {
		control := control
		t.Run(control, func(t *testing.T) {
			leaked := exactPreparedGenerationForRequestTest(
				t, prepared, "semantic "+control+" leaf",
			)
			if err := ValidateExactPreparedGenerationForRequest(prepared, leaked); err == nil {
				t.Fatalf("raw generation accepted leaked control %q", control)
			}
		})
	}
	literalTag := exactPreparedGenerationForRequestTest(
		t, prepared, "A literal <think> element is ordinary result content.",
	)
	if err := ValidateExactPreparedGenerationForRequest(prepared, literalTag); err != nil {
		t.Fatalf("ordinary literal tag content was rejected: %v", err)
	}

	wrongRequest := valid
	wrongRequest.ProviderRequestSHA256 = strings.Repeat("f", 64)
	if err := ValidateExactPreparedGenerationForRequest(prepared, wrongRequest); err == nil {
		t.Fatal("generation from another exact request was accepted")
	}
}

func TestExactPreparedGenerationClassifiesOnlyRequestBoundOutputLimitEvidence(t *testing.T) {
	t.Parallel()
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
	limited := exactPreparedGenerationForRequestTest(t, prepared, "partial declaration")
	limited = exactPreparedGenerationWithDoneReason(t, limited, "length")

	err := ValidateExactPreparedGenerationForRequest(prepared, limited)
	var outputLimit *ExactPreparedOutputLimitReachedError
	if !errors.As(err, &outputLimit) {
		t.Fatalf("request-bound length receipt error=%v", err)
	}
	if outputLimit.DoneReason != "length" ||
		outputLimit.PromptTokens != limited.Usage.PromptEvalCount ||
		outputLimit.OutputTokens != limited.Usage.EvalCount ||
		outputLimit.ContextTokens != prepared.ContextTokens ||
		outputLimit.MaxOutputTokens != prepared.MaxOutputTokens ||
		outputLimit.ContentBytes != len(limited.Content) ||
		outputLimit.Validate() != nil {
		t.Fatalf("typed output-limit evidence=%+v", outputLimit)
	}

	wrongRequest := limited
	wrongRequest.ProviderRequestSHA256 = strings.Repeat("f", 64)
	err = ValidateExactPreparedGenerationForRequest(prepared, wrongRequest)
	outputLimit = nil
	if err == nil || errors.As(err, &outputLimit) {
		t.Fatalf("wrong-request length receipt gained routing authority: %v", err)
	}

	invalidUsage := limited
	invalidUsage.Usage.EvalCount = prepared.ContextTokens
	err = ValidateExactPreparedGenerationForRequest(prepared, invalidUsage)
	outputLimit = nil
	if err == nil || errors.As(err, &outputLimit) {
		t.Fatalf("invalid-usage length receipt gained routing authority: %v", err)
	}
}

func exactPreparedGenerationForRequestTest(
	t *testing.T,
	prepared PreparedModel,
	content string,
) PreparedGeneration {
	t.Helper()
	expected := *prepared.ProviderIdentityExpectation
	attestation, err := NewProviderIdentityAttestation(
		expected, "test:version", "test:installed", "test:runner",
	)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := NewObservedProviderIdentity(
		time.Now().UTC().Truncate(time.Microsecond), attestation,
		providerIdentityTestEvidence(t, expected), prepared.ProviderObservationChallenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	requestSHA256, err := ExactPreparedRequestSHA256(prepared)
	if err != nil {
		t.Fatal(err)
	}
	body := exactProtocolResponseBody(t, content)
	bodySHA256 := providerBodySHA256(body)
	return PreparedGeneration{
		Schema: PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition:    ProviderRequestDispatched,
		Content:                       content,
		ProviderRequestSHA256:         requestSHA256,
		ProviderHTTPStatus:            200,
		ProviderResponseDisposition:   ProviderResponseSucceeded,
		ProviderResponseComplete:      true,
		ProviderContentEncoding:       NewProviderContentEncodingEvidence(nil, false),
		ProviderResponseBytesKnown:    true,
		ProviderResponseSHA256:        bodySHA256,
		ProviderResponseBytes:         int64(len(body)),
		ProviderResponseCaptureSHA256: bodySHA256,
		ProviderResponseCapturedBytes: len(body),
		ProviderResponseCapture:       body,
		ProviderResponseModel:         prepared.ContextModel,
		ProviderDonePresent:           true,
		ProviderDone:                  true,
		ProviderDoneReason:            "stop",
		UsagePresent:                  true,
		Usage: ProviderGenerationUsage{
			PromptEvalCount: 41, EvalCount: 7, TotalDurationNanos: 101,
			LoadDurationNanos: 11, PromptEvalDurationNanos: 21,
			EvalDurationNanos: 31,
		},
		ProviderObservation:      observed.Observation,
		ProviderIdentityEvidence: observed.Evidence,
	}
}

func exactPreparedGenerationWithDoneReason(
	t *testing.T,
	generation PreparedGeneration,
	doneReason string,
) PreparedGeneration {
	t.Helper()
	prior := []byte(`"done_reason":"` + generation.ProviderDoneReason + `"`)
	next := []byte(`"done_reason":"` + doneReason + `"`)
	body := bytes.Replace(generation.ProviderResponseCapture, prior, next, 1)
	if bytes.Equal(body, generation.ProviderResponseCapture) {
		t.Fatalf("provider response body lacks done reason %q", generation.ProviderDoneReason)
	}
	generation.ProviderDoneReason = doneReason
	generation.ProviderResponseCapture = body
	generation.ProviderResponseCapturedBytes = len(body)
	generation.ProviderResponseBytes = int64(len(body))
	generation.ProviderResponseSHA256 = providerBodySHA256(body)
	generation.ProviderResponseCaptureSHA256 = generation.ProviderResponseSHA256
	return generation
}
