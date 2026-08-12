package cognitionpolicy

import (
	"errors"
	"strings"
	"testing"
)

func TestCallResultRejectedDecisionRequiresFinalStop(t *testing.T) {
	attempt := policyTestCallAttempt(t)
	generation := policyTestPreparedGeneration(attempt, `{"invalid":true}`)
	valid := rejectedCallResult(
		attempt, generation, CallFailureInvalidDecision, errors.New("invalid decision"),
	)
	if err := valid.Validate(attempt); err != nil {
		t.Fatalf("valid rejected decision: %v", err)
	}
	mutations := map[string]func(*CallResult){
		"missing done": func(value *CallResult) { value.ProviderDonePresent = false },
		"done false":   func(value *CallResult) { value.ProviderDone = false },
		"length":       func(value *CallResult) { value.ProviderDoneReason = "length" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			forged := valid
			mutate(&forged)
			if err := forged.Validate(attempt); err == nil {
				t.Fatal("decision rejection accepted an unreachable provider finality")
			}
		})
	}
}

func TestCallResultResponseLimitDistinguishesByteAndNativeTokenLimits(t *testing.T) {
	attempt := policyTestCallAttempt(t)
	byteGeneration := policyTestPreparedGeneration(attempt,
		strings.Repeat("x", attempt.RuntimeBudget.MaxOutputBytes+1))
	byteLimit := rejectedCallResult(
		attempt, byteGeneration, CallFailureResponseLimit, errors.New("byte limit"),
	)
	if err := byteLimit.Validate(attempt); err != nil {
		t.Fatalf("valid byte limit: %v", err)
	}

	tokenGeneration := policyTestPreparedGeneration(attempt, `{"invalid":true}`)
	tokenGeneration.ProviderDoneReason = "length"
	tokenGeneration.Usage.EvalCount = attempt.RuntimeBudget.MaxOutputTokens
	policyTestRefreshRawProviderResponse(&tokenGeneration, true)
	tokenLimit := rejectedCallResult(
		attempt, tokenGeneration, CallFailureResponseLimit, errors.New("token limit"),
	)
	if err := tokenLimit.Validate(attempt); err != nil {
		t.Fatalf("valid native token limit: %v", err)
	}
	forged := tokenLimit
	forged.ProviderUsage.EvalCount--
	if err := forged.Validate(attempt); err == nil {
		t.Fatal("token-limit rejection accepted a value below the exact native ceiling")
	}
}
