package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExactR1PreparedRequestUsesAttestedTemplateAndUncappedThinking(t *testing.T) {
	t.Parallel()
	show := exactR1Show(t, "qwen3", false, exactR18BParameters, nil)
	selection, evidence := exactProviderProfileEvidence(t, "opaque-r1-transport:local", show)
	expected, err := DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := DeriveProviderIdentityObservationChallenge("r1-request-contract", expected)
	if err != nil {
		t.Fatal(err)
	}
	temperature := ExactPreparedTemperature(0.6)
	prepared := PreparedModel{
		Protocol:  ExactPreparedProtocolRawTextV1,
		BaseModel: expected.Model, ContextModel: expected.Model,
		Prompt: "return one declaration", PromptHint: MinimalGeneratePrompt,
		MaxOutputTokens: 1024, ContextTokens: expected.NativeContextLimit,
		OutputLimitMode: ExactPreparedOutputLimitNatural,
		ThinkingEnabled: true, Temperature: &temperature,
		RawTextStopSequence:         ExactPreparedCodeStopV1,
		ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
	}
	wire, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatalf("render attested R1 request: %v", err)
	}
	var request map[string]any
	if err := json.Unmarshal(wire, &request); err != nil {
		t.Fatal(err)
	}
	if raw, exists := request["raw"]; !exists || raw != false {
		t.Fatalf("R1 request did not select the attested provider template: %s", wire)
	}
	if think, exists := request["think"]; !exists || think != true {
		t.Fatalf("R1 request did not enable separate provider reasoning: %s", wire)
	}
	if _, exists := request["template"]; exists {
		t.Fatalf("R1 request supplied an unattested template override: %s", wire)
	}
	if request["prompt"] != "return one declaration\n"+MinimalGeneratePrompt {
		t.Fatalf("R1 semantic prompt changed: %q", request["prompt"])
	}
	options, ok := request["options"].(map[string]any)
	if !ok {
		t.Fatalf("R1 request options are not an object: %s", wire)
	}
	if _, exists := options["num_predict"]; exists {
		t.Fatalf("R1 request retained a station output-token cap: %s", wire)
	}
	if temperature, exists := options["temperature"]; !exists || temperature != 0.6 {
		t.Fatalf("R1 request did not bind temperature 0.6: %s", wire)
	}
	if _, exists := options["stop"]; exists {
		t.Fatalf("R1 request overrode its attested model control stops: %s", wire)
	}
	if strings.Contains(string(wire), ExactPreparedCodeStopV1) {
		t.Fatalf("R1 request inherited the Qwen code stop: %s", wire)
	}
}

func TestExactR114BPreparedRequestUsesTheSameReasoningTransport(t *testing.T) {
	t.Parallel()
	show := exactR1Show(t, "qwen2", true, exactR114BParameters, nil)
	selection, evidence := exactProviderProfileEvidence(t, "opaque-r1-fourteen:local", show)
	expected, err := DeriveExactProviderIdentityExpectation(evidence, selection)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := DeriveProviderIdentityObservationChallenge("r1-fourteen-contract", expected)
	if err != nil {
		t.Fatal(err)
	}
	temperature := ExactPreparedTemperature(0.6)
	prepared := PreparedModel{
		Protocol: ExactPreparedProtocolRawTextV1, BaseModel: expected.Model,
		ContextModel: expected.Model, Prompt: "return one declaration",
		PromptHint: MinimalGeneratePrompt, MaxOutputTokens: 1024,
		OutputLimitMode: ExactPreparedOutputLimitNatural,
		ContextTokens:   expected.NativeContextLimit, ThinkingEnabled: true, Temperature: &temperature,
		RawTextStopSequence:         ExactPreparedCodeStopV1,
		ProviderIdentityExpectation: &expected, ProviderObservationChallenge: challenge,
	}
	wire, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	var request map[string]any
	if err := json.Unmarshal(wire, &request); err != nil {
		t.Fatal(err)
	}
	options := request["options"].(map[string]any)
	if request["raw"] != false || request["think"] != true || options["temperature"] != 0.6 {
		t.Fatalf("R1 14B transport=%s", wire)
	}
	if _, exists := options["num_predict"]; exists {
		t.Fatalf("R1 14B request retained an output cap: %s", wire)
	}
	settings, err := ResolveExactPreparedTransport(expected)
	if err != nil || !settings.NativeTemplate || !settings.SeparateThinking ||
		settings.SeparateSystem || settings.Temperature == nil || *settings.Temperature != 0.6 {
		t.Fatalf("R1 14B transport settings=%+v error=%v", settings, err)
	}
}

