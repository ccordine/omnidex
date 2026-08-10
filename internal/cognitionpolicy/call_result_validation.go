package cognitionpolicy

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

func (result CallResult) Validate(attempt CallAttempt) error {
	if err := attempt.Validate(); err != nil {
		return err
	}
	if result.Schema != CallResultSchemaV3 || result.CallID != attempt.ID {
		return fmt.Errorf("%w: call result identity is invalid", ErrInvalidEvidence)
	}
	if err := validateResultProviderEvidence(result, attempt); err != nil {
		return err
	}
	if err := validateCallResponse(result, attempt.RuntimeBudget); err != nil {
		return err
	}
	switch result.Status {
	case CallResultAccepted:
		if !result.ProviderIdentityChecked || !result.ProviderRequestDispatched ||
			result.ProviderResponseDisposition != llm.ProviderResponseSucceeded ||
			!result.ProviderDonePresent || !result.ProviderDone ||
			result.ProviderDoneReason != "stop" || result.ResponseBytes < 1 ||
			!result.ProviderUsagePresent || result.ProviderUsage.ValidateSuccessful() != nil ||
			result.ProviderUsage.PromptEvalCount > attempt.RuntimeBudget.MaxInputTokens ||
			result.ProviderUsage.EvalCount > attempt.RuntimeBudget.MaxOutputTokens ||
			!validPolicySHA256(result.ProviderResponseSHA256) ||
			result.FailureCode != "" || result.FailureMessage != "" ||
			!responseWithinStation(result, attempt.RuntimeBudget) ||
			!validPolicySHA256(result.DecisionSHA256) || result.ActionSchema.Validate() != nil {
			return fmt.Errorf("%w: accepted call result is incomplete", ErrInvalidEvidence)
		}
	case CallResultRejected:
		if err := validateRejectedCallResult(result, attempt); err != nil {
			return err
		}
	case CallResultFailed:
		if err := validateFailedCallResult(result, attempt); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: call result status %q is not registered", ErrInvalidEvidence, result.Status)
	}
	if result.Status != CallResultAccepted && !validBoundedText(result.FailureMessage, MaxCallFailureBytes) {
		return fmt.Errorf("%w: call failure message is invalid", ErrInvalidEvidence)
	}
	return nil
}

