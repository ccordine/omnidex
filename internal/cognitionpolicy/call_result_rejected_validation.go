package cognitionpolicy

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

func validateRejectedCallResult(result CallResult, attempt CallAttempt) error {
	if !result.ProviderIdentityChecked ||
		result.ProviderRequestDisposition != llm.ProviderRequestDispatched ||
		!reflect.DeepEqual(result.ActionSchema, cognition.ActionSchemaRef{}) ||
		result.DecisionSHA256 != "" {
		return fmt.Errorf("%w: rejected call result shape is invalid", ErrInvalidEvidence)
	}
	switch result.FailureCode {
	case CallFailureResponseLimit:
		if !rejectedSuccessfulFinal(result) ||
			result.ProviderUsage.PromptEvalCount > attempt.RuntimeBudget.MaxInputTokens ||
			result.ProviderUsage.EvalCount > attempt.RuntimeBudget.MaxOutputTokens {
			return fmt.Errorf("%w: response-limit rejection lacks an in-budget final response", ErrInvalidEvidence)
		}
		byteLimit := result.ProviderDoneReason == "stop" &&
			result.ResponseBytes > attempt.RuntimeBudget.MaxOutputBytes
		tokenLimit := result.ProviderDoneReason == "length" &&
			result.ProviderUsage.EvalCount == attempt.RuntimeBudget.MaxOutputTokens
		if !byteLimit && !tokenLimit {
			return fmt.Errorf("%w: response limit rejection is within budget", ErrInvalidEvidence)
		}
	case CallFailureInvalidDecision, CallFailureAuthorityDenied:
		if !rejectedSuccessfulFinal(result) || result.ProviderDoneReason != "stop" ||
			result.ProviderUsage.PromptEvalCount > attempt.RuntimeBudget.MaxInputTokens ||
			result.ProviderUsage.EvalCount > attempt.RuntimeBudget.MaxOutputTokens {
			return fmt.Errorf("%w: decision rejection lacks an in-budget stop response", ErrInvalidEvidence)
		}
	case CallFailureProviderUsageLimit:
		if !rejectedSuccessfulFinal(result) ||
			(result.ProviderDoneReason != "stop" && result.ProviderDoneReason != "length") ||
			(result.ProviderUsage.PromptEvalCount <= attempt.RuntimeBudget.MaxInputTokens &&
				result.ProviderUsage.EvalCount <= attempt.RuntimeBudget.MaxOutputTokens) {
			return fmt.Errorf("%w: provider usage limit rejection is not an over-budget final response", ErrInvalidEvidence)
		}
	case CallFailureProviderUsage:
		if result.ProviderResponseDisposition != llm.ProviderResponseSucceeded ||
			result.ProviderResponseSHA256 == "" || result.ResponseBytes == 0 ||
			(result.ProviderDonePresent && result.ProviderDone &&
				(result.ProviderDoneReason == "stop" || result.ProviderDoneReason == "length") &&
				result.ProviderUsagePresent && result.ProviderUsage.ValidateSuccessful() == nil &&
				!(result.ProviderDoneReason == "length" &&
					result.ProviderUsage.EvalCount != attempt.RuntimeBudget.MaxOutputTokens)) {
			return fmt.Errorf("%w: usage rejection lacks exact provider response", ErrInvalidEvidence)
		}
	default:
		return fmt.Errorf("%w: rejected call failure code is invalid", ErrInvalidEvidence)
	}
	return nil
}

func rejectedSuccessfulFinal(result CallResult) bool {
	return result.ProviderResponseDisposition == llm.ProviderResponseSucceeded &&
		result.ResponseBytes > 0 && result.ProviderDonePresent && result.ProviderDone &&
		result.ProviderUsagePresent && result.ProviderUsage.ValidateSuccessful() == nil
}
