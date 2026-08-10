package cognitionpolicy

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

func (policy *Policy) finishCall(
	ctx context.Context,
	attempt CallAttempt,
	result CallResult,
	generation llm.PreparedGeneration,
	primary error,
) error {
	evidence, err := callEvidenceForGeneration(attempt, result, generation)
	if err != nil {
		return joinPrimaryEvidenceError(primary, err)
	}
	return policy.finishCallEvidence(ctx, attempt, result, evidence, primary)
}

func (policy *Policy) finishUntrustedCall(
	ctx context.Context,
	attempt CallAttempt,
	generation llm.PreparedGeneration,
	providerErr error,
	code CallFailureCode,
	primary error,
) error {
	providerEvidence, err := newProviderGenerationOutcomeEvidence(
		attempt.ID, generation, providerErr,
	)
	if err != nil {
		return errors.Join(primary, err)
	}
	capture, err := providerResponseCaptureForUntrustedGeneration(attempt.ID, generation)
	if err != nil {
		return errors.Join(primary, err)
	}
	identity := generation.ProviderIdentityEvidence.Clone()
	identityRef := llm.ProviderIdentityEvidenceRef{}
	if identity.ValidateRequests(llm.ProviderIdentitySelection{
		Model: attempt.Brain.Model, NativeContextLimit: attempt.Brain.NativeContextLimit,
	}) == nil && (!generation.ProviderRequestDispatched || identity.Successful()) {
		identityRef = identity.Ref
	} else {
		identity = llm.ProviderIdentityEvidence{}
	}
	result := untrustedProviderFailedCallResult(
		attempt, generation.ProviderRequestDispatched, identityRef,
		providerEvidence.Ref, capture.Ref, code, primary,
	)
	return policy.finishCallEvidence(ctx, attempt, result, CallEvidence{
		ProviderIdentity:        identity,
		ProviderResponseCapture: capture, ProviderGeneration: providerEvidence,
	}, primary)
}

func providerResponseCaptureForUntrustedGeneration(
	callID string,
	generation llm.PreparedGeneration,
) (ProviderResponseCaptureEvidence, error) {
	if len(generation.ProviderResponseCapture) > llm.MaxExactPreparedProviderResponseBytes+1 {
		return ProviderResponseCaptureEvidence{}, nil
	}
	return providerResponseCaptureForGeneration(callID, generation)
}

func callEvidenceForGeneration(
	attempt CallAttempt,
	result CallResult,
	generation llm.PreparedGeneration,
) (CallEvidence, error) {
	response, err := NewModelResponseEvidence(attempt.ID, generation.Content)
	if err != nil {
		return CallEvidence{}, err
	}
	capture, err := providerResponseCaptureForGeneration(attempt.ID, generation)
	if err != nil {
		return CallEvidence{}, err
	}
	if response.Ref != result.ResponseEvidence || capture.Ref != result.ProviderResponseCapture {
		return CallEvidence{}, fmt.Errorf("%w: derived call evidence differs from result", ErrInvalidEvidence)
	}
	return CallEvidence{
		Response: response, ProviderIdentity: generation.ProviderIdentityEvidence.Clone(),
		ProviderResponseCapture: capture,
	}, nil
}

func providerResponseCaptureForGeneration(
	callID string,
	generation llm.PreparedGeneration,
) (ProviderResponseCaptureEvidence, error) {
	if !generation.ProviderRequestDispatched ||
		(generation.ProviderResponseDisposition == llm.ProviderResponseTransportError &&
			len(generation.ProviderResponseCapture) == 0) {
		return ProviderResponseCaptureEvidence{}, nil
	}
	return NewProviderResponseCaptureEvidence(callID, generation.ProviderResponseCapture)
}

func (policy *Policy) finishCallEvidence(
	ctx context.Context,
	attempt CallAttempt,
	result CallResult,
	evidence CallEvidence,
	primary error,
) error {
	if err := result.Validate(attempt); err != nil {
		return joinPrimaryEvidenceError(primary, err)
	}
	if err := evidence.ValidateFor(attempt, result); err != nil {
		evidenceErr := errors.Join(fmt.Errorf(
			"%w: exact provider/model evidence differs from the call result", ErrInvalidEvidence,
		), err)
		return joinPrimaryEvidenceError(primary, evidenceErr)
	}
	if err := policy.journal.Finish(ctx, attempt, result, evidence); err != nil {
		journalErr := fmt.Errorf("%w: finish call: %v", ErrCallJournal, err)
		if primary == nil {
			return journalErr
		}
		return errors.Join(primary, journalErr)
	}
	return primary
}

func joinPrimaryEvidenceError(primary, evidence error) error {
	if primary == nil {
		return evidence
	}
	return errors.Join(primary, evidence)
}
