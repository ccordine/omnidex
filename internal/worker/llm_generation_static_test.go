package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

func TestProviderRequestFailureReasonPreservesExactTerminalAuthority(t *testing.T) {
	t.Parallel()

	for status, want := range map[model.StepAttemptStatus]llm.ProviderRequestFailureReason{
		model.StepAttemptCanceled:   llm.ProviderRequestFailureAuthorityCanceled,
		model.StepAttemptSuperseded: llm.ProviderRequestFailureAuthoritySuperseded,
		model.StepAttemptExpired:    llm.ProviderRequestFailureAuthorityExpired,
	} {
		got, err := providerRequestFailureForAttempt(status)
		if err != nil {
			t.Fatalf("status %q: %v", status, err)
		}
		if got != want {
			t.Fatalf("status %q reason=%q want %q", status, got, want)
		}
	}
	if _, err := providerRequestFailureForAttempt(model.StepAttemptActive); err == nil {
		t.Fatal("active attempt was represented as terminal provider authority")
	}
}

func TestExactStationStaticBudgetRejectsBeforeProviderDiscovery(t *testing.T) {
	contract, err := llmResponseContractForScope("portable_semantic_worker")
	if err != nil {
		t.Fatal(err)
	}
	err = validateExactStationStaticCall(
		strings.Repeat("x", 8192),
		map[string]any{"type": "object"},
		contract,
		llm.ProviderIdentitySelection{Model: "qwen3.5:9b", NativeContextLimit: 8192},
	)
	if err == nil || !strings.Contains(err.Error(), "exceeds token authority") {
		t.Fatalf("static station budget error=%v", err)
	}
}

func TestExactStationStaticContractAcceptsBoundedRegisteredEnvelope(t *testing.T) {
	contract, err := llmResponseContractForScope("portable_semantic_worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateExactStationStaticCall(
		"exact bounded prompt",
		map[string]any{"type": "object"},
		contract,
		llm.ProviderIdentitySelection{Model: "qwen3.5:9b", NativeContextLimit: 8192},
	); err != nil {
		t.Fatal(err)
	}
}
