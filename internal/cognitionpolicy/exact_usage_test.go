package cognitionpolicy

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

func TestPolicyDurablyRejectsProviderNativeUsageBeyondEitherCeiling(t *testing.T) {
	t.Parallel()
	for name, change := range map[string]func(*llm.ProviderGenerationUsage, cognition.RuntimeBudget){
		"input": func(usage *llm.ProviderGenerationUsage, budget cognition.RuntimeBudget) {
			usage.PromptEvalCount = budget.MaxInputTokens + 1
		},
		"output": func(usage *llm.ProviderGenerationUsage, budget cognition.RuntimeBudget) {
			usage.EvalCount = budget.MaxOutputTokens + 1
		},
	} {
		name, change := name, change
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			projection := policyTestProjection(t, "native usage ceiling authority")
			snapshot, evidence := policyTestSnapshot(t, projection)
			client := &policyTestClient{response: policyTestResponse(t, snapshot, evidence)}
			client.generationOverride = func(
				_ llm.PreparedModel,
				generation llm.PreparedGeneration,
			) (llm.PreparedGeneration, error) {
				change(&generation.Usage, snapshot.Budget())
				policyTestRefreshRawProviderResponse(&generation, true)
				return generation, nil
			}
			journal := &policyTestCallJournal{}
			policy, err := New(
				client, policyTestAttestedBrain(), policyTestActivation(),
				newPolicyTestProjectionLoader(projection), journal,
			)
			if err != nil {
				t.Fatal(err)
			}
			outcome, err := policy.Decide(context.Background(), snapshot)
			if !errors.Is(err, ErrProviderUsageLimit) || !outcome.PolicyCallConsumed {
				t.Fatalf("outcome=%+v error=%v", outcome, err)
			}
			if len(journal.results) != 1 ||
				journal.results[0].Status != CallResultRejected ||
				journal.results[0].FailureCode != CallFailureProviderUsageLimit ||
				!journal.results[0].ProviderUsagePresent {
				t.Fatalf("usage-limit result=%+v", journal.results)
			}
		})
	}
}

func TestPolicyPersistsExecutedResponseWithMalformedNativeUsage(t *testing.T) {
	t.Parallel()
	projection := policyTestProjection(t, "malformed usage authority")
	snapshot, evidence := policyTestSnapshot(t, projection)
	client := &policyTestClient{response: policyTestResponse(t, snapshot, evidence)}
	client.generationOverride = func(
		_ llm.PreparedModel,
		generation llm.PreparedGeneration,
	) (llm.PreparedGeneration, error) {
		generation.UsagePresent = false
		generation.Usage.EvalCount = 0
		policyTestRefreshRawProviderResponse(&generation, false)
		return generation, errors.New("provider omitted eval_count")
	}
	journal := &policyTestCallJournal{}
	policy, err := New(
		client, policyTestAttestedBrain(), policyTestActivation(), newPolicyTestProjectionLoader(projection), journal,
	)
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := policy.Decide(context.Background(), snapshot)
	if !errors.Is(err, ErrProviderUsage) || !outcome.PolicyCallConsumed {
		t.Fatalf("outcome=%+v error=%v", outcome, err)
	}
	if len(journal.results) != 1 {
		t.Fatalf("results=%d want 1", len(journal.results))
	}
	result := journal.results[0]
	if result.FailureCode != CallFailureProviderUsage || result.ProviderUsagePresent ||
		result.ProviderResponseSHA256 == "" || result.ProviderRequestSHA256 == "" ||
		result.ProviderObservation.ObservationSHA256 == "" ||
		result.ProviderRequestDisposition != llm.ProviderRequestDispatched {
		t.Fatalf("malformed usage result lost partial provider evidence: %+v", result)
	}
}

