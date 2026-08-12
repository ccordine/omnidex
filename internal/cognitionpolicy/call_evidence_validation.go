package cognitionpolicy

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/llm"
)

func (evidence CallEvidence) ValidateFor(attempt CallAttempt, result CallResult) error {
	if result.ProviderIdentityEvidence == (llm.ProviderIdentityEvidenceRef{}) {
		if !reflect.DeepEqual(evidence.ProviderIdentity, llm.ProviderIdentityEvidence{}) {
			return fmt.Errorf("%w: result without provider identity has raw identity bytes", ErrInvalidEvidence)
		}
	} else if evidence.ProviderIdentity.ValidateRequests(llm.ProviderIdentitySelection{
		Model: attempt.Brain.Model, NativeContextLimit: attempt.Brain.NativeContextLimit,
	}) != nil || evidence.ProviderIdentity.Ref != result.ProviderIdentityEvidence {
		return fmt.Errorf("%w: provider identity evidence differs from its result", ErrInvalidEvidence)
	}
	if result.ProviderIdentityChecked && validateObservedProviderForAttempt(
		attempt, result.ProviderObservation, evidence.ProviderIdentity,
	) != nil {
		return fmt.Errorf("%w: provider observation differs from exact raw provider identity", ErrInvalidEvidence)
	}
	if result.FailureCode == CallFailureProviderIdentity {
		if !providerIdentityEvidenceProvesFailure(attempt, evidence.ProviderIdentity) {
			return fmt.Errorf("%w: provider identity failure is not proven by raw evidence", ErrInvalidEvidence)
		}
	} else if result.ProviderRequestDisposition.MayHaveReachedProvider() &&
		result.ProviderIdentityEvidence != (llm.ProviderIdentityEvidenceRef{}) &&
		!evidence.ProviderIdentity.Successful() {
		return fmt.Errorf("%w: executed provider result lacks complete identity evidence", ErrInvalidEvidence)
	}
	if result.ProviderResponseCapture == (ProviderResponseCaptureEvidenceRef{}) {
		if !reflect.DeepEqual(evidence.ProviderResponseCapture, ProviderResponseCaptureEvidence{}) {
			return fmt.Errorf("%w: result without provider response has capture bytes", ErrInvalidEvidence)
		}
	} else if evidence.ProviderResponseCapture.Validate() != nil ||
		evidence.ProviderResponseCapture.CallID != attempt.ID ||
		evidence.ProviderResponseCapture.Ref != result.ProviderResponseCapture {
		return fmt.Errorf("%w: provider response capture differs from its result", ErrInvalidEvidence)
	}
	if result.ProviderGenerationEvidence == (ProviderGenerationEvidenceRef{}) {
		if err := validateProviderCaptureProjection(result, evidence); err != nil {
			return err
		}
	}
	if result.ResponseBytes == 0 {
		if !reflect.DeepEqual(evidence.Response, ModelResponseEvidence{}) {
			return fmt.Errorf("%w: empty result has model response evidence", ErrInvalidEvidence)
		}
	} else if evidence.Response.Validate() != nil ||
		evidence.Response.CallID != attempt.ID || evidence.Response.Ref != result.ResponseEvidence {
		return fmt.Errorf("%w: model response evidence differs from its result", ErrInvalidEvidence)
	}
	if result.ProviderGenerationEvidence == (ProviderGenerationEvidenceRef{}) {
		if !reflect.DeepEqual(evidence.ProviderGeneration, ProviderGenerationEvidence{}) {
			return fmt.Errorf("%w: trusted result has untrusted provider evidence", ErrInvalidEvidence)
		}
		return nil
	}
	if evidence.ProviderGeneration.Validate() != nil ||
		evidence.ProviderGeneration.CallID != attempt.ID ||
		evidence.ProviderGeneration.Ref != result.ProviderGenerationEvidence {
		return fmt.Errorf("%w: untrusted provider evidence differs from its result", ErrInvalidEvidence)
	}
	generation, providerErrorPresent, _, complete, err :=
		inspectProviderGenerationOutcomeEvidence(evidence.ProviderGeneration.Generation)
	if err != nil {
		return fmt.Errorf("%w: decode untrusted provider generation: %v", ErrInvalidEvidence, err)
	}
	if !complete {
		if result.FailureCode != CallFailureProviderEvidence &&
			result.FailureCode != CallFailurePolicyAuthority {
			return fmt.Errorf("%w: request mismatch cannot rely on overflow evidence", ErrInvalidEvidence)
		}
		return nil
	}
	if result.ProviderIdentityEvidence != (llm.ProviderIdentityEvidenceRef{}) {
		if !reflect.DeepEqual(generation.ProviderIdentityEvidence, evidence.ProviderIdentity) {
			return fmt.Errorf("%w: opaque generation changed its provider identity evidence", ErrInvalidEvidence)
		}
	}
	if result.ProviderResponseCapture != (ProviderResponseCaptureEvidenceRef{}) {
		generation.ProviderResponseCapture = append(
			[]byte(nil), evidence.ProviderResponseCapture.Content...,
		)
	}
	captureRequired := generation.ProviderRequestDisposition == llm.ProviderRequestDispatched &&
		(generation.ProviderResponseDisposition != llm.ProviderResponseTransportError ||
			len(generation.ProviderResponseCapture) > 0)
	if captureRequired !=
		(result.ProviderResponseCapture != (ProviderResponseCaptureEvidenceRef{})) {
		return fmt.Errorf("%w: untrusted provider capture disposition changed", ErrInvalidEvidence)
	}
	switch result.FailureCode {
	case CallFailureProviderEvidence:
		providerErr := validatePreparedGenerationProvider(attempt, generation)
		responseErr := generation.ValidateProviderResponseEvidence()
		if providerErr == nil && responseErr == nil && !providerErrorPresent {
			return fmt.Errorf("%w: provider evidence failure contains valid provider evidence", ErrInvalidEvidence)
		}
	case CallFailureProviderRequest:
		if generation.ProviderRequestSHA256 == attempt.ExpectedProviderRequestSHA256 {
			return fmt.Errorf("%w: provider request failure contains the expected request", ErrInvalidEvidence)
		}
	case CallFailurePolicyAuthority:
		if generation.ProviderRequestDisposition.MayHaveReachedProvider() {
			return fmt.Errorf("%w: predispatch authority evidence claims dispatch", ErrInvalidEvidence)
		}
	default:
		return fmt.Errorf("%w: untrusted provider evidence has an unrelated failure code", ErrInvalidEvidence)
	}
	return nil
}

