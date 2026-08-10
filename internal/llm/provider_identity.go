package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

const ProviderIdentityAttestationSchemaV1 = "omnidex.provider-identity-attestation.v1"

var providerIdentityDigest = regexp.MustCompile(`^[0-9a-f]{64}$`)

type ProviderIdentityExpectation struct {
	Backend            string `json:"backend"`
	BackendVersion     string `json:"backend_version"`
	Model              string `json:"model"`
	Digest             string `json:"digest"`
	Quantization       string `json:"quantization"`
	NativeContextLimit int    `json:"native_context_limit"`
}

type ProviderIdentityAttestation struct {
	Schema             string `json:"schema"`
	Backend            string `json:"backend"`
	BackendVersion     string `json:"backend_version"`
	Model              string `json:"model"`
	Digest             string `json:"digest"`
	Quantization       string `json:"quantization"`
	NativeContextLimit int    `json:"native_context_limit"`
	BackendEvidence    string `json:"backend_evidence"`
	InstalledEvidence  string `json:"installed_evidence"`
	RunnerEvidence     string `json:"runner_evidence"`
	AttestationSHA256  string `json:"attestation_sha256"`
}

type ProviderIdentityAttestor interface {
	AttestProviderIdentity(
		context.Context,
		ProviderIdentityExpectation,
	) (ProviderIdentityAttestation, error)
}

func (attestation ProviderIdentityAttestation) Validate() error {
	return attestation.ValidateFor(ProviderIdentityExpectation{
		Backend: attestation.Backend, BackendVersion: attestation.BackendVersion,
		Model: attestation.Model, Digest: attestation.Digest,
		Quantization:       attestation.Quantization,
		NativeContextLimit: attestation.NativeContextLimit,
	})
}

func (expected ProviderIdentityExpectation) Validate() error {
	for label, value := range map[string]string{
		"backend": expected.Backend, "backend version": expected.BackendVersion,
		"model": expected.Model, "quantization": expected.Quantization,
	} {
		if !providerIdentityText(value, 256) {
			return fmt.Errorf("provider identity %s is not exact bounded text", label)
		}
	}
	if !providerIdentityDigest.MatchString(expected.Digest) {
		return fmt.Errorf("provider identity digest must be lowercase SHA-256")
	}
	if expected.NativeContextLimit < MinInferenceContextTokens ||
		expected.NativeContextLimit > MaxInferenceContextTokens {
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
		Schema:  ProviderIdentityAttestationSchemaV1,
		Backend: expected.Backend, BackendVersion: expected.BackendVersion,
		Model: expected.Model, Digest: expected.Digest, Quantization: expected.Quantization,
		NativeContextLimit: expected.NativeContextLimit,
		BackendEvidence:    backendEvidence, InstalledEvidence: installedEvidence,
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
	if attestation.Schema != ProviderIdentityAttestationSchemaV1 ||
		attestation.Backend != expected.Backend ||
		attestation.BackendVersion != expected.BackendVersion ||
		attestation.Model != expected.Model || attestation.Digest != expected.Digest ||
		attestation.Quantization != expected.Quantization ||
		attestation.NativeContextLimit != expected.NativeContextLimit {
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

func RequireProviderIdentityAttestation(
	ctx context.Context,
	client Client,
	expected ProviderIdentityExpectation,
) (ProviderIdentityAttestation, error) {
	if ctx == nil || client == nil {
		return ProviderIdentityAttestation{}, fmt.Errorf("provider identity attestation requires context and client")
	}
	attestor, ok := client.(ProviderIdentityAttestor)
	if !ok {
		return ProviderIdentityAttestation{}, fmt.Errorf("configured generation provider cannot attest its live identity")
	}
	attestation, err := attestor.AttestProviderIdentity(ctx, expected)
	if err != nil {
		return ProviderIdentityAttestation{}, err
	}
	if err := attestation.ValidateFor(expected); err != nil {
		return ProviderIdentityAttestation{}, err
	}
	return attestation, nil
}

func providerAttestationSHA256(attestation ProviderIdentityAttestation) string {
	copy := attestation
	copy.AttestationSHA256 = ""
	raw, err := json.Marshal(copy)
	if err != nil {
		panic(fmt.Sprintf("marshal provider identity attestation: %v", err))
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func providerIdentityText(value string, maxBytes int) bool {
	return value != "" && len(value) <= maxBytes && value == strings.TrimSpace(value) &&
		utf8.ValidString(value) && !strings.ContainsRune(value, 0) &&
		strings.IndexFunc(value, unicode.IsControl) < 0
}
