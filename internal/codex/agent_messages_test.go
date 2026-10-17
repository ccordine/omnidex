package codex

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func capturedAsyncMessages(t *testing.T) []json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/planning-gauntlet/execution-resume.json")
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Frames []json.RawMessage `json:"async_native_receipts"`
	}
	if err := json.Unmarshal(raw, &capture); err != nil || len(capture.Frames) != 2 {
		t.Fatalf("captured native messages: %v", err)
	}
	return capture.Frames
}

func capturedAsyncResult(t *testing.T, raw json.RawMessage) codingagent.Result {
	t.Helper()
	var frame struct {
		Params struct {
			ThreadID string `json:"threadId"`
			TurnID   string `json:"turnId"`
		} `json:"params"`
	}
	if err := json.Unmarshal(raw, &frame); err != nil {
		t.Fatal(err)
	}
	return codingagent.Result{ThreadID: frame.Params.ThreadID, TurnID: frame.Params.TurnID}
}

func TestCapturedNativeAsyncQuestionIsExcludedFromTheFinalResponse(t *testing.T) {
	frames := capturedAsyncMessages(t)
	for _, role := range []codingagent.Role{codingagent.Plan, codingagent.Execute, codingagent.Review} {
		result := capturedAsyncResult(t, frames[0])
		for _, raw := range frames {
			if done, err := collectAgentFrame(decodeAgentTestFrame(t, string(raw)), raw, &result, role); err != nil || done {
				t.Fatalf("async message was mistaken for a duplicate terminal answer: %v", err)
			}
		}
		raw, _ := json.Marshal(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": result.ThreadID, "turn": map[string]any{"id": result.TurnID, "status": "completed"}}})
		if done, err := collectAgentFrame(decodeAgentTestFrame(t, string(raw)), raw, &result, role); err != nil || !done || !result.Completed {
			t.Fatalf("actual terminal completion was lost: %v", err)
		}
		var text string
		if err := json.Unmarshal(result.Content, &text); err != nil || text != "I’ll wait for your verification name before reading or changing files." {
			t.Fatalf("parser source differs from the exact final response: %s %v", result.Content, err)
		}
	}
}

func TestNativeAsyncMessagesDoNotAuthorizeMissingOrDuplicateTerminalAnswers(t *testing.T) {
	frames := capturedAsyncMessages(t)
	result := capturedAsyncResult(t, frames[0])
	_, _ = collectAgentFrame(decodeAgentTestFrame(t, string(frames[0])), frames[0], &result, codingagent.Execute)
	raw, _ := json.Marshal(map[string]any{"method": "turn/completed", "params": map[string]any{"threadId": result.ThreadID, "turn": map[string]any{"id": result.TurnID, "status": "completed"}}})
	if _, err := collectAgentFrame(decodeAgentTestFrame(t, string(raw)), raw, &result, codingagent.Execute); err == nil || result.Completed {
		t.Fatal("async question alone was treated as terminal success")
	}
	_, err := collectAgentFrame(decodeAgentTestFrame(t, string(frames[1])), frames[1], &result, codingagent.Execute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := collectAgentFrame(decodeAgentTestFrame(t, string(frames[1])), frames[1], &result, codingagent.Execute); err == nil {
		t.Fatal("two actual terminal answers were accepted")
	}
}

func TestApprovedTaskDispatchRetainsPlanningConstraintWithoutRequestingPlanningAgain(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/planning-gauntlet/execution-resume.json")
	if err != nil {
		t.Fatal(err)
	}
	var capture struct {
		Entries []codingagent.Entry `json:"application_entries"`
	}
	if err := json.Unmarshal(raw, &capture); err != nil {
		t.Fatal(err)
	}
	var task *codingagent.Task
	for _, entry := range capture.Entries {
		if entry.Continuation != nil {
			task = &entry.Continuation.Task
		}
	}
	if task == nil {
		t.Fatal("captured incomplete execution task is missing")
	}
	call := codingagent.Assignment{Role: codingagent.Execute, Task: task}
	prompt, err := renderAgentAssignment(call)
	if err != nil || !strings.Contains(prompt, "approved ledger task") || !strings.Contains(prompt, "planning review is already complete") {
		t.Fatalf("dispatch lost code-owned execution phase: %q %v", prompt, err)
	}
	history, err := renderAgentContext(call)
	if err != nil || !strings.Contains(history, "Make no changes until the owner reviews the plan.") || strings.Contains(prompt, task.Goal) {
		t.Fatalf("phase fix dropped guidance or stuffed it into the current prompt: %v", err)
	}
}
