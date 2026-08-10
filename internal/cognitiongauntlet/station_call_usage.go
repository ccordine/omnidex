package cognitiongauntlet

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

func validateExecutedCallUsage(call ModelCallTrace) error {
	if call.InputBytes <= 0 || call.OutputBytes < 0 || call.InputTokens < 0 || call.OutputTokens < 0 ||
		call.InputBytes > int64(call.Budget.MaxInputBytes) {
		return fmt.Errorf("model-call trace usage authority is invalid")
	}
	if err := validateModelCallResultDisposition(call); err != nil {
		return err
	}
	if call.ProviderUsagePresent {
		if err := call.ProviderUsage.ValidateSuccessful(); err != nil ||
			call.InputTokens != int64(call.ProviderUsage.PromptEvalCount) ||
			call.OutputTokens != int64(call.ProviderUsage.EvalCount) {
			return fmt.Errorf("model-call trace does not bind exact native provider usage")
		}
	} else if call.InputTokens != 0 || call.OutputTokens != 0 ||
		!reflect.DeepEqual(call.ProviderUsage, llm.ProviderGenerationUsage{}) {
		return fmt.Errorf("model-call trace claims unavailable native provider usage")
	}
	inputOver := call.InputTokens > int64(call.Budget.MaxInputTokens)
	outputTokenOver := call.OutputTokens > int64(call.Budget.MaxOutputTokens)
	outputByteOver := call.OutputBytes > int64(call.Budget.MaxOutputBytes)
	if inputOver || outputTokenOver {
		if call.FailureCode != cognitionpolicy.CallFailureProviderUsageLimit {
			return fmt.Errorf("native provider token usage exceeds the station budget without exact rejection")
		}
	}
	if call.FailureCode == cognitionpolicy.CallFailureProviderUsageLimit &&
		!inputOver && !outputTokenOver {
		return fmt.Errorf("provider usage-limit disposition is within its station budget")
	}
	if outputByteOver && call.FailureCode != cognitionpolicy.CallFailureResponseLimit {
		return fmt.Errorf("model output bytes exceed the station budget without exact rejection")
	}
	if call.FailureCode == cognitionpolicy.CallFailureResponseLimit &&
		!outputByteOver && !(call.ProviderDoneReason == "length" && call.ProviderUsagePresent &&
		call.OutputTokens == int64(call.Budget.MaxOutputTokens)) {
		return fmt.Errorf("response-limit disposition is within its station budget")
	}
	return nil
}

func validateModelCallResultDisposition(call ModelCallTrace) error {
	switch call.ResultStatus {
	case cognitionpolicy.CallResultAccepted:
		if call.FailureCode != "" || call.ProviderResponseDisposition != llm.ProviderResponseSucceeded ||
			call.ProviderDoneReason != "stop" || !call.ProviderUsagePresent || call.OutputBytes <= 0 {
			return fmt.Errorf("accepted model-call trace is incomplete")
		}
	case cognitionpolicy.CallResultRejected:
		if !registeredRejectedFailure(call.FailureCode) ||
			call.ProviderResponseDisposition != llm.ProviderResponseSucceeded {
			return fmt.Errorf("rejected model-call trace disposition is invalid")
		}
		if call.FailureCode == cognitionpolicy.CallFailureResponseLimit {
			if call.ProviderDoneReason != "stop" && call.ProviderDoneReason != "length" {
				return fmt.Errorf("response-limit trace lacks an exact provider done reason")
			}
		} else if call.ProviderDoneReason != "stop" {
			return fmt.Errorf("rejected model-call trace changed its provider done reason")
		}
	case cognitionpolicy.CallResultFailed:
		if (call.FailureCode != cognitionpolicy.CallFailureGeneration &&
			call.FailureCode != cognitionpolicy.CallFailureProviderRequest) ||
			!registeredProviderDisposition(call.ProviderResponseDisposition) {
			return fmt.Errorf("failed model-call trace disposition is invalid")
		}
		if call.ProviderResponseDisposition == llm.ProviderResponseSucceeded {
			if call.ProviderDoneReason != "stop" && call.ProviderDoneReason != "length" {
				return fmt.Errorf("failed model-call trace lacks a provider done reason")
			}
		} else if call.ProviderDoneReason != "" {
			return fmt.Errorf("failed provider response claims a done reason")
		}
	default:
		return fmt.Errorf("model-call trace result status is not registered")
	}
	return nil
}

func registeredRejectedFailure(code cognitionpolicy.CallFailureCode) bool {
	switch code {
	case cognitionpolicy.CallFailureResponseLimit,
		cognitionpolicy.CallFailureInvalidDecision,
		cognitionpolicy.CallFailureAuthorityDenied,
		cognitionpolicy.CallFailureProviderUsage,
		cognitionpolicy.CallFailureProviderUsageLimit:
		return true
	default:
		return false
	}
}

func registeredProviderDisposition(value llm.ProviderResponseDisposition) bool {
	switch value {
	case llm.ProviderResponseSucceeded, llm.ProviderResponseTransportError,
		llm.ProviderResponseHTTPError, llm.ProviderResponseBodyLimit,
		llm.ProviderResponseBodyReadError, llm.ProviderResponseInvalidJSON,
		llm.ProviderResponseEmptyContent:
		return true
	default:
		return false
	}
}

func callBudgetQualified(call ModelCallTrace) bool {
	return call.InputBytes <= int64(call.Budget.MaxInputBytes) &&
		call.InputTokens <= int64(call.Budget.MaxInputTokens) &&
		call.OutputBytes <= int64(call.Budget.MaxOutputBytes) &&
		call.OutputTokens <= int64(call.Budget.MaxOutputTokens)
}