func TestCallResultRejectsProviderObservationFromAnotherReservedCall(t *testing.T) {
	t.Parallel()
	first := policyTestCallAttempt(t)
	second := first
	second.Actor.Attempt++
	second.ProviderProcessActivation.Actor = second.Actor
	second.ID = callAttemptID(second)
	if err := second.Validate(); err != nil {
		t.Fatal(err)
	}
	generation := policyTestPreparedGeneration(first, `{"invalid":true}`)
	result := rejectedCallResult(
		second, generation, CallFailureInvalidDecision, errors.New("invalid decision"),
	)
	if err := result.Validate(second); err == nil {
		t.Fatal("provider observation from another call was accepted")
	}
}

func TestCallResultRejectsFalseCodeDerivedLimitAttribution(t *testing.T) {
	t.Parallel()
	attempt := policyTestCallAttempt(t)
	within := policyTestPreparedGeneration(attempt, `{"invalid":true}`)
	for name, result := range map[string]CallResult{
		"provider usage": rejectedCallResult(
			attempt, within, CallFailureProviderUsageLimit, errors.New("false usage limit"),
		),
		"response": rejectedCallResult(
			attempt, within, CallFailureResponseLimit, errors.New("false response limit"),
		),
	} {
		result := result
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := result.Validate(attempt); err == nil {
				t.Fatalf("accepted false %s attribution", name)
			}
		})
	}
	over := within
	over.Content = strings.Repeat("x", attempt.RuntimeBudget.MaxOutputBytes+1)
	valid := rejectedCallResult(attempt, over, CallFailureResponseLimit, errors.New("response limit"))
	if err := valid.Validate(attempt); err != nil {
		t.Fatalf("rejected actual response limit: %v", err)
	}
	evidence, err := NewModelResponseEvidence(attempt.ID, over.Content)
	if err != nil {
		t.Fatal(err)
	}
	if valid.ResponseEvidence != evidence.Ref || valid.ResponseBytes != len(over.Content) ||
		string(evidence.Content) != over.Content {
		t.Fatal("over-station response was not retained as exact rejection evidence")
	}
}

func TestCallResultRejectsUnreachableResponseAndFailureShapes(t *testing.T) {
	t.Parallel()
	attempt := policyTestCallAttempt(t)
	generation := policyTestPreparedGeneration(attempt, `{"invalid":true}`)
	projection := policyTestProjection(t, "result schema")
	snapshot, _ := policyTestSnapshot(t, projection)

	accepted := acceptedCallResult(
		attempt, generation, snapshot.ActionCatalog().Schemas[0].Ref(), strings.Repeat("a", 64),
	)
	acceptedWithoutResponse := accepted
	acceptedWithoutResponse.ResponseBytes = 0
	acceptedWithoutResponse.ResponseSHA256 = ""
	acceptedWithoutResponse.ResponseEvidence = ModelResponseEvidenceRef{}
	acceptedLength := accepted
	acceptedLength.ProviderDoneReason = "length"

	for name, result := range map[string]CallResult{
		"unchecked done reason": func() CallResult {
			failedIdentity := policyTestFailedProviderIdentityGeneration(attempt)
			value := providerIdentityFailedCallResult(
				attempt, failedIdentity, errors.New("identity"),
			)
			value.ProviderDoneReason = "stop"
			return value
		}(),
		"accepted without response": acceptedWithoutResponse,
		"accepted length stop":      acceptedLength,
		"generation succeeded": failedCallResult(
			attempt, generation, errors.New("impossible generation failure"),
		),
	} {
		result := result
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if err := result.Validate(attempt); err == nil {
				t.Fatalf("unreachable result validated: %+v", result)
			}
		})
	}

	for _, code := range []CallFailureCode{
		CallFailureInvalidDecision, CallFailureAuthorityDenied,
		CallFailureProviderUsageLimit, CallFailureResponseLimit,
	} {
		result := rejectedCallResult(attempt, generation, code, errors.New("rejected"))
		result.ResponseBytes = 0
		result.ResponseSHA256 = ""
		result.ResponseEvidence = ModelResponseEvidenceRef{}
		if err := result.Validate(attempt); err == nil {
			t.Fatalf("rejection %q without response evidence validated", code)
		}
	}
}
