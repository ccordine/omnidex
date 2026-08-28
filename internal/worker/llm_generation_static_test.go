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

func TestExactStationStaticBudgetDoesNotGuessMeasuredBytesAreTokens(t *testing.T) {
	contract, err := llmResponseContractForScope("portable_fragment_worker")
	if err != nil {
		t.Fatal(err)
	}
	const measuredRawInputBytes = 6485
	promptBytes := measuredRawInputBytes - len(llm.ExactPreparedPromptJoiner) - len(llm.MinimalGeneratePrompt)
	err = validateExactStationStaticCall(
		strings.Repeat("x", promptBytes),
		contract,
		llm.ProviderIdentitySelection{Model: "qwen3.5:9b", NativeContextLimit: 8192},
	)
	if err != nil {
		t.Fatalf("measured correction envelope was blocked before provider discovery: %v", err)
	}
}

func TestExactStationStaticBudgetRejectsRemovedExplicitOutputMode(t *testing.T) {
	contract, err := llmResponseContractForScope("portable_fragment_worker")
	if err != nil {
		t.Fatal(err)
	}
	contract.OutputLimitMode = llm.ExactPreparedOutputLimitExplicit
	if err := validateExactStationStaticCall(
		"exact bounded prompt", contract,
		llm.ProviderIdentitySelection{Model: "qwen3.5:9b", NativeContextLimit: 8192},
	); err == nil {
		t.Fatal("portable station accepted the removed explicit output mode")
	}
}

func TestExactStationStaticContractAcceptsBoundedRegisteredEnvelope(t *testing.T) {
	contract, err := llmResponseContractForScope("portable_semantic_worker")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateExactStationStaticCall(
		"exact bounded prompt",
		contract,
		llm.ProviderIdentitySelection{Model: "qwen3.5:9b", NativeContextLimit: 8192},
	); err != nil {
		t.Fatal(err)
	}
}
