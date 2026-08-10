package cognitiongauntlet

import (
	"fmt"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/llm"
)

const RatGenerationSchemaV1 = "omnidex.rat-doctrine-generation.v1"

type BrainFingerprint struct {
	Model                   string                                  `json:"model"`
	Digest                  string                                  `json:"digest"`
	Quantization            string                                  `json:"quantization"`
	SamplingSHA256          string                                  `json:"sampling_sha256"`
	Sampling                cognitionpolicy.SamplingIdentity        `json:"sampling"`
	NativeContextLimit      int                                     `json:"native_context_limit"`
	Backend                 string                                  `json:"backend"`
	BackendVersion          string                                  `json:"backend_version"`
	Hardware                string                                  `json:"hardware"`
	HardwareAuthoritySource string                                  `json:"hardware_authority_source"`
	ProviderAttestation     llm.ProviderIdentityAttestation         `json:"provider_attestation"`
	ProviderObservation     llm.ProviderIdentityObservation         `json:"provider_observation"`
	HostAttestation         cognitionpolicy.HostHardwareAttestation `json:"host_attestation"`
}

type FixedExperiment struct {
	Brain                      BrainFingerprint `json:"brain"`
	ContextCeilingBytes        int              `json:"context_ceiling_bytes"`
	EnvironmentContractVersion string           `json:"environment_contract_version"`
	EvaluatorVersion           string           `json:"evaluator_version"`
	AuthorityPolicyVersion     string           `json:"authority_policy_version"`
	OracleIsolationVersion     string           `json:"oracle_isolation_version"`
}

type RuntimeCandidate struct {
	Version          string `json:"version"`
	SourceSHA256     string `json:"source_sha256"`
	ExecutableSHA256 string `json:"executable_sha256"`
	MigrationsSHA256 string `json:"migrations_sha256"`
}

type RatGeneration struct {
	Schema      string           `json:"schema"`
	ID          string           `json:"id"`
	Fixed       FixedExperiment  `json:"fixed"`
	FixedSHA256 string           `json:"fixed_sha256"`
	Runtime     RuntimeCandidate `json:"runtime"`
}

func NewRatGeneration(id string, fixed FixedExperiment, runtime RuntimeCandidate) (RatGeneration, error) {
	if err := requireExact(id, "rat generation ID", 256); err != nil {
		return RatGeneration{}, err
	}
	if err := fixed.Validate(); err != nil {
		return RatGeneration{}, err
	}
	if err := runtime.Validate(); err != nil {
		return RatGeneration{}, err
	}
	digest, err := digestJSON(fixed)
	if err != nil {
		return RatGeneration{}, fmt.Errorf("hash fixed rat experiment: %w", err)
	}
	generation := RatGeneration{
		Schema: RatGenerationSchemaV1, ID: id, Fixed: fixed,
		FixedSHA256: digest, Runtime: runtime,
	}
	return generation, generation.Validate()
}

func (generation RatGeneration) Validate() error {
	if generation.Schema != RatGenerationSchemaV1 {
		return fmt.Errorf("rat generation schema is invalid")
	}
	if err := requireExact(generation.ID, "rat generation ID", 256); err != nil {
		return err
	}
	if err := generation.Fixed.Validate(); err != nil {
		return err
	}
	if err := generation.Runtime.Validate(); err != nil {
		return err
	}
	expected, err := digestJSON(generation.Fixed)
	if err != nil {
		return fmt.Errorf("hash fixed rat experiment: %w", err)
	}
	if generation.FixedSHA256 != expected {
		return fmt.Errorf("rat generation fixed experiment digest is inconsistent")
	}
	return nil
}

