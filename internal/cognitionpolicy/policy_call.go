package cognitionpolicy

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

func (policy *Policy) executeReservedCall(
	ctx context.Context,
	attempt CallAttempt,
	snapshot cognition.RuntimeSnapshot,
) (cognition.PolicyOutcome, error) {
	prepared, err := policy.prepareModelCall(ctx, attempt, snapshot)
	if err != nil {
		failure := fmt.Errorf("%w: prepare exact policy authority: %v", ErrInvalidEvidence, err)
		return cognition.PolicyOutcome{}, policy.finishCall(
			ctx, attempt, policyAuthorityFailedCallResult(
				attempt, llm.PreparedGeneration{}, failure,
			), llm.PreparedGeneration{}, failure,
		)
	}
	defer policy.client.CleanupPreparedModel(prepared)
	contractSHA, err := preparedModelSHA(prepared)
	if err != nil {
		failure := fmt.Errorf("%w: exact prepared contract identity: %v", ErrInvalidEvidence, err)
		return cognition.PolicyOutcome{}, policy.finishCall(
			ctx, attempt, policyAuthorityFailedCallResult(
				attempt, llm.PreparedGeneration{}, failure,
			), llm.PreparedGeneration{}, failure,
		)
	}
	expectedRequestSHA, err := llm.ExactPreparedRequestSHA256(prepared)
	if err != nil || expectedRequestSHA != attempt.ExpectedProviderRequestSHA256 {
		failure := fmt.Errorf("%w: exact prepared provider request identity changed", ErrInvalidEvidence)
		return cognition.PolicyOutcome{}, policy.finishCall(
			ctx, attempt, policyAuthorityFailedCallResult(
				attempt, llm.PreparedGeneration{}, failure,
			), llm.PreparedGeneration{}, failure,
		)
	}
	generation, generationErr := policy.exactClient.GeneratePreparedExact(ctx, prepared)
	executed := cognition.PolicyOutcome{ProviderRequestDispatched: generation.ProviderRequestDispatched}
	if !generation.ProviderRequestDispatched {
		if providerIdentityFailureEvidence(attempt, generation) {
			failure := fmt.Errorf("%w: %v", ErrProviderIdentity, generationErr)
			return executed, policy.finishCall(
				ctx, attempt, providerIdentityFailedCallResult(attempt, generation, failure), generation, failure,
			)
		}
		failure := fmt.Errorf(
			"%w: exact client returned a non-dispatched outcome without failed identity evidence: %v",
			ErrInvalidEvidence, generationErr,
		)
		return executed, policy.finishUntrustedCall(
			ctx, attempt, generation, CallFailurePolicyAuthority, failure,
		)
	}
	providerErr := validatePreparedGenerationProvider(attempt, generation)
	responseEvidenceErr := generation.ValidateProviderResponseEvidence()
	if providerErr != nil || responseEvidenceErr != nil {
		code := CallFailureProviderEvidence
		failure := fmt.Errorf(
			"%w: provider observation or response receipt is invalid: %v / %v",
			ErrInvalidEvidence, providerErr, responseEvidenceErr,
		)
		if validPolicySHA256(generation.ProviderRequestSHA256) &&
			generation.ProviderRequestSHA256 != expectedRequestSHA {
			code = CallFailureProviderRequest
			failure = fmt.Errorf(
				"%w: %w: provider request and receipt authority differ",
				ErrGeneration, ErrInvalidEvidence,
			)
		}
		return executed, policy.finishUntrustedCall(ctx, attempt, generation, code, failure)
	}
	if generation.ProviderRequestSHA256 != expectedRequestSHA {
		failure := fmt.Errorf(
			"%w: %w: provider request identity differs from the exact prepared request",
			ErrGeneration, ErrInvalidEvidence,
		)
		return executed, policy.finishCall(
			ctx, attempt, providerRequestFailedCallResult(attempt, generation, failure),
			generation, failure,
		)
	}
	if after, hashErr := preparedModelSHA(prepared); hashErr != nil || after != contractSHA {
		failure := fmt.Errorf("%w: prepared generation contract was mutated", ErrInvalidEvidence)
		return executed, policy.finishCall(
			ctx, attempt, policyAuthorityFailedCallResult(attempt, generation, failure),
			generation, failure,
		)
	}
	if generationErr != nil {
		if generation.ProviderResponseDisposition == llm.ProviderResponseSucceeded {
			if generation.Validate() == nil {
				failure := fmt.Errorf(
					"%w: provider returned a valid successful response together with an error",
					ErrInvalidEvidence,
				)
				return executed, policy.finishCall(
					ctx, attempt, policyAuthorityFailedCallResult(attempt, generation, failure),
					generation, failure,
				)
			}
			failure := fmt.Errorf("%w: %v", ErrProviderUsage, generationErr)
			result := rejectedCallResult(attempt, generation, CallFailureProviderUsage, failure)
			return executed, policy.finishCall(ctx, attempt, result, generation, failure)
		}
		failure := fmt.Errorf("%w: %v", ErrGeneration, generationErr)
		return executed, policy.finishCall(
			ctx, attempt, failedCallResult(attempt, generation, failure), generation, failure,
		)
	}
	if err := generation.Validate(); err != nil {
		failure := fmt.Errorf("%w: %v", ErrProviderUsage, err)
		result := rejectedCallResult(attempt, generation, CallFailureProviderUsage, failure)
		return executed, policy.finishCall(ctx, attempt, result, generation, failure)
	}
	budget := attempt.RuntimeBudget
	if generation.Usage.PromptEvalCount > budget.MaxInputTokens ||
		generation.Usage.EvalCount > budget.MaxOutputTokens {
		failure := fmt.Errorf(
			"%w: provider used %d input and %d output tokens against ceilings %d/%d",
			ErrProviderUsageLimit, generation.Usage.PromptEvalCount,
			generation.Usage.EvalCount, budget.MaxInputTokens, budget.MaxOutputTokens,
		)
		result := rejectedCallResult(attempt, generation, CallFailureProviderUsageLimit, failure)
		return executed, policy.finishCall(ctx, attempt, result, generation, failure)
	}
	if generation.ProviderDoneReason == "length" {
		if generation.Usage.EvalCount != budget.MaxOutputTokens {
			failure := fmt.Errorf(
				"%w: provider reported length at %d tokens below the exact %d-token ceiling",
				ErrProviderUsage, generation.Usage.EvalCount, budget.MaxOutputTokens,
			)
			result := rejectedCallResult(attempt, generation, CallFailureProviderUsage, failure)
			return executed, policy.finishCall(ctx, attempt, result, generation, failure)
		}
		failure := fmt.Errorf(
			"%w: provider reached the exact %d-token output ceiling",
			ErrResponseLimit, budget.MaxOutputTokens,
		)
		result := rejectedCallResult(attempt, generation, CallFailureResponseLimit, failure)
		return executed, policy.finishCall(ctx, attempt, result, generation, failure)
	}
	response := generation.Content
	if len(response) > budget.MaxOutputBytes {
		failure := fmt.Errorf(
			"%w: response exceeds the %d-byte station ceiling",
			ErrResponseLimit, budget.MaxOutputBytes,
		)
		result := rejectedCallResult(
			attempt, generation, CallFailureResponseLimit, failure,
		)
		return executed, policy.finishCall(ctx, attempt, result, generation, failure)
	}
	if !validBoundedText(response, MaxModelResponseEvidenceBytes) {
		failure := fmt.Errorf("%w: response is not bounded valid UTF-8 JSON", ErrInvalidDecision)
		result := rejectedCallResult(attempt, generation, CallFailureInvalidDecision, failure)
		return executed, policy.finishCall(ctx, attempt, result, generation, failure)
	}

	decision, schema, err := decodePolicyDecision(response, snapshot)
	if err != nil {
		failureCode := CallFailureInvalidDecision
		if errors.Is(err, cognition.ErrAuthorityDenied) {
			failureCode = CallFailureAuthorityDenied
		}
		result := rejectedCallResult(
			attempt, generation, failureCode, err,
		)
		return executed, policy.finishCall(ctx, attempt, result, generation, err)
	}
	decisionRaw, err := json.Marshal(decision)
	if err != nil {
		failure := fmt.Errorf("%w: encode accepted decision: %v", ErrInvalidDecision, err)
		result := rejectedCallResult(
			attempt, generation, CallFailureInvalidDecision, failure,
		)
		return executed, policy.finishCall(ctx, attempt, result, generation, failure)
	}
	result := acceptedCallResult(
		attempt, generation, schema.Ref(), policySHA256(string(decisionRaw)),
	)
	if err := policy.finishCall(ctx, attempt, result, generation, nil); err != nil {
		return executed, err
	}
	executed.Decision = decision.Clone()
	return executed, nil
}

