package llm

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/exactjson"
)

const ProviderIdentityObservationSchemaV2 = "omnidex.provider-identity-observation.v2"

type ProviderIdentityObservation struct {
	Schema                 string                      `json:"schema"`
	ObservedAt             time.Time                   `json:"observed_at"`
	AttestationSHA256      string                      `json:"attestation_sha256"`
	VersionBodySHA256      string                      `json:"version_body_sha256"`
	InstalledBodySHA256    string                      `json:"installed_body_sha256"`
	TokenizerRequestSHA256 string                      `json:"tokenizer_request_sha256"`
	TokenizerBodySHA256    string                      `json:"tokenizer_body_sha256"`
	PreloadBodySHA256      string                      `json:"preload_body_sha256"`
	RunnerBodySHA256       string                      `json:"runner_body_sha256"`
	PreloadMethod          string                      `json:"preload_method"`
	PreloadEndpoint        string                      `json:"preload_endpoint"`
	PreloadRequestSHA256   string                      `json:"preload_request_sha256"`
	ChallengeSHA256        string                      `json:"challenge_sha256"`
	Evidence               ProviderIdentityEvidenceRef `json:"evidence"`
	ObservationSHA256      string                      `json:"observation_sha256"`
}

type ProviderIdentityObservationRequest struct {
	Expectation     ProviderIdentityExpectation `json:"expectation"`
	ChallengeSHA256 string                      `json:"challenge_sha256"`
}

type ObservedProviderIdentity struct {
	Attestation ProviderIdentityAttestation `json:"attestation"`
	Observation ProviderIdentityObservation `json:"observation"`
	Evidence    ProviderIdentityEvidence    `json:"evidence"`
}

type ProviderIdentityObserver interface {
	ObserveProviderIdentity(
		context.Context,
		ProviderIdentityObservationRequest,
	) (ObservedProviderIdentity, error)
}

func RequireProviderIdentityObservation(
	ctx context.Context,
	client Client,
	request ProviderIdentityObservationRequest,
) (ObservedProviderIdentity, error) {
	if ctx == nil || client == nil {
		return ObservedProviderIdentity{}, fmt.Errorf(
			"provider identity observation requires context and client",
		)
	}
	if err := request.Validate(); err != nil {
		return ObservedProviderIdentity{}, err
	}
	observer, ok := client.(ProviderIdentityObserver)
	if !ok {
		return ObservedProviderIdentity{}, fmt.Errorf(
			"configured generation provider cannot observe its live identity",
		)
	}
	observed, err := observer.ObserveProviderIdentity(ctx, request)
	if err != nil {
		selection := ProviderIdentitySelection{
			Model:              request.Expectation.Model,
			NativeContextLimit: request.Expectation.NativeContextLimit,
		}
		if evidenceErr := observed.Evidence.ValidateFailure(
			selection, &request.Expectation,
		); evidenceErr != nil {
			return observed, fmt.Errorf(
				"provider identity failure is not proven by exact request-scoped evidence: %v: %w",
				evidenceErr, err,
			)
		}
		return observed, err
	}
	if err := observed.ValidateFor(request); err != nil {
		return observed, err
	}
	return observed, nil
}

