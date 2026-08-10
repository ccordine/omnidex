package cognitionpolicy

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

func TestVerifyCallAttemptRequiresExactRenderedAuthority(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, _ := policyTestSnapshot(t, projection)
	rendered, err := Render(snapshot, projection, policyTestBrain())
	if err != nil {
		t.Fatal(err)
	}
	attempt, err := newCallAttempt(snapshot, policyTestAttestedBrain(), rendered)
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifyCallAttempt(snapshot, projection, attempt); err != nil {
		t.Fatalf("verify exact attempt: %v", err)
	}

	for name, mutate := range map[string]func(*CallAttempt){
		"self-hashed envelope": func(value *CallAttempt) {
			value.Envelope = strings.Replace(value.Envelope, decisionInstruction, "Choose one registered action and return its exact object.", 1)
			refreshCallInputIdentity(value)
		},
		"self-hashed response contract": func(value *CallAttempt) {
			value.ResponseContractSHA256 = strings.Repeat("b", 64)
			value.ID = callAttemptID(*value)
		},
	} {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			forged := attempt
			mutate(&forged)
			if err := forged.Validate(); err != nil {
				t.Fatalf("forgery must be internally self-consistent: %v", err)
			}
			if err := VerifyCallAttempt(snapshot, projection, forged); !errors.Is(err, ErrInvalidEvidence) {
				t.Fatalf("error = %v, want ErrInvalidEvidence", err)
			}
		})
	}

	wrongHint := attempt
	wrongHint.PromptHint = "different caller hint"
	refreshCallInputIdentity(&wrongHint)
	if err := VerifyCallAttempt(snapshot, projection, wrongHint); !errors.Is(err, ErrInvalidEvidence) {
		t.Fatalf("wrong hint error = %v, want ErrInvalidEvidence", err)
	}
}

func refreshCallInputIdentity(attempt *CallAttempt) {
	attempt.EnvelopeBytes = len(attempt.Envelope)
	attempt.EnvelopeEstimatedTokens = estimatePolicyTokens(attempt.EnvelopeBytes)
	attempt.EnvelopeSHA256 = policySHA256(attempt.Envelope)
	attempt.PromptHintBytes = len(attempt.PromptHint)
	attempt.PromptHintSHA256 = policySHA256(attempt.PromptHint)
	modelInput := attempt.Envelope + llm.ExactPreparedPromptJoiner + attempt.PromptHint
	attempt.ModelVisibleInputBytes = len(modelInput)
	attempt.ModelVisibleEstimatedTokens = estimatePolicyTokens(attempt.ModelVisibleInputBytes)
	upperBound, err := llm.ModelInputTokenUpperBound(
		modelInput, attempt.Brain.Sampling.InputSpecialTokenReserve,
	)
	if err != nil {
		panic(err)
	}
	attempt.ModelInputTokenUpperBound = upperBound
	attempt.ModelVisibleInputSHA256 = policySHA256(modelInput)
	attempt.ID = callAttemptID(*attempt)
}

func TestRenderCountsExactPromptHintAtInputBoundary(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "direct authority")
	snapshot, _ := policyTestSnapshot(t, projection)
	measured, err := MeasureEnvelope(snapshot, projection)
	if err != nil {
		t.Fatal(err)
	}
	budget := snapshot.Budget()
	budget.MaxInputBytes = measured.Bytes + len(llm.MinimalGeneratePrompt) - 1
	budget.MaxInputTokens = budget.MaxInputBytes + policyTestBrain().Sampling.InputSpecialTokenReserve
	bounded, err := cognition.NewRuntimeSnapshot(
		snapshot.Goal(), snapshot.CurrentRevision(), snapshot.CurrentObligation(),
		snapshot.ActionCatalog(), snapshot.Attempt(), snapshot.ContextProjection(),
		budget, snapshot.EvidenceRefs(),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Render(bounded, projection, policyTestBrain()); !errors.Is(err, ErrEnvelopeLimit) {
		t.Fatalf("error = %v, want ErrEnvelopeLimit", err)
	}
}
