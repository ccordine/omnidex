package codex

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestNativeFinalOutputExcludesEarlierMessagesWithoutLosingEvidence(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Plan, codingagent.Execute, codingagent.Review} {
		for _, earlier := range []string{"Which format should I use? May I install a dependency?", "Work is blocked: the required runtime is unavailable."} {
			result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
			items := map[string]string{}
			receipts := []json.RawMessage{}
			const final = "  The requested work is complete.\nAll required checks passed.  "
			for _, item := range []map[string]any{
				{"id": "question", "type": "agentMessage", "phase": "commentary", "delivery": "async", "text": earlier},
				{"id": "progress", "type": "agentMessage", "phase": "commentary", "text": "I am still working."},
				{"id": "tool", "type": "commandExecution", "status": "completed", "aggregatedOutput": "An earlier diagnostic failed."},
				{"id": "reasoning", "type": "reasoning", "text": "Intermediate analysis is not the final answer."},
				{"id": "answer", "type": "agentMessage", "phase": "final_answer", "text": final},
			} {
				for _, method := range []string{"item/started", "item/completed"} {
					raw, err := json.Marshal(map[string]any{"method": method, "params": map[string]any{"threadId": "thread", "turnId": "turn", "item": item}})
					if err != nil {
						t.Fatal(err)
					}
					done, err := consumeAgentFrame(decodeAgentTestFrame(t, string(raw)), raw, &result, role, items, func(event codingagent.NativeEvent) error {
						receipts = append(receipts, append(json.RawMessage(nil), event.Frame...))
						return nil
					})
					if err != nil || done || !bytes.Equal(receipts[len(receipts)-1], raw) {
						t.Fatalf("%s discarded original evidence or accepted an unfinished turn: %v", role, err)
					}
				}
			}
			raw := json.RawMessage(`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed"}}}`)
			done, err := consumeAgentFrame(decodeAgentTestFrame(t, string(raw)), raw, &result, role, items, func(event codingagent.NativeEvent) error {
				receipts = append(receipts, event.Frame)
				return nil
			})
			if err != nil || !done || !result.Completed || len(receipts) != 11 {
				t.Fatalf("%s lost completion or evidence: %v", role, err)
			}
			var source string
			if err := json.Unmarshal(result.Content, &source); err != nil || source != final {
				t.Fatalf("%s parser source includes stream history or rewrites the final answer: %q %v", role, source, err)
			}
		}
	}
}

func TestTurnSummaryUsesOnlyItsFinalOutput(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Plan, codingagent.Execute, codingagent.Review} {
		result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
		raw := `{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed","items":[` +
			`{"type":"agentMessage","phase":"final_answer","delivery":"async","text":"Earlier permission request."},` +
			`{"type":"agentMessage","phase":"commentary","text":"Earlier progress."},` +
			`{"type":"commandExecution","text":"Earlier command output."},` +
			`{"type":"agentMessage","phase":"final_answer","text":"Final answer only."}]}}}`
		if done, err := collectAgentFrame(decodeAgentTestFrame(t, raw), []byte(raw), &result, role); err != nil || !done {
			t.Fatalf("%s failed to collect terminal summary: %v", role, err)
		}
		var source string
		if err := json.Unmarshal(result.Content, &source); err != nil || source != "Final answer only." {
			t.Fatalf("%s parser source includes earlier messages: %s %v", role, result.Content, err)
		}
	}
}

func TestTurnSummaryRejectsAsyncMessagesWithoutFinalOutput(t *testing.T) {
	result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
	raw := `{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed","items":[{"type":"agentMessage","phase":"final_answer","delivery":"async","text":"An earlier question."}]}}}`
	if _, err := collectAgentFrame(decodeAgentTestFrame(t, raw), []byte(raw), &result, codingagent.Execute); err == nil || result.Completed || len(result.Content) != 0 {
		t.Fatal("an asynchronous message substituted for missing final output")
	}
}
