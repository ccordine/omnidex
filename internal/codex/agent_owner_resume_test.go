package codex

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestOwnerResumeInspectsTurnBeforeContinuing(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review} {
		for _, fixture := range []struct {
			name, status, turn, reply, previous string
			continueTurn                        bool
		}{
			{name: "interrupted", status: "interrupted", turn: "retained-turn", reply: "Resume.", continueTurn: true},
			{name: "failed", status: "failed", turn: "retained-turn", reply: "Resume.", continueTurn: true},
			{name: "completed", status: "completed", turn: "retained-turn", reply: "Resume."},
			{name: "still running", status: "inProgress", turn: "retained-turn", reply: "Resume."},
			{name: "no owner resume", status: "interrupted", turn: "retained-turn"},
			{name: "different turn", status: "interrupted", turn: "unrelated-turn", reply: "Resume."},
			{name: "previous turn", status: "interrupted", turn: "retained-turn", previous: "retained-turn", reply: "Resume."},
			{name: "missing identity", status: "interrupted", reply: "Resume."},
		} {
			t.Run(string(role)+"/"+fixture.name, func(t *testing.T) {
				writer := &protocolWriter{}
				responses := fmt.Sprintf("{\"id\":1,\"result\":{}}\n{\"id\":2,\"result\":{\"model\":\"fixture\",\"thread\":{\"id\":\"retained-thread\"}}}\n{\"id\":3,\"result\":{\"data\":[{\"id\":%q,\"status\":%q,\"items\":[{\"type\":\"agentMessage\",\"phase\":\"final_answer\",\"text\":\"Retained result.\"}]}]}}\n{\"id\":4,\"result\":{}}\n{\"id\":5,\"result\":{\"turn\":{\"id\":\"continued-turn\"}}}\n", fixture.turn, fixture.status)
				conn := &connection{input: writer, output: bufio.NewReader(bytes.NewBufferString(responses))}
				call := codingagent.Assignment{Role: role, Root: t.TempDir(), Route: codingagent.Route{Model: "fixture"},
					Task: &codingagent.Task{ID: "retained-task", Goal: "Finish the approved task."}, Reply: fixture.reply,
					Session: &codingagent.ExecutionSession{ThreadID: "retained-thread", TurnID: "retained-turn", PreviousTurnID: fixture.previous, State: "running"}}
				thread, _, err := conn.startAgentThread(call, "")
				if err != nil {
					t.Fatal(err)
				}
				var retained *codingagent.ExecutionSession
				result, continuing, err := conn.recoverOrContinueAgent(&call, codingagent.Result{ThreadID: thread.Thread.ID}, func(event codingagent.NativeEvent) error {
					if event.Session != nil {
						retained = event.Session
					}
					return nil
				})
				if continuing != fixture.continueTurn {
					t.Fatalf("continuing=%v, error=%v", continuing, err)
				}
				if continuing {
					if retained == nil || retained.State != "interrupted" || retained.ThreadID != "retained-thread" || retained.TurnID != "retained-turn" {
						t.Fatalf("continuation authority would be lost on restart: %+v", retained)
					}
					if err != nil || result.TurnID != "" || result.Completed || call.Session.State != "interrupted" || call.Session.TurnID != "retained-turn" {
						t.Fatalf("invalid continuation: result=%+v session=%+v error=%v", result, call.Session, err)
					}
					prompt, err := renderAgentAssignment(call)
					if err != nil || !strings.Contains(prompt, "Inspect retained command results") || !strings.Contains(prompt, fixture.reply) {
						t.Fatalf("continuation lost retained-work instructions: %s %v", prompt, err)
					}
					if _, err := conn.injectAgentContext(call, thread.Thread.ID); err != nil {
						t.Fatal(err)
					}
					turn, _, _, err := conn.startAgentTurn(call, thread, prompt)
					if err != nil || turn != "continued-turn" {
						t.Fatalf("continuation dispatch: %s %v", turn, err)
					}
				} else if fixture.status == "completed" {
					if err != nil || !result.Completed || string(result.Content) != `"Retained result."` {
						t.Fatalf("completed result lost: %+v %v", result, err)
					}
				} else if err == nil {
					t.Fatal("unresolved turn became successful")
				}
				wire := writer.String()
				wantStarts := 0
				if fixture.continueTurn {
					wantStarts = 1
				}
				if strings.Contains(wire, `"method":"thread/start"`) || strings.Count(wire, `"method":"turn/start"`) != wantStarts || !strings.Contains(wire, `"method":"thread/turns/list"`) {
					t.Fatalf("unexpected dispatch: %s", wire)
				}
			})
		}
	}
}
