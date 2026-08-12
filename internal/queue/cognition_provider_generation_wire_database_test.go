package queue

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPostgresProviderGenerationWireMatchesGoNumericDecoder(t *testing.T) {
	_, pool, ctx := policyInputFreshRepository(t)
	bootstrap := cognitionTestBrainBootstrap()
	evidence, err := cognitionpolicy.NewProviderGenerationEvidence(
		"wire-parity-call",
		llm.PreparedGeneration{
			Schema:                     llm.PreparedGenerationSchemaV1,
			ProviderRequestDisposition: llm.ProviderRequestNotDispatched,
			ProviderObservation:        bootstrap.AttestedBrain.BootstrapObservation,
			ProviderIdentityEvidence:   bootstrap.BootstrapEvidence,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	assertProviderGenerationWireExact(t, pool, ctx, evidence, evidence.Generation, true)
	mutations := map[string]func([]byte) []byte{
		"original_bytes_fraction": replaceWireNumber(`"original_bytes":0`, `"original_bytes":0.5`),
		"original_bytes_exponent": replaceWireNumber(`"original_bytes":0`, `"original_bytes":1e0`),
		"http_status_fraction":    replaceWireNumber(`"provider_http_status":0`, `"provider_http_status":0.5`),
		"http_status_overflow": replaceWireNumber(
			`"provider_http_status":0`, `"provider_http_status":9223372036854775808`,
		),
		"response_bytes_string": replaceWireNumber(
			`"provider_response_bytes":0`, `"provider_response_bytes":"0"`,
		),
		"captured_bytes_null": replaceWireNumber(
			`"provider_response_captured_bytes":0`, `"provider_response_captured_bytes":null`,
		),
		"usage_prompt_fraction": replaceWireNumber(`"prompt_eval_count":0`, `"prompt_eval_count":0.5`),
		"usage_eval_exponent":   replaceWireNumber(`"eval_count":0`, `"eval_count":1e0`),
		"usage_total_overflow": replaceWireNumber(
			`"total_duration_nanos":0`, `"total_duration_nanos":9223372036854775808`,
		),
		"encoding_values_fraction":     replaceWireNumber(`"values":0`, `"values":0.5`),
		"observation_month_normalized": replaceWireNumber(`"observed_month":8`, `"observed_month":13`),
		"observation_nanos_normalized": replaceWireNumber(
			`"observed_nanosecond":0`, `"observed_nanosecond":1000000000`,
		),
		"observation_evidence_fraction": replaceWireNumber(
			fmt.Sprintf(`"evidence_bytes":%d`, bootstrap.BootstrapEvidence.Ref.Bytes),
			`"evidence_bytes":0.5`,
		),
		"identity_operation_count_negative": replaceWireNumber(
			fmt.Sprintf(`"original_operations":%d`, len(bootstrap.BootstrapEvidence.Operations)),
			`"original_operations":-1`,
		),
		"identity_request_fraction": replaceWireNumber(`"request_bytes":0`, `"request_bytes":0.5`),
		"identity_status_string": replaceWireNumber(
			fmt.Sprintf(`"http_status":%d`, bootstrap.BootstrapEvidence.Operations[0].HTTPStatus),
			`"http_status":"200"`,
		),
		"identity_response_overflow": replaceWireNumber(
			fmt.Sprintf(`"response_bytes":%d`, bootstrap.BootstrapEvidence.Operations[0].ResponseBytes),
			`"response_bytes":9223372036854775808`,
		),
		"provider_error_absent_nonempty": mutateAbsentProviderError,
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			forged := mutate(append([]byte(nil), evidence.Generation...))
			if bytes.Equal(forged, evidence.Generation) {
				t.Fatal("test mutation did not change provider generation wire")
			}
			assertProviderGenerationWireExact(t, pool, ctx, evidence, forged, false)
		})
	}
	structuralCases := map[string]struct {
		want   bool
		mutate func(map[string]any)
	}{
		"top_incomplete_invalid_nested_time": {false, func(wire map[string]any) {
			wire["schema"] = providerGenerationWireWitness([]byte(strings.Repeat("x", 4097)), 4096)
			wire["provider_observation"].(map[string]any)["observed_month"] = 13
		}},
		"observation_incomplete_invalid_time": {false, func(wire map[string]any) {
			observation := wire["provider_observation"].(map[string]any)
			observation["schema"] = providerGenerationWireWitness(
				[]byte(strings.Repeat("x", 4097)), 4096,
			)
			observation["observed_month"] = 13
		}},
		"absent_incomplete_provider_error": {true, func(wire map[string]any) {
			wire["provider_error"] = providerGenerationWireWitness(
				[]byte(strings.Repeat("x", 4097)), 4096,
			)
		}},
		"present_incomplete_provider_error": {true, func(wire map[string]any) {
			wire["provider_error_present"] = true
			wire["provider_error"] = providerGenerationWireWitness(
				[]byte(strings.Repeat("x", 4097)), 4096,
			)
		}},
		"present_opaque_provider_error": {true, func(wire map[string]any) {
			wire["provider_error_present"] = true
			wire["provider_error"] = providerGenerationWireWitness([]byte{0xff}, 4096)
		}},
		"opaque_observation_location": {true, func(wire map[string]any) {
			wire["provider_observation"].(map[string]any)["observed_location"] =
				providerGenerationWireWitness([]byte{0xff}, 4096)
		}},
		"positive_odd_second_offset": {true, func(wire map[string]any) {
			mutateProviderObservationOffset(t, wire, 3599, "+00:59")
		}},
		"negative_odd_second_offset": {true, func(wire map[string]any) {
			mutateProviderObservationOffset(t, wire, -3599, "-00:59")
		}},
	}
	for name, testCase := range structuralCases {
		t.Run(name, func(t *testing.T) {
			forged := mutateProviderGenerationDocument(t, evidence.Generation, testCase.mutate)
			assertProviderGenerationWireExact(t, pool, ctx, evidence, forged, testCase.want)
		})
	}
}