func validateProviderCaptureProjection(result CallResult, evidence CallEvidence) error {
	generation := llm.PreparedGeneration{
		Schema:                     llm.PreparedGenerationSchemaV1,
		ProviderRequestDisposition: result.ProviderRequestDisposition,
		Content:                    string(evidence.Response.Content), ProviderRequestSHA256: result.ProviderRequestSHA256,
		ProviderHTTPStatus:            result.ProviderHTTPStatus,
		ProviderResponseDisposition:   result.ProviderResponseDisposition,
		ProviderResponseComplete:      result.ProviderResponseComplete,
		ProviderContentEncoding:       result.ProviderContentEncoding,
		ProviderResponseBytesKnown:    result.ProviderResponseBytesKnown,
		ProviderResponseSHA256:        result.ProviderResponseSHA256,
		ProviderResponseBytes:         result.ProviderResponseBytes,
		ProviderResponseCaptureSHA256: result.ProviderResponseCaptureSHA256,
		ProviderResponseCapturedBytes: result.ProviderResponseCapturedBytes,
		ProviderResponseCapture:       evidence.ProviderResponseCapture.Content,
		ProviderResponseModel:         result.ProviderResponseModel,
		ProviderDonePresent:           result.ProviderDonePresent, ProviderDone: result.ProviderDone,
		ProviderDoneReason: result.ProviderDoneReason,
		UsagePresent:       result.ProviderUsagePresent, Usage: result.ProviderUsage,
	}
	if err := llm.ValidateExactPreparedResponseProjection(generation); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEvidence, err)
	}
	return nil
}
