package cognitionpolicy

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/llm"
)

func providerIdentityFailedCallResult(
	attempt CallAttempt,
	generation llm.PreparedGeneration,
	cause error,
) CallResult {
	result := CallResult{
		Schema: CallResultSchemaV3, CallID: attempt.ID, Status: CallResultFailed,
		ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
		FailureCode:                CallFailureProviderIdentity, FailureMessage: boundedFailureMessage(cause),
	}
	if generation.ProviderIdentityEvidence.Schema != "" {
		result.ProviderIdentityEvidence = generation.ProviderIdentityEvidence.Ref
	}
	return result
}

func policyAuthorityFailedCallResult(
	attempt CallAttempt,
	generation llm.PreparedGeneration,
	cause error,
) CallResult {
	result := providerResult(attempt, generation)
	if result.ProviderRequestDisposition == "" {
		result.ProviderRequestDisposition = llm.ProviderRequestNotDispatched
	}
	result.Status = CallResultFailed
	result.FailureCode = CallFailurePolicyAuthority
	result.FailureMessage = boundedFailureMessage(cause)
	return result
}

func untrustedProviderFailedCallResult(
	attempt CallAttempt,
	requestDisposition llm.ProviderRequestDisposition,
	identity llm.ProviderIdentityEvidenceRef,
	evidence ProviderGenerationEvidenceRef,
	capture ProviderResponseCaptureEvidenceRef,
	code CallFailureCode,
	cause error,
) CallResult {
	if requestDisposition.Validate() != nil {
		requestDisposition = ""
	}
	return CallResult{
		Schema: CallResultSchemaV3, CallID: attempt.ID, Status: CallResultFailed,
		ProviderRequestDisposition: requestDisposition, ProviderResponseCapture: capture,
		ProviderIdentityEvidence:   identity,
		ProviderGenerationEvidence: evidence,
		FailureCode:                code, FailureMessage: boundedFailureMessage(cause),
	}
}

func failedCallResult(
	attempt CallAttempt,
	generation llm.PreparedGeneration,
	cause error,
) CallResult {
	result := providerResult(attempt, generation)
	result.Status = CallResultFailed
	result.FailureCode = CallFailureGeneration
	result.FailureMessage = boundedFailureMessage(cause)
	return result
}

func providerRequestFailedCallResult(
	attempt CallAttempt,
	generation llm.PreparedGeneration,
	cause error,
) CallResult {
	result := providerResult(attempt, generation)
	result.Status = CallResultFailed
	result.FailureCode = CallFailureProviderRequest
	result.FailureMessage = boundedFailureMessage(cause)
	return result
}

func rejectedCallResult(
	attempt CallAttempt,
	generation llm.PreparedGeneration,
	code CallFailureCode,
	cause error,
) CallResult {
	result := providerResult(attempt, generation)
	result.Status = CallResultRejected
	result.FailureCode = code
	result.FailureMessage = boundedFailureMessage(cause)
	return result
}

func acceptedCallResult(
	attempt CallAttempt,
	generation llm.PreparedGeneration,
	schema cognition.ActionSchemaRef,
	decisionSHA string,
) CallResult {
	result := providerResult(attempt, generation)
	result.Status = CallResultAccepted
	result.ActionSchema = schema
	result.DecisionSHA256 = decisionSHA
	return result
}

func providerResult(attempt CallAttempt, generation llm.PreparedGeneration) CallResult {
	result := CallResult{
		Schema: CallResultSchemaV3, CallID: attempt.ID,
		ProviderRequestDisposition:    generation.ProviderRequestDisposition,
		ProviderRequestSHA256:         generation.ProviderRequestSHA256,
		ProviderHTTPStatus:            generation.ProviderHTTPStatus,
		ProviderResponseDisposition:   generation.ProviderResponseDisposition,
		ProviderResponseComplete:      generation.ProviderResponseComplete,
		ProviderContentEncoding:       generation.ProviderContentEncoding,
		ProviderResponseBytesKnown:    generation.ProviderResponseBytesKnown,
		ProviderResponseSHA256:        generation.ProviderResponseSHA256,
		ProviderResponseBytes:         generation.ProviderResponseBytes,
		ProviderResponseCaptureSHA256: generation.ProviderResponseCaptureSHA256,
		ProviderResponseCapturedBytes: generation.ProviderResponseCapturedBytes,
		ProviderResponseModel:         generation.ProviderResponseModel,
		ProviderDonePresent:           generation.ProviderDonePresent,
		ProviderDone:                  generation.ProviderDone,
		ProviderDoneReason:            generation.ProviderDoneReason,
		ProviderUsagePresent:          generation.UsagePresent,
		ProviderUsage:                 generation.Usage,
	}
	if generation.ProviderObservation.Schema != "" {
		result.ProviderIdentityChecked = true
		result.ProviderAttestation = attempt.ProviderAttestation
		result.ProviderObservation = generation.ProviderObservation
	}
	if generation.ProviderIdentityEvidence.Schema != "" {
		result.ProviderIdentityEvidence = generation.ProviderIdentityEvidence.Ref
	}
	if generation.Content != "" {
		result.ResponseSHA256 = policySHA256(generation.Content)
		result.ResponseBytes = len(generation.Content)
		result.ResponseEvidence = modelResponseEvidenceRef(attempt.ID, generation.Content)
	}
	if generation.ProviderRequestDisposition == llm.ProviderRequestDispatched &&
		(generation.ProviderResponseDisposition != llm.ProviderResponseTransportError ||
			len(generation.ProviderResponseCapture) > 0) {
		result.ProviderResponseCapture = providerResponseCaptureEvidenceRef(
			attempt.ID, generation.ProviderResponseCapture,
		)
	}
	return result
}

func boundedFailureMessage(cause error) string {
	if cause == nil {
		return "The cognition policy call failed without a registered detail."
	}
	message := strings.TrimSpace(cause.Error())
	if validBoundedText(message, MaxCallFailureBytes) {
		return message
	}
	return fmt.Sprintf(
		"The failure detail was not safe to persist; exact byte SHA-256 is %s.",
		policySHA256(cause.Error()),
	)
}

func decisionFromAcceptedResult(
	result CallResult,
	evidence ModelResponseEvidence,
	snapshot cognition.RuntimeSnapshot,
) (cognition.CognitionDecision, error) {
	if result.Status != CallResultAccepted || evidence.Validate() != nil ||
		evidence.Ref != result.ResponseEvidence {
		return cognition.CognitionDecision{}, fmt.Errorf("%w: prior call was not accepted", ErrCallRejected)
	}
	response := string(evidence.Content)
	catalog := snapshot.ActionCatalog()
	kind, err := responseActionKind(response)
	if err != nil {
		return cognition.CognitionDecision{}, err
	}
	schema, exists := catalog.Schema(kind)
	if !exists || schema.Ref() != result.ActionSchema {
		return cognition.CognitionDecision{}, fmt.Errorf("%w: prior call schema is no longer exact", ErrInvalidEvidence)
	}
	decision, err := cognition.DecodeCognitionDecision(evidence.Content, schema)
	if err != nil {
		return cognition.CognitionDecision{}, fmt.Errorf("%w: %v", ErrInvalidEvidence, err)
	}
	if err := validateDecisionForSnapshot(decision, snapshot); err != nil {
		return cognition.CognitionDecision{}, err
	}
	raw, err := json.Marshal(decision)
	if err != nil || policySHA256(string(raw)) != result.DecisionSHA256 {
		return cognition.CognitionDecision{}, fmt.Errorf("%w: prior decision hash changed", ErrInvalidEvidence)
	}
	return decision.Clone(), nil
}
