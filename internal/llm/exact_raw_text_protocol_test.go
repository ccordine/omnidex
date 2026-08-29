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
	zero := ExactPreparedTemperature(0)
	prepared := PreparedModel{
		Protocol:  protocol,
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: "return one declaration", PromptHint: MinimalGeneratePrompt,
		MaxOutputTokens: 1024, ContextTokens: expected.NativeContextLimit,
		OutputLimitMode:             ExactPreparedOutputLimitExplicit,
		Temperature:                 &zero,
		RawTextStopSequence:         ExactPreparedRawChatEndV1,
		ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
	}
	return prepared
}

func TestExactRawTextProtocolOmitsFormatAndBindsExactBytes(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
	got, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"qwen:9b","options":{"num_ctx":32768,"num_predict":1024,"stop":["<|im_end|>"],"temperature":0},"prompt":"<|im_start|>user\nreturn one declaration\nReturn only the requested output.<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n","raw":true,"shift":false,"stream":false,"think":false,"truncate":false}`
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

func TestExactRawTextProtocolBindsRegisteredChatMLTerminator(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
	prepared.OutputLimitMode = ExactPreparedOutputLimitNatural
	got, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"qwen:9b","options":{"num_ctx":32768,"num_predict":1024,"stop":["<|im_end|>"],"temperature":0},"prompt":"<|im_start|>user\nreturn one declaration\nReturn only the requested output.<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n","raw":true,"shift":false,"stream":false,"think":false,"truncate":false}`
	if string(got) != want {
		t.Fatalf("raw ChatML-stopped request changed:\n got %s\nwant %s", got, want)
	}
}