func validateResultProviderEvidence(result CallResult, attempt CallAttempt) error {
	if result.ProviderIdentityEvidence != (llm.ProviderIdentityEvidenceRef{}) &&
		result.ProviderIdentityEvidence.Validate() != nil {
		return fmt.Errorf("%w: provider identity evidence reference is invalid", ErrInvalidEvidence)
	}
	if result.ProviderGenerationEvidence != (ProviderGenerationEvidenceRef{}) {
		if result.Status != CallResultFailed ||
			(result.FailureCode != CallFailureProviderEvidence &&
				result.FailureCode != CallFailureProviderRequest &&
				result.FailureCode != CallFailurePolicyAuthority) ||
			(result.FailureCode == CallFailureProviderRequest && !result.ProviderRequestDispatched) ||
			result.ProviderGenerationEvidence.ValidateFor(attempt.ID) != nil ||
			result.ProviderIdentityChecked ||
			!reflect.DeepEqual(result.ProviderAttestation, llm.ProviderIdentityAttestation{}) ||
			!reflect.DeepEqual(result.ProviderObservation, llm.ProviderIdentityObservation{}) ||
			result.ProviderRequestSHA256 != "" || result.ProviderHTTPStatus != 0 ||
			result.ProviderResponseDisposition != "" || result.ProviderResponseComplete ||
			result.ProviderContentEncoding != (llm.ProviderContentEncodingEvidence{}) ||
			result.ProviderResponseBytesKnown || result.ProviderResponseSHA256 != "" ||
			result.ProviderResponseBytes != 0 || result.ProviderResponseCaptureSHA256 != "" ||
			result.ProviderResponseCapturedBytes != 0 || result.ProviderResponseModel != "" ||
			result.ProviderDonePresent || result.ProviderDone || result.ProviderDoneReason != "" ||
			result.ProviderUsagePresent || result.ProviderUsage != (llm.ProviderGenerationUsage{}) ||
			(result.ProviderResponseCapture != (ProviderResponseCaptureEvidenceRef{}) &&
				result.ProviderResponseCapture.ValidateFor(attempt.ID) != nil) {
			return fmt.Errorf("%w: untrusted provider result shape is invalid", ErrInvalidEvidence)
		}
		return nil
	}
	expected, err := attempt.Brain.ProviderExpectation()
	if err != nil {
		return fmt.Errorf("%w: call result brain is invalid", ErrInvalidEvidence)
	}
	if !result.ProviderIdentityChecked {
		if !reflect.DeepEqual(result.ProviderAttestation, llm.ProviderIdentityAttestation{}) ||
			!reflect.DeepEqual(result.ProviderObservation, llm.ProviderIdentityObservation{}) ||
			result.ProviderRequestDispatched || result.ProviderRequestSHA256 != "" ||
			result.ProviderHTTPStatus != 0 || result.ProviderResponseDisposition != "" ||
			result.ProviderResponseComplete || result.ProviderResponseBytesKnown ||
			result.ProviderContentEncoding != (llm.ProviderContentEncodingEvidence{}) ||
			result.ProviderResponseSHA256 != "" ||
			result.ProviderResponseBytes != 0 || result.ProviderResponseCaptureSHA256 != "" ||
			result.ProviderResponseCapturedBytes != 0 || result.ProviderResponseModel != "" ||
			result.ProviderDonePresent || result.ProviderDone || result.ProviderDoneReason != "" ||
			result.ProviderUsagePresent ||
			!reflect.DeepEqual(result.ProviderUsage, llm.ProviderGenerationUsage{}) ||
			result.ProviderResponseCapture != (ProviderResponseCaptureEvidenceRef{}) ||
			(result.FailureCode != CallFailureProviderIdentity &&
				result.ProviderIdentityEvidence != (llm.ProviderIdentityEvidenceRef{})) ||
			(result.FailureCode == CallFailureProviderIdentity &&
				result.ProviderIdentityEvidence.Validate() != nil) {
			return fmt.Errorf("%w: unchecked result claims provider evidence", ErrInvalidEvidence)
		}
		return nil
	}
	if result.ProviderIdentityEvidence.Validate() != nil {
		return fmt.Errorf("%w: checked provider result lacks raw identity evidence", ErrInvalidEvidence)
	}
	if result.ProviderObservation.Evidence != result.ProviderIdentityEvidence {
		return fmt.Errorf("%w: provider observation names another raw identity evidence", ErrInvalidEvidence)
	}
	challenge, err := callProviderObservationChallenge(attempt, expected)
	if err != nil || result.ProviderAttestation != attempt.ProviderAttestation ||
		result.ProviderAttestation.ValidateFor(expected) != nil {
		return fmt.Errorf("%w: fresh provider observation is invalid", ErrInvalidEvidence)
	}
	invocation := llm.PreparedGeneration{
		Schema: llm.PreparedGenerationSchemaV1, ProviderRequestDispatched: result.ProviderRequestDispatched,
		ProviderRequestSHA256:         result.ProviderRequestSHA256,
		ProviderHTTPStatus:            result.ProviderHTTPStatus,
		ProviderResponseDisposition:   result.ProviderResponseDisposition,
		ProviderResponseComplete:      result.ProviderResponseComplete,
		ProviderContentEncoding:       result.ProviderContentEncoding,
		ProviderResponseBytesKnown:    result.ProviderResponseBytesKnown,
		ProviderResponseSHA256:        result.ProviderResponseSHA256,
		ProviderResponseBytes:         result.ProviderResponseBytes,
		ProviderResponseCaptureSHA256: result.ProviderResponseCaptureSHA256,
		ProviderResponseCapturedBytes: result.ProviderResponseCapturedBytes,
		ProviderResponseModel:         result.ProviderResponseModel,
		ProviderDonePresent:           result.ProviderDonePresent,
		ProviderDone:                  result.ProviderDone,
		ProviderDoneReason:            result.ProviderDoneReason,
		UsagePresent:                  result.ProviderUsagePresent,
		Usage:                         result.ProviderUsage,
		ProviderObservation:           result.ProviderObservation,
	}
	if result.ProviderObservation.ValidateFor(result.ProviderAttestation, challenge) != nil {
		return fmt.Errorf("%w: fresh provider observation is invalid", ErrInvalidEvidence)
	}
	if err := invocation.ValidateProviderResponseReceipt(); err != nil {
		return fmt.Errorf("%w: provider invocation receipt: %v", ErrInvalidEvidence, err)
	}
	if (result.ProviderResponseDisposition == llm.ProviderResponseSucceeded ||
		result.ProviderResponseDisposition == llm.ProviderResponseEmptyContent) &&
		result.ProviderResponseModel != attempt.Brain.Model {
		return fmt.Errorf("%w: provider response model differs from the exact brain", ErrInvalidEvidence)
	}
	if result.ProviderResponseDisposition == llm.ProviderResponseTransportError {
		if result.ProviderResponseCapture != (ProviderResponseCaptureEvidenceRef{}) {
			return fmt.Errorf("%w: transport result claims response capture evidence", ErrInvalidEvidence)
		}
	} else if result.ProviderResponseCapture.ValidateFor(attempt.ID) != nil ||
		result.ProviderResponseCapture.SHA256 != result.ProviderResponseCaptureSHA256 ||
		result.ProviderResponseCapture.Bytes != result.ProviderResponseCapturedBytes {
		return fmt.Errorf("%w: provider response capture reference is invalid", ErrInvalidEvidence)
	}
	requestMismatch := result.ProviderRequestSHA256 != attempt.ExpectedProviderRequestSHA256
	registeredMismatch := result.Status == CallResultFailed &&
		result.FailureCode == CallFailureProviderRequest
	if requestMismatch != registeredMismatch {
		return fmt.Errorf("%w: provider request identity does not match its result disposition", ErrInvalidEvidence)
	}
	return nil
}