func providerIdentityFailureEvidence(
	attempt CallAttempt,
	generation llm.PreparedGeneration,
) bool {
	return providerIdentityEvidenceProvesFailure(attempt, generation.ProviderIdentityEvidence)
}

func providerIdentityEvidenceProvesFailure(
	attempt CallAttempt,
	evidence llm.ProviderIdentityEvidence,
) bool {
	selection := llm.ProviderIdentitySelection{
		Model: attempt.Brain.Model, NativeContextLimit: attempt.Brain.NativeContextLimit,
	}
	if evidence.ValidateRequests(selection) != nil {
		return false
	}
	if !evidence.Successful() {
		return true
	}
	derived, err := llm.DeriveExactProviderIdentityExpectation(
		evidence, selection,
	)
	if err != nil {
		return true
	}
	expected, err := attempt.Brain.ProviderExpectation()
	return err == nil && derived != expected
}

func decodePolicyDecision(
	response string,
	snapshot cognition.RuntimeSnapshot,
) (cognition.CognitionDecision, cognition.ActionSchema, error) {
	kind, err := responseActionKind(response)
	if err != nil {
		return cognition.CognitionDecision{}, cognition.ActionSchema{}, err
	}
	schema, exists := snapshot.ActionCatalog().Schema(kind)
	if !exists {
		return cognition.CognitionDecision{}, cognition.ActionSchema{}, fmt.Errorf(
			"%w: response action kind %q is absent from the bound catalog", ErrInvalidDecision, kind,
		)
	}
	decision, err := cognition.DecodeCognitionDecision([]byte(response), schema)
	if err != nil {
		return cognition.CognitionDecision{}, cognition.ActionSchema{}, fmt.Errorf("%w: %w", ErrInvalidDecision, err)
	}
	if err := validateDecisionForSnapshot(decision, snapshot); err != nil {
		return cognition.CognitionDecision{}, cognition.ActionSchema{}, err
	}
	return decision, schema, nil
}

func (policy *Policy) replayReservedCall(
	reservation CallReservation,
	snapshot cognition.RuntimeSnapshot,
) (cognition.PolicyOutcome, error) {
	if reservation.ExistingResult == nil {
		return cognition.PolicyOutcome{}, fmt.Errorf(
			"%w: reserved call %s has no durable terminal result",
			ErrCallIndeterminate, reservation.Attempt.ID,
		)
	}
	result := *reservation.ExistingResult
	switch result.Status {
	case CallResultAccepted:
		if reservation.ExistingResponseEvidence == nil {
			return cognition.PolicyOutcome{}, fmt.Errorf("%w: accepted replay lacks response evidence", ErrInvalidEvidence)
		}
		decision, err := decisionFromAcceptedResult(
			result, *reservation.ExistingResponseEvidence, snapshot,
		)
		return cognition.PolicyOutcome{Decision: decision}, err
	case CallResultRejected:
		return cognition.PolicyOutcome{}, CallResultError(result)
	case CallResultFailed:
		return cognition.PolicyOutcome{}, CallResultError(result)
	default:
		return cognition.PolicyOutcome{}, fmt.Errorf("%w: prior call result is invalid", ErrInvalidEvidence)
	}
}
