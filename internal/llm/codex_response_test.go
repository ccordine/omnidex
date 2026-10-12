package llm

import (
	"encoding/json"
	"strings"
	"testing"
)

func codexResponseFixture(t *testing.T) []byte {
	t.Helper()
	capture := CodexResponseCapture{
		Thread:     json.RawMessage(`{"id":3,"result":{"model":"fixture-model","thread":{"id":"thread"}}}`),
		Turn:       json.RawMessage(`{"id":4,"result":{"turn":{"id":"turn"}}}`),
		Message:    json.RawMessage(`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"type":"agentMessage","phase":"final_answer","text":"value"}}}`),
		Usage:      json.RawMessage(`{"method":"thread/tokenUsage/updated","params":{"threadId":"thread","turnId":"turn","tokenUsage":{"last":{"inputTokens":40,"outputTokens":12,"reasoningOutputTokens":5,"totalTokens":52}}}}`),
		Completion: json.RawMessage(`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed"}}}`),
	}
	raw, err := json.Marshal(capture)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCodexReceiptBindsCompletionUsageAndContinuation(t *testing.T) {
	raw := codexResponseFixture(t)
	response, err := DecodeExactPreparedResponseForProtocol(ExactPreparedProtocolCodexV1, 0, raw)
	if err != nil {
		t.Fatal(err)
	}
	if response.Content != "value" || response.Usage.EvalCount != 7 || response.Usage.ReasoningEvalCount != 5 {
		t.Fatalf("native projection = %+v", response)
	}
	retained, err := DecodeCodexRetainedContext(raw)
	if err != nil {
		t.Fatal(err)
	}
	context, err := ParseCodexContinuation(retained)
	if err != nil || context.ThreadID != "thread" || context.TurnID != "turn" {
		t.Fatalf("context=%+v err=%v", context, err)
	}
	if err := ValidateExactPreparedNativeUsage(51, -1, response.Usage); err == nil {
		t.Fatal("reasoning omitted from context budget")
	}
}

func TestCodexReceiptRejectsMissingForeignAndFailedEvidence(t *testing.T) {
	for _, change := range [][2]string{
		{`"status":"completed"`, `"status":"failed"`},
		{`"turnId":"turn"`, `"turnId":"other"`},
		{`"phase":"final_answer"`, `"phase":"commentary"`},
		{`"totalTokens":52`, `"totalTokens":53`},
		{`"inputTokens":40`, `"inputTokens":0`},
	} {
		raw := []byte(strings.ReplaceAll(string(codexResponseFixture(t)), change[0], change[1]))
		if _, err := DecodeCodexRetainedContext(raw); err == nil {
			t.Fatalf("invalid native receipt accepted: %v", change)
		}
	}
	if _, err := DecodeExactPreparedResponseForProtocol(ExactPreparedProtocolCodexV1, 200, codexResponseFixture(t)); err == nil {
		t.Fatal("JSON-RPC receipt fabricated an HTTP status")
	}
}
