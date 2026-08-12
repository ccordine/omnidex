package cognitionpolicy

import (
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/llm"
)

const providerIdentityFailureMetadataBytes = 64 * 1024

type ProviderIdentityFailureCode string

const (
	ProviderIdentityObservationFailed   ProviderIdentityFailureCode = "provider_identity_failed"
	ProviderIdentityObservationInvalid  ProviderIdentityFailureCode = "provider_observation_invalid"
	ProviderAttestationIdentityMismatch ProviderIdentityFailureCode = "provider_attestation_mismatch"
	ProviderHostAttestationFailed       ProviderIdentityFailureCode = "host_attestation_failed"
	ProviderHostIdentityMismatch        ProviderIdentityFailureCode = "host_identity_mismatch"
)

type providerIdentityFailureProof struct {
	Attestation llm.ProviderIdentityAttestation `json:"provider_attestation"`
	Observation llm.ProviderIdentityObservation `json:"provider_observation"`
}

func validateProviderIdentityFailureProof(
	code ProviderIdentityFailureCode,
	brain BrainRef,
	challenge string,
	proof providerIdentityFailureProof,
	evidence llm.ProviderIdentityEvidence,
) error {
	expected, err := brain.ProviderExpectation()
	if err != nil {
		return err
	}
	selection := llm.ProviderIdentitySelection{
		Model: brain.Model, NativeContextLimit: brain.NativeContextLimit,
	}
	if evidence.ValidateRequests(selection) != nil {
		return fmt.Errorf("%w: failure evidence changed its exact requests", ErrInvalidEvidence)
	}
	empty := providerIdentityFailureProof{}
	switch code {
	case ProviderIdentityObservationFailed:
		if proof != empty || evidence.ValidateFailure(selection, &expected) != nil {
			return fmt.Errorf("%w: raw evidence does not prove provider identity failure", ErrInvalidEvidence)
		}
	case ProviderIdentityObservationInvalid:
		if !providerIdentityFailureProofBounded(proof) || !evidence.Successful() ||
			(llm.ObservedProviderIdentity{
				Attestation: proof.Attestation, Observation: proof.Observation, Evidence: evidence,
			}).ValidateFor(llm.ProviderIdentityObservationRequest{
				Expectation: expected, ChallengeSHA256: challenge,
			}) == nil {
			return fmt.Errorf("%w: provider observation failure proof is not exact", ErrInvalidEvidence)
		}
	case ProviderAttestationIdentityMismatch,
		ProviderHostAttestationFailed, ProviderHostIdentityMismatch:
		observed := llm.ObservedProviderIdentity{
			Attestation: proof.Attestation, Observation: proof.Observation, Evidence: evidence,
		}
		if observed.ValidateFor(llm.ProviderIdentityObservationRequest{
			Expectation: expected, ChallengeSHA256: challenge,
		}) != nil {
			return fmt.Errorf("%w: host failure lacks exact provider evidence", ErrInvalidEvidence)
		}
	default:
		return fmt.Errorf("%w: provider identity failure code is not registered", ErrInvalidEvidence)
	}
	return nil
}

func providerIdentityFailureProofBounded(proof providerIdentityFailureProof) bool {
	remaining := providerIdentityFailureMetadataBytes
	return consumeBoundedMetadata(reflect.ValueOf(proof), &remaining, 0)
}

func consumeBoundedMetadata(value reflect.Value, remaining *int, depth int) bool {
	if !value.IsValid() || depth > 16 {
		return depth <= 16
	}
	for value.Kind() == reflect.Interface || value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return true
		}
		value = value.Elem()
		depth++
		if depth > 16 {
			return false
		}
	}
	switch value.Kind() {
	case reflect.String:
		return consumeMetadataBytes(value.Len(), remaining)
	case reflect.Slice:
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return consumeMetadataBytes(value.Len(), remaining)
		}
		fallthrough
	case reflect.Array:
		for index := 0; index < value.Len(); index++ {
			if !consumeBoundedMetadata(value.Index(index), remaining, depth+1) {
				return false
			}
		}
	case reflect.Struct:
		kind := value.Type()
		for index := 0; index < value.NumField(); index++ {
			if kind.Field(index).PkgPath != "" {
				continue
			}
			if !consumeBoundedMetadata(value.Field(index), remaining, depth+1) {
				return false
			}
		}
	}
	return true
}

func consumeMetadataBytes(size int, remaining *int) bool {
	if size < 0 || size > *remaining {
		return false
	}
	*remaining -= size
	return true
}