func NewObservedProviderIdentity(
	observedAt time.Time,
	attestation ProviderIdentityAttestation,
	evidence ProviderIdentityEvidence,
	challengeSHA256 string,
) (ObservedProviderIdentity, error) {
	if err := attestation.Validate(); err != nil {
		return ObservedProviderIdentity{}, err
	}
	if observedAt.IsZero() || observedAt.Location() != time.UTC {
		return ObservedProviderIdentity{}, fmt.Errorf("provider identity observation time must be nonzero UTC")
	}
	selection := ProviderIdentitySelection{
		Model: attestation.Model, NativeContextLimit: attestation.NativeContextLimit,
	}
	expected, err := DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil || attestation.ValidateFor(expected) != nil {
		return ObservedProviderIdentity{}, fmt.Errorf("provider identity raw evidence differs from its attestation")
	}
	operations := evidence.Operations
	value := ProviderIdentityObservation{
		Schema: ProviderIdentityObservationSchemaV2, ObservedAt: observedAt,
		AttestationSHA256:      attestation.AttestationSHA256,
		VersionBodySHA256:      operations[0].ResponseSHA256,
		InstalledBodySHA256:    operations[1].ResponseSHA256,
		TokenizerRequestSHA256: operations[2].RequestSHA256,
		TokenizerBodySHA256:    operations[2].ResponseSHA256,
		PreloadBodySHA256:      operations[3].ResponseSHA256,
		RunnerBodySHA256:       operations[4].ResponseSHA256,
		PreloadMethod:          operations[3].Method, PreloadEndpoint: operations[3].Endpoint,
		PreloadRequestSHA256: operations[3].RequestSHA256,
		ChallengeSHA256:      challengeSHA256,
		Evidence:             evidence.Ref,
	}
	value.ObservationSHA256 = providerObservationSHA256(value)
	observed := ObservedProviderIdentity{Attestation: attestation, Observation: value, Evidence: evidence}
	return observed, observed.ValidateFor(ProviderIdentityObservationRequest{
		Expectation: expected, ChallengeSHA256: challengeSHA256,
	})
}

func (observation ProviderIdentityObservation) Validate() error {
	if observation.Schema != ProviderIdentityObservationSchemaV2 ||
		observation.ObservedAt.IsZero() || observation.ObservedAt.Location() != time.UTC {
		return fmt.Errorf("provider identity observation authority is invalid")
	}
	for _, digest := range []string{
		observation.AttestationSHA256, observation.VersionBodySHA256,
		observation.InstalledBodySHA256, observation.TokenizerRequestSHA256,
		observation.TokenizerBodySHA256, observation.PreloadBodySHA256,
		observation.RunnerBodySHA256, observation.PreloadRequestSHA256,
		observation.ChallengeSHA256,
		observation.ObservationSHA256,
	} {
		if !providerIdentityDigest.MatchString(digest) {
			return fmt.Errorf("provider identity observation contains an invalid digest")
		}
	}
	if !providerIdentityText(observation.PreloadMethod, 16) ||
		!providerIdentityText(observation.PreloadEndpoint, 256) ||
		observation.Evidence.Schema != ProviderIdentityEvidenceRefSchemaV1 ||
		observation.Evidence.ID != "provider_identity_"+observation.Evidence.SHA256 ||
		!providerIdentityDigest.MatchString(observation.Evidence.SHA256) ||
		observation.Evidence.Bytes < 1 || observation.Evidence.Bytes > MaxProviderIdentityEvidenceBytes {
		return fmt.Errorf("provider identity preload operation is invalid")
	}
	if observation.ObservationSHA256 != providerObservationSHA256(observation) {
		return fmt.Errorf("provider identity observation hash changed")
	}
	return nil
}

func (observation ProviderIdentityObservation) ValidateFor(
	attestation ProviderIdentityAttestation,
	challengeSHA256 string,
) error {
	if err := attestation.Validate(); err != nil {
		return err
	}
	if err := observation.Validate(); err != nil {
		return err
	}
	if observation.AttestationSHA256 != attestation.AttestationSHA256 ||
		observation.ChallengeSHA256 != challengeSHA256 {
		return fmt.Errorf("provider identity observation differs from its attestation")
	}
	return nil
}