func replaceWireNumber(oldValue, newValue string) func([]byte) []byte {
	return func(raw []byte) []byte {
		return bytes.Replace(raw, []byte(oldValue), []byte(newValue), 1)
	}
}

func mutateAbsentProviderError(raw []byte) []byte {
	empty := `"provider_error":{"capture":"","complete":true,"original_bytes":0,` +
		`"original_sha256":"e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"}`
	nonempty := `"provider_error":{"capture":"eA==","complete":true,"original_bytes":1,` +
		`"original_sha256":"2d711642b726b04401627ca9fbac32f5c8530fb1903cc4db02258717921a4881"}`
	return bytes.Replace(raw, []byte(empty), []byte(nonempty), 1)
}

func assertProviderGenerationWireExact(
	t *testing.T,
	pool *pgxpool.Pool,
	ctx context.Context,
	evidence cognitionpolicy.ProviderGenerationEvidence,
	raw []byte,
	want bool,
) {
	t.Helper()
	bound := bindProviderGenerationWire(t, evidence, raw)
	if got := bound.Validate() == nil; got != want {
		t.Fatalf("Go generation wire exact=%v, want %v: %s", got, want, raw)
	}
	var got bool
	if err := pool.QueryRow(ctx,
		`SELECT cognition_provider_generation_wire_is_exact($1::json)`, string(raw),
	).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("generation wire exact=%v, want %v: %s", got, want, raw)
	}
}

func mutateProviderGenerationDocument(
	t *testing.T,
	raw []byte,
	mutate func(map[string]any),
) []byte {
	t.Helper()
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	document := map[string]any{}
	if err := decoder.Decode(&document); err != nil {
		t.Fatal(err)
	}
	mutate(document)
	encoded, err := exactjson.Canonical(document)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func providerGenerationWireWitness(raw []byte, limit int) map[string]any {
	digest := sha256.Sum256(raw)
	capture := raw
	complete := len(raw) <= limit
	if !complete {
		capture = raw[:limit+1]
	}
	return map[string]any{
		"original_bytes": len(raw), "original_sha256": hex.EncodeToString(digest[:]),
		"complete": complete, "capture": base64.StdEncoding.EncodeToString(capture),
	}
}

func mutateProviderObservationOffset(
	t *testing.T,
	wire map[string]any,
	offset int,
	suffix string,
) {
	t.Helper()
	observation := wire["provider_observation"].(map[string]any)
	at := observation["observed_at"].(map[string]any)
	raw, err := base64.StdEncoding.DecodeString(at["capture"].(string))
	if err != nil || !bytes.HasSuffix(raw, []byte("Z")) {
		t.Fatalf("decode provider observation time: %q / %v", raw, err)
	}
	raw = append(append([]byte(nil), raw[:len(raw)-1]...), suffix...)
	observation["observed_at"] = providerGenerationWireWitness(raw, 4096)
	observation["observed_offset_seconds"] = offset
}

func bindProviderGenerationWire(
	t *testing.T,
	evidence cognitionpolicy.ProviderGenerationEvidence,
	raw []byte,
) cognitionpolicy.ProviderGenerationEvidence {
	t.Helper()
	evidence.Generation = append([]byte(nil), raw...)
	digest := sha256.Sum256(raw)
	evidence.Ref.SHA256 = hex.EncodeToString(digest[:])
	evidence.Ref.Bytes = len(raw)
	identityRef := evidence.Ref
	identityRef.ID = ""
	authority, err := exactjson.Canonical(struct {
		CallID string                                        `json:"call_id"`
		Ref    cognitionpolicy.ProviderGenerationEvidenceRef `json:"ref"`
	}{evidence.CallID, identityRef})
	if err != nil {
		t.Fatal(err)
	}
	identityDigest := sha256.Sum256(authority)
	evidence.Ref.ID = "provider_generation_" + hex.EncodeToString(identityDigest[:])
	return evidence
}