func (fixed FixedExperiment) Validate() error {
	if err := fixed.Brain.Validate(); err != nil {
		return err
	}
	if fixed.Brain.Sampling.ContextCeilingBytes != fixed.ContextCeilingBytes {
		return fmt.Errorf("rat experiment sampling identity changed the fixed context ceiling")
	}
	if fixed.ContextCeilingBytes <= 0 || fixed.ContextCeilingBytes > 64*1024*1024 {
		return fmt.Errorf("rat experiment context ceiling is invalid")
	}
	for label, value := range map[string]string{
		"environment contract version": fixed.EnvironmentContractVersion,
		"evaluator version":            fixed.EvaluatorVersion,
		"authority policy version":     fixed.AuthorityPolicyVersion,
		"oracle isolation version":     fixed.OracleIsolationVersion,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	return nil
}

func (brain BrainFingerprint) Validate() error {
	for label, value := range map[string]string{
		"brain model": brain.Model, "brain quantization": brain.Quantization,
		"brain backend": brain.Backend, "brain backend version": brain.BackendVersion,
		"brain hardware":                  brain.Hardware,
		"brain hardware authority source": brain.HardwareAuthoritySource,
	} {
		if err := requireExact(value, label, 256); err != nil {
			return err
		}
	}
	if !validDigest(brain.Digest) || !validDigest(brain.SamplingSHA256) ||
		brain.NativeContextLimit <= 0 || brain.NativeContextLimit > 10_000_000 {
		return fmt.Errorf("rat experiment brain fingerprint is invalid")
	}
	if err := brain.Sampling.Validate(); err != nil {
		return err
	}
	samplingSHA, err := brain.Sampling.SHA256()
	if err != nil || samplingSHA != brain.SamplingSHA256 ||
		brain.Sampling.NativeContextLimit != brain.NativeContextLimit {
		return fmt.Errorf("rat experiment sampling identity does not match its frozen brain")
	}
	if _, err := brain.attestedBrain(); err != nil {
		return err
	}
	return nil
}

func brainFingerprintFromAttested(brain cognitionpolicy.AttestedBrain) (BrainFingerprint, error) {
	if err := brain.Validate(); err != nil {
		return BrainFingerprint{}, err
	}
	value := BrainFingerprint{
		Model: brain.Ref.Model, Digest: brain.Ref.Digest,
		Quantization: brain.Ref.Quantization, SamplingSHA256: brain.Ref.SamplingSHA256,
		Sampling: brain.Ref.Sampling, NativeContextLimit: brain.Ref.NativeContextLimit,
		Backend: brain.Ref.Backend, BackendVersion: brain.Ref.BackendVersion,
		Hardware:                brain.Ref.Hardware,
		HardwareAuthoritySource: cognitionpolicy.HostHardwareAttestationSchemaV2,
		ProviderAttestation:     brain.Attestation,
		ProviderObservation:     brain.BootstrapObservation,
		HostAttestation:         brain.Host,
	}
	if err := value.Validate(); err != nil {
		return BrainFingerprint{}, err
	}
	return value, nil
}

func (brain BrainFingerprint) attestedBrain() (cognitionpolicy.AttestedBrain, error) {
	if brain.HardwareAuthoritySource != cognitionpolicy.HostHardwareAttestationSchemaV2 ||
		brain.Hardware != "host-attestation:"+brain.HostAttestation.AttestationSHA256 {
		return cognitionpolicy.AttestedBrain{}, fmt.Errorf(
			"rat experiment hardware is not derived from its code-owned host attestation",
		)
	}
	ref, err := cognitionpolicy.NewBrainRef(
		brain.Model, brain.Digest, brain.Quantization, brain.Backend,
		brain.BackendVersion, brain.Hardware, brain.Sampling,
	)
	if err != nil {
		return cognitionpolicy.AttestedBrain{}, err
	}
	if ref.SamplingSHA256 != brain.SamplingSHA256 {
		return cognitionpolicy.AttestedBrain{}, fmt.Errorf("rat experiment sampling hash is inconsistent")
	}
	attested, err := cognitionpolicy.NewAttestedBrain(
		ref, brain.ProviderAttestation, brain.ProviderObservation, brain.HostAttestation,
	)
	if err != nil {
		return cognitionpolicy.AttestedBrain{}, err
	}
	return attested, nil
}

func sameFrozenBrain(
	observed cognitionpolicy.AttestedBrain,
	frozen cognitionpolicy.AttestedBrain,
) bool {
	return observed.Ref == frozen.Ref && observed.Attestation == frozen.Attestation &&
		observed.Host == frozen.Host
}

func (runtime RuntimeCandidate) Validate() error {
	if err := requireExact(runtime.Version, "cognition runtime candidate version", 256); err != nil {
		return err
	}
	if !validDigest(runtime.SourceSHA256) || !validDigest(runtime.ExecutableSHA256) ||
		!validDigest(runtime.MigrationsSHA256) {
		return fmt.Errorf("cognition runtime candidate source, executable, or migration digest is invalid")
	}
	return nil
}

func RequireComparableRatGenerations(left, right RatGeneration) error {
	if err := left.Validate(); err != nil {
		return fmt.Errorf("left rat generation: %w", err)
	}
	if err := right.Validate(); err != nil {
		return fmt.Errorf("right rat generation: %w", err)
	}
	if left.FixedSHA256 != right.FixedSHA256 {
		return fmt.Errorf("rat generations changed the frozen brain or experimental constants")
	}
	if left.Runtime == right.Runtime {
		return fmt.Errorf("rat generations require distinct cognition runtime candidates")
	}
	return nil
}
