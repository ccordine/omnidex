package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
)

type exactEvidenceStationFixture struct {
	candidate     string
	nativeContext string
	doneReason    string
	promptTokens  int
	outputTokens  int
	partial       []byte
	entered       chan<- struct{}
	release       <-chan struct{}
}

type exactEvidenceStationClient struct {
	fixtures []exactEvidenceStationFixture
	prepared []llm.PreparedModel
	calls    int
}

func (client *exactEvidenceStationClient) GeneratePreparedExact(
	_ context.Context,
	prepared llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	if client.calls >= len(client.fixtures) {
		return llm.PreparedGeneration{}, fmt.Errorf("unexpected provider invocation")
	}
	fixture := client.fixtures[client.calls]
	client.calls++
	client.prepared = append(client.prepared, prepared)
	if fixture.entered != nil {
		close(fixture.entered)
	}
	if fixture.release != nil {
		<-fixture.release
	}
	if fixture.partial != nil {
		return exactEvidencePartialGeneration(prepared, fixture.partial), errors.New(
			"provider body read stopped after the captured prefix",
		)
	}
	if fixture.doneReason == "length" {
		return exactEvidenceLengthGeneration(
			prepared, fixture.candidate, fixture.promptTokens, fixture.outputTokens,
		)
	}
	generation, err := exactEvidenceSuccessfulGeneration(prepared, fixture.candidate)
	if err == nil && fixture.nativeContext != "" {
		generation = exactEvidenceWithNativeContext(generation, fixture.nativeContext)
	}
	return generation, err
}

func exactEvidenceSuccessfulGeneration(
	prepared llm.PreparedModel,
	candidate string,
) (llm.PreparedGeneration, error) {
	return exactEvidenceCompletedGeneration(prepared, candidate, "stop", 11, 7)
}

func exactEvidenceLengthGeneration(
	prepared llm.PreparedModel,
	candidate string,
	promptTokens int,
	outputTokens int,
) (llm.PreparedGeneration, error) {
	if promptTokens == 0 {
		promptTokens = 11
	}
	if outputTokens <= 0 {
		outputTokens = prepared.MaxOutputTokens
		if outputTokens == -1 {
			outputTokens = 7
		}
	}
	return exactEvidenceCompletedGeneration(
		prepared, candidate, "length", promptTokens, outputTokens,
	)
}

func exactEvidenceCompletedGeneration(
	prepared llm.PreparedModel,
	candidate string,
	doneReason string,
	promptTokens int,
	outputTokens int,
) (llm.PreparedGeneration, error) {
	raw := []byte(fmt.Sprintf(
		`{"created_at":"2026-08-31T12:00:00Z","response":%q,"done":true,"done_reason":%q,"total_duration":19,"load_duration":2,"prompt_eval_count":%d,"prompt_eval_duration":3,"eval_count":%d,"eval_duration":5}`,
		candidate, doneReason, promptTokens, outputTokens,
	))
	decoded, err := llm.DecodeExactPreparedResponseForProtocol(prepared.Protocol, 200, raw)
	if err != nil {
		return llm.PreparedGeneration{}, err
	}
	return llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition: llm.ProviderRequestDispatched,
		Content:                    candidate, ProviderHTTPStatus: 200,
		ProviderResponseDisposition: decoded.Disposition,
		ProviderResponseComplete:    true, ProviderResponseBytesKnown: true,
		ProviderContentEncoding:       llm.ClassifyProviderContentEncoding(nil, false),
		ProviderResponseBytes:         int64(len(raw)),
		ProviderResponseCapturedBytes: len(raw), ProviderResponseCapture: raw,
		ProviderDonePresent: decoded.DonePresent, ProviderDone: decoded.Done,
		ProviderDoneReason: decoded.DoneReason,
		UsagePresent:       decoded.UsagePresent, Usage: decoded.Usage,
	}, nil
}

func exactEvidencePartialGeneration(
	prepared llm.PreparedModel,
	raw []byte,
) llm.PreparedGeneration {
	return llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, Protocol: prepared.Protocol,
		ProviderRequestDisposition:    llm.ProviderRequestDispatched,
		ProviderHTTPStatus:            200,
		ProviderResponseDisposition:   llm.ProviderResponseBodyReadError,
		ProviderContentEncoding:       llm.ClassifyProviderContentEncoding(nil, false),
		ProviderResponseCapturedBytes: len(raw), ProviderResponseCapture: append([]byte(nil), raw...),
	}
}

func exactEvidenceWithNativeContext(generation llm.PreparedGeneration, nativeContext string) llm.PreparedGeneration {
	raw := generation.ProviderResponseCapture
	raw = append(append([]byte(nil), raw[:len(raw)-1]...), []byte(`,"context":`+nativeContext+`}`)...)
	generation.ProviderResponseCapture = raw
	generation.ProviderResponseCapturedBytes = len(raw)
	generation.ProviderResponseBytes = int64(len(raw))
	return generation
}

func assertExactEvidenceRequestContext(t *testing.T, request []byte, expected string) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(request, &fields); err != nil {
		t.Fatal(err)
	}
	if actual := string(fields["context"]); actual != expected {
		t.Fatalf("provider request context=%s, want %s", actual, expected)
	}
}
