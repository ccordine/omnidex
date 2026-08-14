package llm

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func exactProtocolPrepared(t *testing.T, protocol ExactPreparedProtocol) PreparedModel {
	t.Helper()
	expected := providerIdentityTestExpectation()
	challenge, err := DeriveProviderIdentityObservationChallenge("raw-text-protocol-test", expected)
	if err != nil {
		t.Fatal(err)
	}
	zero := 0.0
	prepared := PreparedModel{
		Protocol:  protocol,
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: "return one declaration", PromptHint: MinimalGeneratePrompt,
		MaxOutputTokens: 1024, ContextTokens: expected.NativeContextLimit,
		OutputLimitMode: ExactPreparedOutputLimitExplicit,
		ThinkingEnabled: false, Temperature: &zero,
		ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
	}
	if protocol == ExactPreparedProtocolStructuredV1 {
		prepared.ResponseFormat = ResponseFormatJSON
		prepared.ResponseSchema = map[string]any{
			"additionalProperties": false,
			"properties":           map[string]any{"candidate_id": map[string]any{"type": "string"}},
			"required":             []string{"candidate_id"},
			"type":                 "object",
		}
	}
	return prepared
}

func TestExactStructuredProtocolV1RequestBytesRemainFrozen(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolStructuredV1)
	got, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"format":{"additionalProperties":false,"properties":{"candidate_id":{"type":"string"}},"required":["candidate_id"],"type":"object"},"model":"qwen:9b","options":{"num_ctx":32768,"num_predict":1024,"temperature":0},"prompt":"return one declaration\nReturn only the requested output.","raw":true,"shift":false,"stream":false,"think":false,"truncate":false}`
	if string(got) != want {
		t.Fatalf("structured exact request changed:\n got %s\nwant %s", got, want)
	}
}

func TestExactRawTextProtocolOmitsFormatAndBindsExactBytes(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV1)
	got, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"qwen:9b","options":{"num_ctx":32768,"num_predict":1024,"temperature":0},"prompt":"return one declaration\nReturn only the requested output.","raw":true,"shift":false,"stream":false,"think":false,"truncate":false}`
	if string(got) != want || strings.Contains(string(got), `"format"`) {
		t.Fatalf("raw-text exact request=%s want=%s", got, want)
	}
	firstSHA, err := ExactPreparedRequestSHA256(prepared)
	if err != nil {
		t.Fatal(err)
	}
	prepared.Prompt += "."
	secondSHA, err := ExactPreparedRequestSHA256(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if firstSHA == secondSHA {
		t.Fatal("raw-text request mutation did not change the exact request hash")
	}
}

func TestExactRawTextProtocolBindsRegisteredAdvisoryTerminator(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV1)
	prepared.RawTextStopSequence = ExactPreparedObjectiveAdvisoryStopV1
	got, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"qwen:9b","options":{"num_ctx":32768,"num_predict":1024,"stop":["\n<END_OBJECTIVE_ADVISORY_V1>"],"temperature":0},"prompt":"return one declaration\nReturn only the requested output.","raw":true,"shift":false,"stream":false,"think":false,"truncate":false}`
	if string(got) != want {
		t.Fatalf("raw-text stopped request changed:\n got %s\nwant %s", got, want)
	}
}

func TestExactRawTextProtocolBindsRegisteredCodeTerminator(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV1)
	prepared.RawTextStopSequence = ExactPreparedCodeStopV1
	prepared.OutputLimitMode = ExactPreparedOutputLimitNatural
	got, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"qwen:9b","options":{"num_ctx":32768,"stop":["<|endoftext|>"],"temperature":0},"prompt":"return one declaration\nReturn only the requested output.","raw":true,"shift":false,"stream":false,"think":false,"truncate":false}`
	if string(got) != want {
		t.Fatalf("raw-code stopped request changed:\n got %s\nwant %s", got, want)
	}
}

func TestExactPreparedStopIsRawOnlyAndRegistered(t *testing.T) {
	structured := exactProtocolPrepared(t, ExactPreparedProtocolStructuredV1)
	structured.RawTextStopSequence = ExactPreparedObjectiveAdvisoryStopV1
	if _, err := ExactPreparedRequestBytes(structured); err == nil {
		t.Fatal("structured exact request accepted a raw-text stop sequence")
	}

	raw := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV1)
	raw.RawTextStopSequence = "<END>"
	if _, err := ExactPreparedRequestBytes(raw); err == nil {
		t.Fatal("raw-text exact request accepted an unregistered stop sequence")
	}
}

func TestExactRawTextProtocolRejectsImplicitOrStructuredAuthority(t *testing.T) {
	valid := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV1)
	mutations := map[string]func(*PreparedModel){
		"missing protocol": func(value *PreparedModel) { value.Protocol = "" },
		"unknown protocol": func(value *PreparedModel) { value.Protocol = "unknown" },
		"response format":  func(value *PreparedModel) { value.ResponseFormat = ResponseFormatJSON },
		"response schema":  func(value *PreparedModel) { value.ResponseSchema = map[string]any{"type": "object"} },
		"empty schema":     func(value *PreparedModel) { value.ResponseSchema = map[string]any{} },
		"prompt hint":      func(value *PreparedModel) { value.PromptHint = "Return TypeScript." },
		"temperature":      func(value *PreparedModel) { one := 1.0; value.Temperature = &one },
		"negative zero": func(value *PreparedModel) {
			negativeZero := math.Copysign(0, -1)
			value.Temperature = &negativeZero
		},
		"missing identity":    func(value *PreparedModel) { value.ProviderIdentityExpectation = nil },
		"missing challenge":   func(value *PreparedModel) { value.ProviderObservationChallenge = "" },
		"missing output mode": func(value *PreparedModel) { value.OutputLimitMode = "" },
		"unknown output mode": func(value *PreparedModel) { value.OutputLimitMode = "unknown" },
		"zero output":         func(value *PreparedModel) { value.MaxOutputTokens = 0 },
		"zero context":        func(value *PreparedModel) { value.ContextTokens = 0 },
	}
	for name, mutate := range mutations {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			changed := valid
			mutate(&changed)
			if _, err := ExactPreparedRequestBytes(changed); err == nil {
				t.Fatal("invalid raw-text authority was accepted")
			}
		})
	}
}

