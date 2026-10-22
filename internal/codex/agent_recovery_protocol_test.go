package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestRecoveryReadsOriginalNativeTurnWithoutDispatch(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review} {
		t.Run(string(role), func(t *testing.T) {
			for _, status := range []string{"completed", "inProgress", "interrupted"} {
				t.Run(status, func(t *testing.T) {
					writer := &protocolWriter{}
					responses := fmt.Sprintf("{\"id\":1,\"result\":{}}\n{\"id\":2,\"result\":{\"model\":\"fixture\",\"thread\":{\"id\":\"retained-thread\"}}}\n{\"id\":3,\"result\":{\"data\":[{\"id\":\"retained-turn\",\"status\":%q,\"items\":[{\"type\":\"agentMessage\",\"phase\":\"final_answer\",\"text\":\"Retained result.\"}]}]}}\n", status)
					connection := &connection{input: writer, output: bufio.NewReader(bytes.NewBufferString(responses))}
					call := codingagent.Assignment{Role: role, Root: t.TempDir(), Route: codingagent.Route{Model: "fixture"}, Session: &codingagent.ExecutionSession{ThreadID: "retained-thread", TurnID: "retained-turn", State: "running"}}
					thread, _, err := connection.startAgentThread(call, "")
					if err != nil || thread.Thread.ID != "retained-thread" {
						t.Fatalf("resume=%+v %v", thread, err)
					}
					frames := 0
					sessions := 0
					result, err := connection.recoverExecutionResult(call, codingagent.Result{ThreadID: thread.Thread.ID}, func(event codingagent.NativeEvent) error {
						if len(event.Frame) > 0 {
							frames++
						}
						if event.Session != nil {
							sessions++
							if event.Session.ThreadID != "retained-thread" || event.Session.TurnID != "retained-turn" || event.Session.State != "idle" {
								t.Fatalf("lost terminal session %+v", event.Session)
							}
						}
						return nil
					})
					if status == "completed" {
						if err != nil || !result.Completed || result.TurnID != "retained-turn" || string(result.Content) != `"Retained result."` {
							t.Fatalf("result=%+v %v", result, err)
						}
					} else if err == nil {
						t.Fatal("uncompleted native turn became complete")
					}
					if (status == "interrupted" && sessions != 1) || (status != "interrupted" && sessions != 0) {
						t.Fatalf("unexpected terminal session count %d", sessions)
					}
					if frames != 1 {
						t.Fatalf("missing original response evidence: %d", frames)
					}
					encoded := writer.String()
					if strings.Contains(encoded, "turn/start") || strings.Contains(encoded, "thread/start") || !strings.Contains(encoded, "thread/resume") || !strings.Contains(encoded, "thread/turns/list") {
						t.Fatalf("recovery dispatched new work: %s", encoded)
					}
					var requests []map[string]any
					scanner := bufio.NewScanner(strings.NewReader(encoded))
					for scanner.Scan() {
						var request map[string]any
						if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
							t.Fatal(err)
						}
						requests = append(requests, request)
					}
					if len(requests) != 4 {
						t.Fatalf("unexpected protocol calls %d", len(requests))
					}
				})
			}
		})
	}
}
