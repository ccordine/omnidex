package cognitionpolicy

import (
	"fmt"

	"github.com/gryph/omnidex/internal/llm"
)

const HardwareProvenanceConfiguredAuthority = "configured_authority"

type BrainRef struct {
	Model               string           `json:"model"`
	Digest              string           `json:"digest"`
	Quantization        string           `json:"quantization"`
	SamplingSHA256      string           `json:"sampling_sha256"`
	Sampling            SamplingIdentity `json:"sampling"`
	NativeContextLimit  int              `json:"native_context_limit"`
	ContextCeilingBytes int              `json:"context_ceiling_bytes"`
	Backend             string           `json:"backend"`
	BackendVersion      string           `json:"backend_version"`
	Hardware            string           `json:"hardware"`
	HardwareProvenance  string           `json:"hardware_provenance"`
}

func (brain BrainRef) Validate() error {
	if !validExactName(brain.Model, MaxModelNameBytes) {
		return fmt.Errorf("%w: model must be one bounded whitespace-free value", ErrInvalidBrain)
	}
	for label, value := range map[string]string{
		"quantization": brain.Quantization, "backend": brain.Backend,
		"backend version": brain.BackendVersion, "hardware": brain.Hardware,
	} {
		if !validExactText(value, MaxBrainIdentityBytes) {
			return fmt.Errorf("%w: %s must be one exact bounded value", ErrInvalidBrain, label)
		}
	}
	if brain.HardwareProvenance != HardwareProvenanceConfiguredAuthority {
		return fmt.Errorf("%w: hardware must declare configured-authority provenance", ErrInvalidBrain)
	}
	if !validPolicySHA256(brain.Digest) || !validPolicySHA256(brain.SamplingSHA256) {
		return fmt.Errorf("%w: model and sampling hashes must be lowercase SHA-256", ErrInvalidBrain)
	}
	if brain.NativeContextLimit <= 0 || brain.NativeContextLimit > MaxNativeContextLimit {
		return fmt.Errorf("%w: native context limit is outside registered bounds", ErrInvalidBrain)
	}
	if brain.ContextCeilingBytes <= 0 || brain.ContextCeilingBytes > MaxContextCeilingBytes {
		return fmt.Errorf("%w: context byte ceiling is outside registered bounds", ErrInvalidBrain)
	}
	if err := brain.Sampling.Validate(); err != nil {
		return err
	}
	samplingSHA, err := brain.Sampling.SHA256()
	if err != nil || samplingSHA != brain.SamplingSHA256 ||
		brain.Sampling.NativeContextLimit != brain.NativeContextLimit ||
		brain.Sampling.ContextCeilingBytes != brain.ContextCeilingBytes {
		return fmt.Errorf("%w: sampling identity does not match the frozen brain", ErrInvalidBrain)
	}
	return nil
}

func NewBrainRef(
	model string,
	digest string,
	quantization string,
	backend string,
	backendVersion string,
	hardware string,
	sampling SamplingIdentity,
) (BrainRef, error) {
	samplingSHA, err := sampling.SHA256()
	if err != nil {
		return BrainRef{}, err
	}
	brain := BrainRef{
		Model: model, Digest: digest, Quantization: quantization,
		SamplingSHA256: samplingSHA, Sampling: sampling,
		NativeContextLimit:  sampling.NativeContextLimit,
		ContextCeilingBytes: sampling.ContextCeilingBytes,
		Backend:             backend, BackendVersion: backendVersion, Hardware: hardware,
		HardwareProvenance: HardwareProvenanceConfiguredAuthority,
	}
	if err := brain.Validate(); err != nil {
		return BrainRef{}, err
	}
	return brain, nil
}

func (brain BrainRef) ProviderExpectation() (llm.ProviderIdentityExpectation, error) {
	if err := brain.Validate(); err != nil {
		return llm.ProviderIdentityExpectation{}, err
	}
	expected := llm.ProviderIdentityExpectation{
		Backend: brain.Backend, BackendVersion: brain.BackendVersion,
		Model: brain.Model, Digest: brain.Digest, Quantization: brain.Quantization,
		NativeContextLimit: brain.NativeContextLimit,
	}
	if err := expected.Validate(); err != nil {
		return llm.ProviderIdentityExpectation{}, fmt.Errorf("%w: %v", ErrInvalidBrain, err)
	}
	return expected, nil
}
