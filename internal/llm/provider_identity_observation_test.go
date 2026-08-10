package llm

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestProviderIdentityObservationBindsEveryFreshResponseBody(t *testing.T) {
	t.Parallel()
	expected := providerIdentityTestExpectation()
	attestation, err := NewProviderIdentityAttestation(
		expected, "test:version", "test:installed", "test:runner",
	)
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Date(2026, 8, 9, 20, 0, 0, 1, time.UTC)
	challenge, err := DeriveProviderIdentityObservationChallenge("test-observation", expected)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := NewObservedProviderIdentity(
		observedAt, attestation, providerIdentityTestEvidence(t, expected), challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	observation := observed.Observation
	if err := observed.ValidateFor(ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: challenge,
	}); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*ProviderIdentityObservation){
		"time": func(value *ProviderIdentityObservation) { value.ObservedAt = observedAt.Add(time.Nanosecond) },
		"attestation": func(value *ProviderIdentityObservation) {
			value.AttestationSHA256 = strings.Repeat("a", 64)
		},
		"version":   func(value *ProviderIdentityObservation) { value.VersionBodySHA256 = strings.Repeat("b", 64) },
		"installed": func(value *ProviderIdentityObservation) { value.InstalledBodySHA256 = strings.Repeat("c", 64) },
		"preload":   func(value *ProviderIdentityObservation) { value.PreloadBodySHA256 = strings.Repeat("d", 64) },
		"runner":    func(value *ProviderIdentityObservation) { value.RunnerBodySHA256 = strings.Repeat("e", 64) },
		"request":   func(value *ProviderIdentityObservation) { value.PreloadRequestSHA256 = strings.Repeat("f", 64) },
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			changed := observation
			mutate(&changed)
			if err := changed.ValidateFor(attestation, challenge); err == nil {
				t.Fatal("changed provider observation was accepted")
			}
		})
	}
}

func TestPreparedGenerationRequiresExactUsageAndProviderObservation(t *testing.T) {
	t.Parallel()
	expected := providerIdentityTestExpectation()
	attestation, err := NewProviderIdentityAttestation(
		expected, "test:version", "test:installed", "test:runner",
	)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := DeriveProviderIdentityObservationChallenge("test-generation", expected)
	if err != nil {
		t.Fatal(err)
	}
	observed, err := NewObservedProviderIdentity(
		time.Now().UTC(), attestation, providerIdentityTestEvidence(t, expected), challenge,
	)
	if err != nil {
		t.Fatal(err)
	}
	providerBody := []byte(`{"model":"qwen:9b","created_at":"2026-08-09T22:00:00Z",` +
		`"response":"{}","done":true,"done_reason":"stop","total_duration":100,` +
		`"load_duration":10,"prompt_eval_count":12,"prompt_eval_duration":20,` +
		`"eval_count":3,"eval_duration":30}`)
	result := PreparedGeneration{
		Schema: PreparedGenerationSchemaV1, ProviderRequestDispatched: true, Content: `{}`,
		ProviderRequestSHA256: strings.Repeat("b", 64), ProviderHTTPStatus: 200,
		ProviderResponseDisposition: ProviderResponseSucceeded,
		ProviderResponseComplete:    true, ProviderResponseBytesKnown: true,
		ProviderResponseSHA256: providerBodySHA256(providerBody),
		ProviderResponseBytes:  int64(len(providerBody)), ProviderResponseCaptureSHA256: strings.Repeat("a", 64),
		ProviderResponseCapturedBytes: len(providerBody), ProviderResponseCapture: providerBody,
		ProviderResponseModel: expected.Model,
		ProviderDonePresent:   true, ProviderDone: true,
		ProviderDoneReason: "stop", UsagePresent: true,
		Usage: ProviderGenerationUsage{
			PromptEvalCount: 12, EvalCount: 3, TotalDurationNanos: 100,
			LoadDurationNanos: 10, PromptEvalDurationNanos: 20, EvalDurationNanos: 30,
		},
		ProviderObservation:      observed.Observation,
		ProviderIdentityEvidence: observed.Evidence,
	}
	result.ProviderResponseCaptureSHA256 = result.ProviderResponseSHA256
	if err := result.Validate(); err != nil {
		t.Fatal(err)
	}
	result.Usage.PromptEvalCount = -1
	if err := result.Validate(); err == nil {
		t.Fatal("negative exact provider usage was accepted")
	}
	result.Usage.PromptEvalCount = 12
	result.ProviderResponseCapturedBytes--
	if err := result.Validate(); err == nil {
		t.Fatal("complete response with divergent captured byte count was accepted")
	}
	result.ProviderResponseCapturedBytes = len(providerBody)
	result.ProviderResponseCaptureSHA256 = strings.Repeat("c", 64)
	if err := result.Validate(); err == nil {
		t.Fatal("complete response with divergent captured hash was accepted")
	}
	result.ProviderResponseCaptureSHA256 = result.ProviderResponseSHA256
	result.Usage.TotalDurationNanos = 59
	if err := result.Validate(); err == nil {
		t.Fatal("native duration components larger than total were accepted")
	}
	result.Usage.TotalDurationNanos = 100
	result.ProviderResponseCapturedBytes = MaxExactPreparedProviderResponseBytes + 1
	result.ProviderResponseBytes = int64(result.ProviderResponseCapturedBytes)
	if err := result.Validate(); err == nil {
		t.Fatal("complete response beyond the provider capture bound was accepted")
	}
	result.ProviderResponseComplete = false
	result.ProviderResponseBytesKnown = false
	result.ProviderResponseBytes = 0
	result.ProviderResponseSHA256 = ""
	result.ProviderResponseDisposition = ProviderResponseBodyLimit
	result.ProviderResponseModel = ""
	result.ProviderDonePresent = false
	result.ProviderDone = false
	result.ProviderDoneReason = ""
	result.UsagePresent = false
	result.Usage = ProviderGenerationUsage{}
	result.ProviderResponseCapture = bytes.Repeat(
		[]byte{'x'}, MaxExactPreparedProviderResponseBytes+1,
	)
	result.ProviderResponseCaptureSHA256 = providerBodySHA256(result.ProviderResponseCapture)
	if err := result.ValidateProviderResponseEvidence(); err != nil {
		t.Fatalf("exact max+1 body-limit capture rejected: %v", err)
	}
	result.ProviderResponseCapturedBytes--
	if err := result.ValidateProviderResponseEvidence(); err == nil {
		t.Fatal("inexact body-limit capture was accepted")
	}
}
