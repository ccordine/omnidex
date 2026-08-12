package queue

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
)

func TestPostgresProviderIdentityDerivationRejectsGoIncompatibleJSONTypes(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	mutations := map[string]providerIdentityBodyMutation{
		"preload_done_string": {3, func(raw []byte) []byte {
			return []byte(`{"done":"true"}`)
		}},
		"installed_size_string":       modelFieldMutation(1, "size", "1"),
		"installed_size_vram_string":  modelFieldMutation(1, "size_vram", "1"),
		"installed_context_string":    modelFieldMutation(1, "context_length", "32768"),
		"installed_modified_boolean":  modelFieldMutation(1, "modified_at", false),
		"installed_modified_bad_time": modelFieldMutation(1, "modified_at", "not-a-time"),
		"installed_expires_number":    modelFieldMutation(1, "expires_at", 1),
		"installed_name_number":       modelFieldMutation(1, "name", 1),
		"installed_model_boolean":     modelFieldMutation(1, "model", false),
		"installed_digest_number":     modelFieldMutation(1, "digest", 1),
		"details_parent_number":       modelDetailMutation(1, "parent_model", 1),
		"details_format_boolean":      modelDetailMutation(1, "format", false),
		"details_family_number":       modelDetailMutation(1, "family", 1),
		"details_families_number":     modelDetailMutation(1, "families", 1),
		"details_family_item_number":  modelDetailMutation(1, "families", []any{1}),
		"details_parameter_boolean":   modelDetailMutation(1, "parameter_size", false),
		"details_quantization_number": modelDetailMutation(1, "quantization_level", 1),
		"runner_context_string":       modelFieldMutation(4, "context_length", "32768"),
	}
	for name, mutation := range mutations {
		t.Run(name, func(t *testing.T) {
			fixture := startTaskGenerationRetirementFixtureIn(
				t, repository, pool, ctx, "provider-identity-types-"+name,
			)
			journal := captureAcceptedPolicyResult(t, fixture)
			changed := mutateProviderIdentityEvidence(t, journal.evidence.ProviderIdentity, mutation)
			selection := llm.ProviderIdentitySelection{
				Model:              journal.attempt.Brain.Model,
				NativeContextLimit: journal.attempt.Brain.NativeContextLimit,
			}
			if _, err := llm.DeriveExactProviderIdentityExpectation(changed, selection); err == nil {
				t.Fatal("Go accepted provider identity JSON with an incompatible field type")
			}
			if providerIdentityMatchesAttemptInPostgres(t, fixture, journal.attempt, changed) {
				t.Fatal("PostgreSQL accepted provider identity JSON that Go rejects")
			}
		})
	}
}

func TestPostgresProviderIdentityDerivationAcceptsGoCompatibleOptionalNullsAndTimes(t *testing.T) {
	repository, pool, ctx := policyInputFreshRepository(t)
	for name, value := range map[string]any{
		"nulls":             nil,
		"single_hour":       "2026-08-09T1:02:03Z",
		"comma_fraction":    "2026-08-09T01:02:03,12345678901Z",
		"legacy_max_offset": "2026-08-09T01:02:03+24:60",
	} {
		t.Run(name, func(t *testing.T) {
			fixture := startTaskGenerationRetirementFixtureIn(
				t, repository, pool, ctx, "provider-identity-compatible-"+name,
			)
			journal := captureAcceptedPolicyResult(t, fixture)
			changed := mutateProviderIdentityEvidence(t, journal.evidence.ProviderIdentity,
				modelFieldMutation(1, "modified_at", value))
			selection := llm.ProviderIdentitySelection{
				Model:              journal.attempt.Brain.Model,
				NativeContextLimit: journal.attempt.Brain.NativeContextLimit,
			}
			if _, err := llm.DeriveExactProviderIdentityExpectation(changed, selection); err != nil {
				t.Fatalf("Go rejected its compatible optional timestamp: %v", err)
			}
			if !providerIdentityMatchesAttemptInPostgres(t, fixture, journal.attempt, changed) {
				t.Fatal("PostgreSQL rejected provider identity JSON accepted by Go")
			}
		})
	}
}

type providerIdentityBodyMutation struct {
	operation int
	mutate    func([]byte) []byte
}

func modelFieldMutation(operation int, field string, value any) providerIdentityBodyMutation {
	return providerIdentityBodyMutation{operation, func(raw []byte) []byte {
		return appendUnselectedProviderModel(raw, field, value, false)
	}}
}

func modelDetailMutation(operation int, field string, value any) providerIdentityBodyMutation {
	return providerIdentityBodyMutation{operation, func(raw []byte) []byte {
		return appendUnselectedProviderModel(raw, field, value, true)
	}}
}

func appendUnselectedProviderModel(raw []byte, field string, value any, detail bool) []byte {
	document := map[string]any{}
	if json.Unmarshal(raw, &document) != nil {
		return raw
	}
	model := map[string]any{
		"name": "unselected:model", "model": "unselected:model", "size": 1,
		"digest":  strings.Repeat("b", 64),
		"details": map[string]any{"quantization_level": "Q4_K_M"},
	}
	if detail {
		model["details"].(map[string]any)[field] = value
	} else {
		model[field] = value
	}
	document["models"] = append(document["models"].([]any), model)
	encoded, err := json.Marshal(document)
	if err != nil {
		return raw
	}
	return encoded
}

func mutateProviderIdentityEvidence(
	t *testing.T,
	evidence llm.ProviderIdentityEvidence,
	mutation providerIdentityBodyMutation,
) llm.ProviderIdentityEvidence {
	t.Helper()
	operations := evidence.Clone().Operations
	operation := operations[mutation.operation]
	changedBody := mutation.mutate(append([]byte(nil), operation.ResponseCapture...))
	if string(changedBody) == string(operation.ResponseCapture) {
		t.Fatal("provider identity test mutation did not change the raw response")
	}
	changed, err := llm.NewProviderIdentityOperationEvidence(
		operation.Operation, operation.Method, operation.Endpoint,
		operation.RequestDisposition, operation.Request, operation.HTTPStatus,
		operation.Disposition, operation.ResponseComplete, operation.ContentEncoding, changedBody,
	)
	if err != nil {
		t.Fatal(err)
	}
	operations[mutation.operation] = changed
	result, err := llm.NewProviderIdentityEvidence(operations)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func providerIdentityMatchesAttemptInPostgres(
	t *testing.T,
	fixture taskGenerationRetirementFixture,
	attempt cognitionpolicy.CallAttempt,
	evidence llm.ProviderIdentityEvidence,
) bool {
	t.Helper()
	tx, err := fixture.Pool.Begin(fixture.Context)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if err := insertCognitionProviderIdentityEvidenceBodyTx(
		fixture.Context, tx, evidence,
	); err != nil {
		t.Fatal(err)
	}
	attemptJSON, err := exactjson.Canonical(attempt)
	if err != nil {
		t.Fatal(err)
	}
	var matches bool
	if err := tx.QueryRow(fixture.Context,
		`SELECT cognition_provider_identity_evidence_matches_attempt($1,$2::jsonb)`,
		evidence.Ref.ID, string(attemptJSON),
	).Scan(&matches); err != nil {
		t.Fatal(err)
	}
	return matches
}