func (observation ProviderIdentityObservation) ValidateEvidence(
	evidence ProviderIdentityEvidence,
) error {
	if err := evidence.Validate(); err != nil || !evidence.Successful() ||
		observation.Evidence != evidence.Ref {
		return fmt.Errorf("provider identity observation evidence is invalid")
	}
	operations := evidence.Operations
	if observation.VersionBodySHA256 != operations[0].ResponseSHA256 ||
		observation.InstalledBodySHA256 != operations[1].ResponseSHA256 ||
		observation.TokenizerRequestSHA256 != operations[2].RequestSHA256 ||
		observation.TokenizerBodySHA256 != operations[2].ResponseSHA256 ||
		observation.PreloadMethod != operations[3].Method ||
		observation.PreloadEndpoint != operations[3].Endpoint ||
		observation.PreloadRequestSHA256 != operations[3].RequestSHA256 ||
		observation.PreloadBodySHA256 != operations[3].ResponseSHA256 ||
		observation.RunnerBodySHA256 != operations[4].ResponseSHA256 {
		return fmt.Errorf("provider identity observation differs from its raw evidence")
	}
	return nil
}

func (observed ObservedProviderIdentity) ValidateFor(
	request ProviderIdentityObservationRequest,
) error {
	if err := request.Validate(); err != nil {
		return err
	}
	if err := observed.Attestation.ValidateFor(request.Expectation); err != nil {
		return err
	}
	if err := observed.Observation.ValidateEvidence(observed.Evidence); err != nil {
		return fmt.Errorf("provider identity observation lacks its exact raw evidence")
	}
	selection := ProviderIdentitySelection{
		Model:              request.Expectation.Model,
		NativeContextLimit: request.Expectation.NativeContextLimit,
	}
	if err := observed.Evidence.ValidateRequests(selection); err != nil {
		return fmt.Errorf("provider identity raw evidence changed its exact requests: %w", err)
	}
	derived, err := DeriveExactProviderIdentityExpectation(observed.Evidence, ProviderIdentitySelection{
		Model: request.Expectation.Model, NativeContextLimit: request.Expectation.NativeContextLimit,
	})
	if err != nil || derived != request.Expectation {
		return fmt.Errorf("provider identity raw evidence differs from its frozen expectation")
	}
	return observed.Observation.ValidateFor(observed.Attestation, request.ChallengeSHA256)
}

func (request ProviderIdentityObservationRequest) Validate() error {
	if err := request.Expectation.Validate(); err != nil {
		return err
	}
	if !providerIdentityDigest.MatchString(request.ChallengeSHA256) {
		return fmt.Errorf("provider identity observation challenge is invalid")
	}
	return nil
}

func DeriveProviderIdentityObservationChallenge(
	scope string,
	expected ProviderIdentityExpectation,
) (string, error) {
	if !providerIdentityText(scope, 512) {
		return "", fmt.Errorf("provider identity observation challenge scope is invalid")
	}
	if err := expected.Validate(); err != nil {
		return "", err
	}
	raw, err := exactjson.Canonical(struct {
		Scope       string                      `json:"scope"`
		Expectation ProviderIdentityExpectation `json:"expectation"`
	}{scope, expected})
	if err != nil {
		return "", err
	}
	return providerBodySHA256(raw), nil
}

func DeriveProviderIdentityDiscoveryChallenge(
	scope string,
	selection ProviderIdentitySelection,
) (string, error) {
	if !providerIdentityText(scope, 512) {
		return "", fmt.Errorf("provider identity discovery challenge scope is invalid")
	}
	if err := selection.Validate(); err != nil {
		return "", err
	}
	raw, err := exactjson.Canonical(struct {
		Scope     string                    `json:"scope"`
		Selection ProviderIdentitySelection `json:"selection"`
	}{scope, selection})
	if err != nil {
		return "", err
	}
	return providerBodySHA256(raw), nil
}

func providerBodySHA256(raw []byte) string {
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}

func providerObservationSHA256(observation ProviderIdentityObservation) string {
	copy := observation
	copy.ObservationSHA256 = ""
	raw, err := exactjson.Canonical(copy)
	if err != nil {
		panic(fmt.Sprintf("marshal provider identity observation: %v", err))
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
