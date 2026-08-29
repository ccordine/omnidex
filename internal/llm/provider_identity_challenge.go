package llm

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/gryph/omnidex/internal/exactjson"
)

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
