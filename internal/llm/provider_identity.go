package llm

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/exactjson"
)

const ProviderIdentityAttestationSchemaV2 = "omnidex.provider-identity-attestation.v2"

var providerIdentityDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ProviderIdentityExpectation struct {
	Backend            string `json:"backend"`
	BackendVersion     string `json:"backend_version"`
	Model              string `json:"model"`
	Digest             string `json:"digest"`
	Quantization       string `json:"quantization"`
	NativeContextLimit int    `json:"native_context_limit"`
	TokenizerProfile   string `json:"tokenizer_profile"`
}

type ProviderIdentityAttestation struct {
	Schema             string `json:"schema"`
	Backend            string `json:"backend"`
	BackendVersion     string `json:"backend_version"`
	Model              string `json:"model"`
	Digest             string `json:"digest"`
	Quantization       string `json:"quantization"`
	NativeContextLimit int    `json:"native_context_limit"`
	TokenizerProfile   string `json:"tokenizer_profile"`
	BackendEvidence    string `json:"backend_evidence"`
	InstalledEvidence  string `json:"installed_evidence"`
	RunnerEvidence     string `json:"runner_evidence"`
	AttestationSHA256  string `json:"attestation_sha256"`
}

func (attestation ProviderIdentityAttestation) Validate() error {
	return attestation.ValidateFor(ProviderIdentityExpectation{
		Backend: attestation.Backend, BackendVersion: attestation.BackendVersion,
		Model: attestation.Model, Digest: attestation.Digest,
		Quantization:       attestation.Quantization,
		NativeContextLimit: attestation.NativeContextLimit,
		TokenizerProfile:   attestation.TokenizerProfile,
	})
}

func (expected ProviderIdentityExpectation) Validate() error {
	for label, value := range map[string]string{
		"backend": expected.Backend, "backend version": expected.BackendVersion,
		"model": expected.Model, "quantization": expected.Quantization,
		"tokenizer profile": expected.TokenizerProfile,
	} {
		if !providerIdentityText(value, 256) {
			return fmt.Errorf("provider identity %s is not exact bounded text", label)
		}
	}
	if !providerIdentityDigest.MatchString(expected.Digest) {
		return fmt.Errorf("provider identity digest must be lowercase SHA-256")
	}
	if err := ValidateInferenceContextTokens(expected.NativeContextLimit); err != nil {
		return fmt.Errorf("provider identity native context is outside registered bounds")
	}
	return nil
}

func NewProviderIdentityAttestation(
	expected ProviderIdentityExpectation,
	backendEvidence string,
	installedEvidence string,
	runnerEvidence string,
) (ProviderIdentityAttestation, error) {
	if err := expected.Validate(); err != nil {
		return ProviderIdentityAttestation{}, err
	}
	attestation := ProviderIdentityAttestation{
		Schema:  ProviderIdentityAttestationSchemaV2,
		Backend: expected.Backend, BackendVersion: expected.BackendVersion,
		Model: expected.Model, Digest: expected.Digest, Quantization: expected.Quantization,
		NativeContextLimit: expected.NativeContextLimit, TokenizerProfile: expected.TokenizerProfile,
		BackendEvidence: backendEvidence, InstalledEvidence: installedEvidence,
		RunnerEvidence: runnerEvidence,
	}
	attestation.AttestationSHA256 = providerAttestationSHA256(attestation)
	if err := attestation.ValidateFor(expected); err != nil {
		return ProviderIdentityAttestation{}, err
	}
	return attestation, nil
}

func (attestation ProviderIdentityAttestation) ValidateFor(
	expected ProviderIdentityExpectation,
) error {
	if err := expected.Validate(); err != nil {
		return err
	}
	if attestation.Schema != ProviderIdentityAttestationSchemaV2 ||
		attestation.Backend != expected.Backend ||
		attestation.BackendVersion != expected.BackendVersion ||
		attestation.Model != expected.Model || attestation.Digest != expected.Digest ||
		attestation.Quantization != expected.Quantization ||
		attestation.NativeContextLimit != expected.NativeContextLimit ||
		attestation.TokenizerProfile != expected.TokenizerProfile {
		return fmt.Errorf("provider identity attestation differs from the frozen expectation")
	}
	for label, value := range map[string]string{
		"backend evidence":   attestation.BackendEvidence,
		"installed evidence": attestation.InstalledEvidence,
		"runner evidence":    attestation.RunnerEvidence,
	} {
		if !providerIdentityText(value, 256) {
			return fmt.Errorf("provider identity %s is not exact bounded text", label)
		}
	}
	if !providerIdentityDigest.MatchString(attestation.AttestationSHA256) ||
		attestation.AttestationSHA256 != providerAttestationSHA256(attestation) {
		return fmt.Errorf("provider identity attestation hash is invalid")
	}
	return nil
}

func providerAttestationSHA256(attestation ProviderIdentityAttestation) string {
	copy := attestation
	copy.AttestationSHA256 = ""
	raw, err := exactjson.Canonical(copy)
	if err != nil {
		panic(fmt.Sprintf("marshal provider identity attestation: %v", err))
	}
	return providerBodySHA256(raw)
}

func providerIdentityText(value string, maxBytes int) bool {
	return value != "" && len(value) <= maxBytes && value == strings.TrimSpace(value) &&
		utf8.ValidString(value) && !strings.ContainsRune(value, 0) &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}
