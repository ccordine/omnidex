package codex

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestNativeCompactionContinuesSameAssignment(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Plan, codingagent.Execute, codingagent.Review} {
		t.Run(string(role), func(t *testing.T) {
			result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
			var evidence []json.RawMessage
			call := codingagent.Assignment{Role: role}
			if role != codingagent.Plan {
				call.Task = &codingagent.Task{ID: "task"}
			}
			stream := newAgentActivityStream(call, &result, func(event codingagent.NativeEvent) error {
				if len(event.Frame) > 0 {
					evidence = append(evidence, append(json.RawMessage(nil), event.Frame...))
				}
				return nil
			})
			inputs := []string{
				`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"before","type":"agentMessage","phase":"commentary"}}}`,
				`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"before","type":"agentMessage","phase":"commentary","text":"Work in progress."}}}`,
				`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"compact","type":"contextCompaction"}}}`,
				`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"compact","type":"contextCompaction"}}}`,
				`{"method":"thread/compacted","params":{"threadId":"thread","turnId":"turn"}}`,
				`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"after","type":"commandExecution","command":"verify retained work","cwd":"/workspace"}}}`,
				`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"after","type":"commandExecution","command":"verify retained work","cwd":"/workspace","status":"completed","exitCode":0}}}`,
				`{"method":"item/started","params":{"threadId":"thread","turnId":"turn","item":{"id":"answer","type":"agentMessage","phase":"final_answer"}}}`,
				`{"method":"item/completed","params":{"threadId":"thread","turnId":"turn","item":{"id":"answer","type":"agentMessage","phase":"final_answer","text":"Verified retained work."}}}`,
				`{"method":"turn/completed","params":{"threadId":"thread","turn":{"id":"turn","status":"completed"}}}`,
			}
			for i, input := range inputs {
				var frame rpcFrame
				if err := json.Unmarshal([]byte(input), &frame); err != nil {
					t.Fatal(err)
				}
				done, err := consumeAgentFrame(frame, json.RawMessage(input), &result, role, stream.observe)
				if err != nil || done != (i == len(inputs)-1) {
					t.Fatalf("frame %d interrupted or prematurely completed assignment: done=%v err=%v", i, done, err)
				}
			}
			if !result.Completed || string(result.Content) != `"Verified retained work."` || result.ThreadID != "thread" || result.TurnID != "turn" || len(stream.items) != 0 {
				t.Fatalf("lost assignment state after compaction: %+v, activity=%v", result, stream.items)
			}
			if len(evidence) != len(inputs) {
				t.Fatalf("lost evidence: got %d frames, want %d", len(evidence), len(inputs))
			}
			for i, input := range inputs {
				if !bytes.Equal(evidence[i], []byte(input)) {
					t.Fatalf("changed raw evidence at frame %d", i)
				}
			}
		})
	}
}

func TestNativeContextFailureHonorsProviderRetryStatus(t *testing.T) {
	for _, retry := range []bool{true, false} {
		result := codingagent.Result{ThreadID: "thread", TurnID: "turn"}
		params, err := json.Marshal(map[string]any{
			"threadId": "thread", "turnId": "turn", "willRetry": retry,
			"error": map[string]any{"message": "Context window exhausted", "codexErrorInfo": "contextWindowExceeded"},
		})
		if err != nil {
			t.Fatal(err)
		}
		frame := rpcFrame{Method: "error", Params: params}
		raw, err := json.Marshal(frame)
		if err != nil {
			t.Fatal(err)
		}
		done, err := consumeAgentFrame(frame, raw, &result, codingagent.Execute, nil)
		var incomplete *codingagent.IncompleteTaskError
		if done || result.Completed || (retry && err != nil) || (!retry && !errors.As(err, &incomplete)) || len(result.Evidence) != 1 {
			t.Fatalf("retry=%v: lost failure, retry or evidence: done=%v err=%v result=%+v", retry, done, err, result)
		}
	}
}