func validateRejectedCallResult(result CallResult, attempt CallAttempt) error {
	if !result.ProviderIdentityChecked || !result.ProviderRequestDispatched ||
		!reflect.DeepEqual(result.ActionSchema, cognition.ActionSchemaRef{}) ||
		result.DecisionSHA256 != "" {
		return fmt.Errorf("%w: rejected call result shape is invalid", ErrInvalidEvidence)
	}
	switch result.FailureCode {
	case CallFailureResponseLimit, CallFailureInvalidDecision, CallFailureAuthorityDenied,
		CallFailureProviderUsageLimit:
		if result.ProviderResponseDisposition != llm.ProviderResponseSucceeded ||
			result.ResponseBytes < 1 ||
			!result.ProviderUsagePresent || result.ProviderUsage.ValidateSuccessful() != nil {
			return fmt.Errorf("%w: rejected result lacks exact provider usage", ErrInvalidEvidence)
		}
		if result.FailureCode == CallFailureProviderUsageLimit &&
			result.ProviderUsage.PromptEvalCount <= attempt.RuntimeBudget.MaxInputTokens &&
			result.ProviderUsage.EvalCount <= attempt.RuntimeBudget.MaxOutputTokens {
			return fmt.Errorf("%w: provider usage limit rejection is within budget", ErrInvalidEvidence)
		}
		if result.FailureCode != CallFailureProviderUsageLimit &&
			(result.ProviderUsage.PromptEvalCount > attempt.RuntimeBudget.MaxInputTokens ||
				result.ProviderUsage.EvalCount > attempt.RuntimeBudget.MaxOutputTokens) {
			return fmt.Errorf("%w: non-usage rejection exceeds native usage budget", ErrInvalidEvidence)
		}
		if result.FailureCode == CallFailureResponseLimit &&
			result.ResponseBytes <= attempt.RuntimeBudget.MaxOutputBytes &&
			!(result.ProviderDoneReason == "length" &&
				result.ProviderUsage.EvalCount == attempt.RuntimeBudget.MaxOutputTokens) {
			return fmt.Errorf("%w: response limit rejection is within budget", ErrInvalidEvidence)
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

func validateFailedCallResult(result CallResult, attempt CallAttempt) error {
	if result.FailureCode != CallFailureGeneration &&
		result.FailureCode != CallFailureProviderIdentity &&
		result.FailureCode != CallFailureProviderRequest &&
		result.FailureCode != CallFailurePolicyAuthority &&
		result.FailureCode != CallFailureProviderEvidence {
		return fmt.Errorf("%w: failed call failure code is invalid", ErrInvalidEvidence)
	}
	providerFailure := result.FailureCode == CallFailureProviderIdentity
	requestFailure := result.FailureCode == CallFailureProviderRequest
	evidenceFailure := result.FailureCode == CallFailureProviderEvidence
	authorityFailure := result.FailureCode == CallFailurePolicyAuthority
	if providerFailure && (result.ProviderIdentityChecked || result.ProviderRequestDispatched) {
		return fmt.Errorf("%w: provider identity failure claims provider execution", ErrInvalidEvidence)
	}
	if !providerFailure && !evidenceFailure && result.ProviderGenerationEvidence == (ProviderGenerationEvidenceRef{}) &&
		result.ProviderIdentityChecked != result.ProviderRequestDispatched {
		return fmt.Errorf("%w: generation failure has inconsistent provider execution", ErrInvalidEvidence)
	}
	if requestFailure && result.ProviderGenerationEvidence == (ProviderGenerationEvidenceRef{}) &&
		(!result.ProviderIdentityChecked || !result.ProviderRequestDispatched) {
		return fmt.Errorf("%w: provider request mismatch lacks executed provider evidence", ErrInvalidEvidence)
	}
	if evidenceFailure && !result.ProviderRequestDispatched {
		return fmt.Errorf("%w: provider evidence failure lacks a dispatched request", ErrInvalidEvidence)
	}
	if authorityFailure && result.ProviderRequestDispatched && !result.ProviderIdentityChecked {
		return fmt.Errorf("%w: dispatched policy authority failure lacks provider evidence", ErrInvalidEvidence)
	}
	if result.ProviderUsagePresent && result.ProviderUsage.Validate() != nil {
		return fmt.Errorf("%w: failed result has invalid native usage telemetry", ErrInvalidEvidence)
	}
	if result.ProviderResponseDisposition == llm.ProviderResponseTransportError &&
		(result.ProviderUsagePresent || result.ProviderUsage != (llm.ProviderGenerationUsage{})) {
		return fmt.Errorf("%w: transport failure claims native usage", ErrInvalidEvidence)
	}
	if result.FailureCode == CallFailureGeneration &&
		result.ProviderResponseDisposition == llm.ProviderResponseSucceeded {
		return fmt.Errorf("%w: generation failure claims a successful provider return", ErrInvalidEvidence)
	}
	if !reflect.DeepEqual(result.ActionSchema, cognition.ActionSchemaRef{}) || result.DecisionSHA256 != "" {
		return fmt.Errorf("%w: failed call result shape is invalid", ErrInvalidEvidence)
	}
	return nil
}

func validateCallResponse(result CallResult, budget cognition.RuntimeBudget) error {
	if result.ResponseBytes < 0 || !validPolicySHA256OrEmpty(result.ResponseSHA256) {
		return fmt.Errorf("%w: response identity is invalid", ErrInvalidEvidence)
	}
	if result.ResponseBytes == 0 {
		if result.ResponseSHA256 != "" || result.ResponseEvidence != (ModelResponseEvidenceRef{}) {
			return fmt.Errorf("%w: empty response claims model evidence", ErrInvalidEvidence)
		}
		return nil
	}
	if !result.ProviderRequestDispatched || !result.ProviderResponseComplete ||
		!result.ProviderResponseBytesKnown ||
		(result.ProviderResponseDisposition != llm.ProviderResponseSucceeded &&
			!(result.Status == CallResultFailed && result.FailureCode == CallFailureGeneration &&
				result.ProviderResponseDisposition == llm.ProviderResponseEmptyContent)) {
		return fmt.Errorf("%w: model response lacks an exact complete provider result", ErrInvalidEvidence)
	}
	if result.ResponseEvidence.ValidateFor(result.CallID) != nil ||
		result.ResponseEvidence.SHA256 != result.ResponseSHA256 ||
		result.ResponseEvidence.Bytes != result.ResponseBytes {
		return fmt.Errorf("%w: response evidence reference is invalid", ErrInvalidEvidence)
	}
	return nil
}

func responseWithinStation(result CallResult, budget cognition.RuntimeBudget) bool {
	return result.ResponseBytes <= budget.MaxOutputBytes
}

func callProviderObservationChallenge(
	attempt CallAttempt,
	expected llm.ProviderIdentityExpectation,
) (string, error) {
	return llm.DeriveProviderIdentityObservationChallenge(
		"cognition-policy-call:"+attempt.ID, expected,
	)
}