func TestExactRawTextProtocolDoesNotTreatMeasuredInputBytesAsTokens(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV1)
	expected := *prepared.ProviderIdentityExpectation
	expected.NativeContextLimit = 8192
	challenge, err := DeriveProviderIdentityObservationChallenge(
		"raw-text-protocol-test", expected,
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared.ContextTokens = expected.NativeContextLimit
	prepared.MaxOutputTokens = 2048
	prepared.ProviderIdentityExpectation = &expected
	prepared.ProviderObservationChallenge = challenge

	const measuredRawInputBytes = 6485
	promptBytes := measuredRawInputBytes - len(ExactPreparedPromptJoiner) - len(MinimalGeneratePrompt)
	prepared.Prompt = strings.Repeat("x", promptBytes)
	rawInput, err := ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
	if err != nil {
		t.Fatal(err)
	}
	if len(rawInput) != measuredRawInputBytes {
		t.Fatalf("raw input=%dB want %dB", len(rawInput), measuredRawInputBytes)
	}
	wire, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatalf("measured input bytes were guessed to be tokens: %v", err)
	}
	for _, required := range []string{
		`"num_ctx":8192`, `"num_predict":2048`, `"raw":true`, `"truncate":false`,
	} {
		if !strings.Contains(string(wire), required) {
			t.Fatalf("exact provider request omitted %s", required)
		}
	}
}

func TestExactRawTextProtocolRejectsOutputReservationWithoutInputCapacity(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV1)
	prepared.MaxOutputTokens = prepared.ContextTokens
	if _, err := ExactPreparedRequestBytes(prepared); err == nil {
		t.Fatal("exact request accepted an output reservation with no input capacity")
	}
}

func TestExactRawTextProtocolRejectsInputBeyondGrossByteCeiling(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV1)
	prepared.Prompt = strings.Repeat("x", MaxExactPreparedModelInputBytes+1)
	if _, err := ExactPreparedRequestBytes(prepared); err == nil {
		t.Fatal("exact request accepted input beyond the registered gross byte ceiling")
	}
}

func TestExactPreparedNativeUsageUsesProviderCounts(t *testing.T) {
	valid := ProviderGenerationUsage{PromptEvalCount: 1730, EvalCount: 1418}
	if err := ValidateExactPreparedNativeUsage(8192, 6144, 2048, valid); err != nil {
		t.Fatalf("measured provider usage was rejected: %v", err)
	}
	for name, usage := range map[string]ProviderGenerationUsage{
		"prompt":  {PromptEvalCount: 6145, EvalCount: 1},
		"output":  {PromptEvalCount: 1, EvalCount: 2049},
		"context": {PromptEvalCount: 6144, EvalCount: 2049},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateExactPreparedNativeUsage(8192, 6144, 2048, usage); err == nil {
				t.Fatal("provider usage outside native authority was accepted")
			}
		})
	}
}

func TestExactPreparedNaturalUsageUsesOnlyTheNativeContextBoundary(t *testing.T) {
	valid := ProviderGenerationUsage{PromptEvalCount: 1730, EvalCount: 3000}
	if err := ValidateExactPreparedNaturalUsage(8192, valid); err != nil {
		t.Fatalf("natural provider usage was constrained by a stale sub-ceiling: %v", err)
	}
	if err := ValidateExactPreparedNaturalUsage(8192, ProviderGenerationUsage{
		PromptEvalCount: 5000, EvalCount: 3193,
	}); err == nil {
		t.Fatal("natural provider usage beyond native context was accepted")
	}
}

func TestDecodeExactPreparedResponseKeepsStructuredJSONAndAcceptsRawTypeScript(t *testing.T) {
	typeScript := "export function add(a: number, b: number): number {\n  return a + b;\n}\n"
	body := exactProtocolResponseBody(t, typeScript)
	raw, err := DecodeExactPreparedResponseForProtocol(
		ExactPreparedProtocolRawTextV1, 200, body,
	)
	if err != nil || raw.Content != typeScript {
		t.Fatalf("raw TypeScript response=%+v error=%v", raw, err)
	}
	structuredText := " {\n  \"candidate_id\": \"C31\"\n} "
	structured, err := DecodeExactPreparedResponse(
		200,
		exactProtocolResponseBody(t, structuredText),
	)
	if err != nil || structured.Content != structuredText {
		t.Fatalf("structured response=%+v error=%v", structured, err)
	}
}

func exactProtocolResponseBody(t *testing.T, content string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"model": "qwen:9b", "created_at": "2026-08-09T22:00:00Z",
		"response": content, "done": true, "done_reason": "stop",
		"total_duration": 101, "load_duration": 11,
		"prompt_eval_count": 41, "prompt_eval_duration": 21,
		"eval_count": 7, "eval_duration": 31,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
