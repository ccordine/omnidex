package codex

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestNativeCompactionHandsBackIncompleteWorkWithoutContinuing(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Plan, codingagent.Execute, codingagent.Review} {
		result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
		raw := `{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"compaction","type":"contextCompaction"}}}`
		receipts := 0
		finished, err := consumeAgentFrame(decodeAgentTestFrame(t, raw), []byte(raw), &result, role, map[string]string{}, func(event codingagent.NativeEvent) error { receipts++; return nil })
		var incomplete *codingagent.IncompleteTaskError
		if !errors.As(err, &incomplete) || finished || result.Completed || receipts != 1 || incomplete.ThreadID != "thread" || incomplete.TurnID != "turn" {
			t.Fatalf("%s continued or discarded its context boundary: finished=%v receipts=%d error=%v", role, finished, receipts, err)
		}
	}
}

func TestNativeContextWindowFailureIsAPlanningHandoff(t *testing.T) {
	for _, raw := range []string{`"contextWindowExceeded"`, `"ContextWindowExceeded"`, `{"contextWindowExceeded":{}}`} {
		var incomplete *codingagent.IncompleteTaskError
		if err := nativeAgentFailure(&agentNativeError{Message: "Context is full", CodexErrorInfo: json.RawMessage(raw)}); !errors.As(err, &incomplete) {
			t.Fatalf("context exhaustion became a terminal failure: %v", err)
		}
	}
	result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
	raw := `{"method":"error","params":{"threadId":"thread","turnId":"turn","willRetry":true,"error":{"message":"Context full","codexErrorInfo":"contextWindowExceeded"}}}`
	var incomplete *codingagent.IncompleteTaskError
	if _, err := consumeAgentFrame(decodeAgentTestFrame(t, raw), []byte(raw), &result, codingagent.Execute, map[string]string{}, nil); !errors.As(err, &incomplete) || incomplete.ThreadID != "thread" || len(result.Evidence) != 1 {
		t.Fatalf("native retry status swallowed the context handoff: %v", err)
	}
}

func TestNativeAssignmentsUseOnlyCompiledContextWithout128KiBCutoff(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Plan, codingagent.Execute, codingagent.Review} {
		call := codingagent.Assignment{Role: role, Context: "The exact task, required guidance, and owner answer.", Task: &codingagent.Task{Goal: strings.Repeat("raw context ", 20000)}}
		prompt, err := renderAgentAssignment(call)
		if err != nil || strings.Contains(prompt, call.Context) || strings.Contains(prompt, "raw context") {
			t.Fatalf("%s put shared context into the literal prompt: %v", role, err)
		}
		context, err := renderAgentContext(call)
		if err != nil || !strings.Contains(context, call.Context) || strings.Contains(context, "raw context") {
			t.Fatalf("%s duplicated raw context: %v", role, err)
		}
		call.Context = ""
		if role == codingagent.Plan {
			call.Requirements = []codingagent.Requirement{{ID: "required", Outcome: call.Task.Goal}}
		}
		context, err = renderAgentContext(call)
		if err != nil || len(context) < 128*1024 {
			t.Fatalf("native history retained an unrelated byte ceiling: bytes=%d %v", len(context), err)
		}
	}
}

func TestPlanningHasNormalNativeToolsButParserHasNoExecution(t *testing.T) {
	for _, kind := range []string{"commandExecution", "fileChange", "mcpToolCall", "webSearch", "plan"} {
		frame := decodeAgentTestFrame(t, `{"method":"item/started","params":{"item":{"id":"item","type":"`+kind+`"}}}`)
		if err := validateAgentItems(frame, codingagent.Plan, map[string]string{}); err != nil {
			t.Fatalf("planning tool %s blocked: %v", kind, err)
		}
	}
	frame := decodeAgentTestFrame(t, `{"method":"item/started","params":{"item":{"id":"item","type":"commandExecution"}}}`)
	if err := validateAgentItems(frame, codingagent.ExecutionResponse, map[string]string{}); err == nil {
		t.Fatal("pure response parser acquired execution")
	}
}
