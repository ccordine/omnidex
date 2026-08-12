package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

func TestPostgresPolicyObservationMustProjectEveryRawIdentityOperation(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	mutations := map[string]func(*llm.ProviderIdentityObservation){
		"version_body": func(value *llm.ProviderIdentityObservation) {
			value.VersionBodySHA256 = strings.Repeat("1", 64)
		},
		"installed_body": func(value *llm.ProviderIdentityObservation) {
			value.InstalledBodySHA256 = strings.Repeat("2", 64)
		},
		"tokenizer_request": func(value *llm.ProviderIdentityObservation) {
			value.TokenizerRequestSHA256 = strings.Repeat("3", 64)
		},
		"tokenizer_body": func(value *llm.ProviderIdentityObservation) {
			value.TokenizerBodySHA256 = strings.Repeat("4", 64)
		},
		"preload_method": func(value *llm.ProviderIdentityObservation) {
			value.PreloadMethod = "GET"
		},
		"preload_endpoint": func(value *llm.ProviderIdentityObservation) {
			value.PreloadEndpoint = "/forged"
		},
		"preload_request": func(value *llm.ProviderIdentityObservation) {
			value.PreloadRequestSHA256 = strings.Repeat("5", 64)
		},
		"preload_body": func(value *llm.ProviderIdentityObservation) {
			value.PreloadBodySHA256 = strings.Repeat("6", 64)
		},
		"runner_body": func(value *llm.ProviderIdentityObservation) {
			value.RunnerBodySHA256 = strings.Repeat("7", 64)
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := startTaskGenerationRetirementFixtureIn(
				t, repository, pool, ctx, "policy-observation-raw-"+name,
			)
			journal := captureAcceptedPolicyResult(t, fixture)
			forged := journal.result
			mutate(&forged.ProviderObservation)
			forged.ProviderObservation.ObservationSHA256 =
				queueProviderObservationSHA256(t, forged.ProviderObservation)
			if err := forged.Validate(journal.attempt); err != nil {
				t.Fatalf("normalized forged observation should remain self-consistent: %v", err)
			}
			if err := journal.evidence.ValidateFor(journal.attempt, forged); err == nil {
				t.Fatal("Go accepted an observation that differs from raw identity evidence")
			}
			if err := commitDirectPolicyResult(
				fixture, journal.attempt, forged, journal.evidence,
			); err == nil {
				t.Fatal("direct SQL accepted an observation that differs from raw identity evidence")
			}
		})
	}
}

func queueProviderObservationSHA256(
	t *testing.T,
	observation llm.ProviderIdentityObservation,
) string {
	t.Helper()
	observation.ObservationSHA256 = ""
	raw, err := exactjson.Canonical(observation)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	return hex.EncodeToString(digest[:])
}