func TestExactR1ProfileLeavesQwenRequestBytesUnchanged(t *testing.T) {
	t.Parallel()
	prepared := exactProtocolPrepared(t, ExactPreparedProtocolRawTextV1)
	prepared.RawTextStopSequence = ExactPreparedCodeStopV1
	prepared.OutputLimitMode = ExactPreparedOutputLimitNatural
	got, err := ExactPreparedRequestBytes(prepared)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"model":"qwen:9b","options":{"num_ctx":32768,"stop":["<|endoftext|>"],"temperature":0},"prompt":"return one declaration\nReturn only the requested output.","raw":true,"shift":false,"stream":false,"think":false,"truncate":false}`
	if string(got) != want {
		t.Fatalf("Qwen exact request changed:\n got %s\nwant %s", got, want)
	}
}

func TestDecodeExactPreparedResponseRetainsThinkingOutsideFinalCandidate(t *testing.T) {
	t.Parallel()
	reasoningCandidate := `{"candidate_id":"reasoning-must-not-win"}`
	finalCandidate := `{"candidate_id":"final"}`
	body := exactR1ResponseBody(t, reasoningCandidate, finalCandidate)
	decoded, err := DecodeExactPreparedResponseForProtocol(
		ExactPreparedProtocolStructuredV1, 200, body,
	)
	if err != nil {
		t.Fatalf("decode R1 response with separate thinking: %v", err)
	}
	if decoded.Content != finalCandidate || strings.Contains(decoded.Content, reasoningCandidate) {
		t.Fatalf("candidate content=%q want only final response %q", decoded.Content, finalCandidate)
	}
	if decoded.Thinking != reasoningCandidate {
		t.Fatalf("provider thinking was not retained separately: %+v", decoded)
	}
	var candidate struct {
		CandidateID string `json:"candidate_id"`
	}
	if err := json.Unmarshal([]byte(decoded.Content), &candidate); err != nil || candidate.CandidateID != "final" {
		t.Fatalf("decoded final candidate=%+v error=%v", candidate, err)
	}
}

func TestDecodeExactPreparedResponseRejectsNonStringThinking(t *testing.T) {
	t.Parallel()
	var fields map[string]any
	if err := json.Unmarshal(exactR1ResponseBody(t, "trace", "final"), &fields); err != nil {
		t.Fatal(err)
	}
	fields["thinking"] = nil
	body, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeExactPreparedResponseForProtocol(
		ExactPreparedProtocolRawTextV1, 200, body,
	); err == nil {
		t.Fatal("provider response accepted non-string thinking")
	}
}

func TestValidateExactPreparedResponseProjectionBindsThinking(t *testing.T) {
	t.Parallel()
	body := exactR1ResponseBody(t, "exact trace", "final")
	decoded, err := DecodeExactPreparedResponseForProtocol(
		ExactPreparedProtocolRawTextV1, 200, body,
	)
	if err != nil {
		t.Fatal(err)
	}
	generation := PreparedGeneration{
		Protocol: ExactPreparedProtocolRawTextV1, ProviderHTTPStatus: 200,
		ProviderResponseCapture: body, ProviderResponseDisposition: decoded.Disposition,
		ProviderResponseModel: decoded.Model, Content: decoded.Content, Thinking: decoded.Thinking,
		ProviderDonePresent: decoded.DonePresent, ProviderDone: decoded.Done,
		ProviderDoneReason: decoded.DoneReason, UsagePresent: decoded.UsagePresent, Usage: decoded.Usage,
	}
	if err := ValidateExactPreparedResponseProjection(generation); err != nil {
		t.Fatal(err)
	}
	generation.Thinking = "changed trace"
	if err := ValidateExactPreparedResponseProjection(generation); err == nil {
		t.Fatal("normalized provider projection accepted changed thinking")
	}
}

func TestDecodeExactPreparedResponseNeverUsesThinkingAsCandidateFallback(t *testing.T) {
	t.Parallel()
	body := exactR1ResponseBody(t, `{"candidate_id":"reasoning-only"}`, "")
	decoded, err := DecodeExactPreparedResponseForProtocol(
		ExactPreparedProtocolStructuredV1, 200, body,
	)
	if err == nil || decoded.Disposition != ProviderResponseEmptyContent || decoded.Content != "" {
		t.Fatalf("thinking became a candidate fallback: response=%+v error=%v", decoded, err)
	}
}

func exactR1ResponseBody(t *testing.T, thinking string, response string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"model": "opaque-r1-transport:local", "created_at": "2026-08-14T12:00:00Z",
		"thinking": thinking, "response": response,
		"done": true, "done_reason": "stop",
		"total_duration": 101, "load_duration": 11,
		"prompt_eval_count": 41, "prompt_eval_duration": 21,
		"eval_count": 7, "eval_duration": 31,
	})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