func TestExactRawTextProtocolBindsRegisteredLFFraming(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
	prepared.RawTextStopSequence = ExactPreparedLineStopV1
	prepared.OutputLimitMode = ExactPreparedOutputLimitNatural
	got, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"qwen:9b","options":{"num_ctx":32768,"num_predict":1024,"stop":["\n"],"temperature":0},"prompt":"<|im_start|>user\nreturn one declaration\nReturn only the requested output.<|im_end|>\n<|im_start|>assistant\n<think>\n\n</think>\n\n","raw":true,"shift":false,"stream":false,"think":false,"truncate":false}`
	if string(got) != want {
		t.Fatalf("raw LF-framed request changed:\n got %s\nwant %s", got, want)
	}
	modelInput, err := ExactPreparedRequestModelInput(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(modelInput, ExactPreparedRawChatUserPrefixV1) ||
		!strings.HasSuffix(modelInput, ExactPreparedRawChatAssistantBoundaryV1) {
		t.Fatalf("raw single-line input does not have its exact ChatML boundary: %q", modelInput)
	}
}

func TestExactPreparedRequestModelInputBindsRawMultilineChatMLBoundary(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
	prepared.OutputLimitMode = ExactPreparedOutputLimitNatural
	base, err := ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ExactPreparedRequestModelInput(prepared)
	if err != nil {
		t.Fatal(err)
	}
	want := ExactPreparedRawChatUserPrefixV1 + base +
		ExactPreparedRawChatAssistantBoundaryV1
	if got != want {
		t.Fatalf("raw multiline model input lacks its ChatML boundary: got %q want %q", got, want)
	}
}

func TestExactRawTextProtocolRejectsReservedChatMLControlCollision(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
	prepared.OutputLimitMode = ExactPreparedOutputLimitNatural
	for _, control := range []string{"<|im_start|>", ExactPreparedRawChatEndV1} {
		changed := prepared
		changed.Prompt += " " + control
		if _, err := ExactPreparedRequestBytes(changed); err == nil {
			t.Fatalf("raw request accepted reserved ChatML control collision %q", control)
		}
	}
}

func TestExactPreparedRequestModelInputLeavesNativePhiSingleLineFieldsUnchanged(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
	expected := *prepared.ProviderIdentityExpectation
	expected.Model = "phi4:14b"
	expected.TokenizerProfile = ExactPreparedTokenizerProfilePhi3GPT4O
	prepared.BaseModel, prepared.ContextModel = expected.Model, expected.Model
	prepared.ProviderIdentityExpectation = &expected
	prepared.RawTextStopSequence = ExactPreparedLineStopV1
	prepared.OutputLimitMode = ExactPreparedOutputLimitNatural
	prepared.Temperature = nil
	challenge, err := DeriveProviderIdentityObservationChallenge("native-phi-line-test", expected)
	if err != nil {
		t.Fatal(err)
	}
	prepared.ProviderObservationChallenge = challenge

	modelInput, err := ExactPreparedRequestModelInput(prepared)
	if err != nil {
		t.Fatal(err)
	}
	wantInput, err := ExactPreparedModelInput(prepared.Prompt, prepared.PromptHint)
	if err != nil {
		t.Fatal(err)
	}
	if modelInput != wantInput || strings.HasSuffix(modelInput, ExactPreparedLineStopV1) {
		t.Fatalf("native Phi model input changed: got %q want %q", modelInput, wantInput)
	}
	wire, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	wantWire := `{"model":"phi4:14b","options":{"num_ctx":32768,"stop":["\n"]},"prompt":"Return only the requested output.","raw":false,"shift":false,"stream":false,"system":"return one declaration","truncate":false}`
	if string(wire) != wantWire {
		t.Fatalf("native Phi single-line wire changed:\n got %s\nwant %s", wire, wantWire)
	}
}

func TestExactPreparedRequestModelInputRejectsInvalidProfileIdentity(t *testing.T) {
	valid := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
	for name, mutate := range map[string]func(*PreparedModel){
		"missing expectation": func(prepared *PreparedModel) {
			prepared.ProviderIdentityExpectation = nil
		},
		"unknown profile": func(prepared *PreparedModel) {
			expected := *prepared.ProviderIdentityExpectation
			expected.TokenizerProfile = "unregistered-profile"
			prepared.ProviderIdentityExpectation = &expected
		},
		"model mismatch": func(prepared *PreparedModel) {
			expected := *prepared.ProviderIdentityExpectation
			expected.Model = "different:model"
			prepared.ProviderIdentityExpectation = &expected
		},
		"context mismatch": func(prepared *PreparedModel) {
			prepared.ContextTokens--
		},
	} {
		t.Run(name, func(t *testing.T) {
			prepared := valid
			mutate(&prepared)
			if _, err := ExactPreparedRequestModelInput(prepared); err == nil {
				t.Fatal("invalid exact request identity was accepted")
			}
		})
	}
}

func TestExactPreparedStopSequenceMustBeRegistered(t *testing.T) {
	raw := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
	raw.RawTextStopSequence = "<END>"
	if _, err := ExactPreparedRequestBytes(raw); err == nil {
		t.Fatal("raw-text exact request accepted an unregistered stop sequence")
	}
}

func TestExactRawTextProtocolRejectsImplicitOrStructuredAuthority(t *testing.T) {
	valid := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
	mutations := map[string]func(*PreparedModel){
		"missing protocol": func(value *PreparedModel) { value.Protocol = "" },
		"unknown protocol": func(value *PreparedModel) { value.Protocol = "unknown" },
		"prompt hint":      func(value *PreparedModel) { value.PromptHint = "Return TypeScript." },
		"temperature": func(value *PreparedModel) {
			unregistered := ExactPreparedTemperature(0.3)
			value.Temperature = &unregistered
		},
		"negative zero": func(value *PreparedModel) {
			negativeZero := ExactPreparedTemperature(math.Copysign(0, -1))
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
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
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
	promptBytes := measuredRawInputBytes - len(ExactPreparedRawChatUserPrefixV1) -
		len(ExactPreparedPromptJoiner) - len(MinimalGeneratePrompt) -
		len(ExactPreparedRawChatAssistantBoundaryV1)
	prepared.Prompt = strings.Repeat("x", promptBytes)
	rawInput, err := ExactPreparedRequestModelInput(prepared)
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
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
	prepared.MaxOutputTokens = prepared.ContextTokens
	if _, err := ExactPreparedRequestBytes(prepared); err == nil {
		t.Fatal("exact request accepted an output reservation with no input capacity")
	}
}

func TestExactRawTextProtocolRejectsInputBeyondGrossByteCeiling(t *testing.T) {
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV2)
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

func TestDecodeExactPreparedResponseKeepsRawContent(t *testing.T) {
	typeScript := "export function add(a: number, b: number): number {\n  return a + b;\n}\n"
	body := exactProtocolResponseBody(t, typeScript)
	raw, err := DecodeExactPreparedResponseForProtocol(
		ExactPreparedProtocolRawTextV2, 200, body,
	)
	if err != nil || raw.Content != typeScript {
		t.Fatalf("raw TypeScript response=%+v error=%v", raw, err)
	}
	rawText := " exact semantic leaf "
	semantic, err := DecodeExactPreparedResponse(
		200,
		exactProtocolResponseBody(t, rawText),
	)
	if err != nil || semantic.Content != rawText {
		t.Fatalf("raw semantic response=%+v error=%v", semantic, err)
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
