package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

// Production JSON-RPC serialization, with scripted server replies. No model
// judgment or real implementation behavior is claimed by this protocol check.
func TestTaskPacketNativeExecutionAndReviewProtocol(t *testing.T) {
	task := codingagent.Task{ID: "packet", Goal: "Exact approved owner instruction.", Objective: "Separate Research destination.",
		OwnerContext: []model.TaskContext{{Text: "Read-only research is approved.", Provenance: model.LedgerProvenance{Kind: "owner_turn", Reference: "owner-17", Revision: 4}}},
		Constraints:  []string{"Preserve accepted work."}, ReadFiles: []string{"input.go"}, WriteFiles: []string{"output.go"},
		FileResponsibilities: []model.TaskFileResponsibility{{Path: "output.go", Action: "create", Responsibility: "Implement routing."}},
		Acceptance:           []model.TaskAcceptance{{ID: "routing", Text: "Research is separate.", Verification: []string{"Run routing tests."}}}}
	task.Payload, _ = json.Marshal(task.LedgerTask())
	for _, role := range []codingagent.Role{codingagent.Execute, codingagent.Review} {
		t.Run(string(role), func(t *testing.T) {
			call := codingagent.Assignment{JobID: 30, Generation: 2, Role: role, Task: &task, Root: t.TempDir(), Route: codingagent.Route{Provider: "codex", Model: "fixture"}, ReadFiles: []string{"input.go", "output.go"}, Clarifications: []codingagent.TaskClarification{{Question: "Which format?", Answer: "CSV", QuestionCallID: 11, AnswerEvidenceID: 12}}}
			if role == codingagent.Execute {
				call.WriteFiles = task.WriteFiles
			} else {
				call.ExecutionReport = &codingagent.ExecutionReport{CallID: 13, Round: 1, Response: "Original execution evidence: verification not run."}
			}
			if err := validateAgentAssignment(call); err != nil {
				t.Fatal(err)
			}
			writer := &protocolWriter{}
			responses := fmt.Sprintf("{\"id\":1,\"result\":{}}\n{\"id\":2,\"result\":{\"model\":\"fixture\",\"thread\":{\"id\":%q}}}\n{\"id\":3,\"result\":{}}\n{\"id\":4,\"result\":{\"turn\":{\"id\":\"turn\"}}}\n", "fresh-"+string(role))
			conn := &connection{input: writer, output: bufio.NewReader(bytes.NewBufferString(responses))}
			thread, _, err := conn.startAgentThread(call, "")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = conn.injectAgentContext(call, thread.Thread.ID); err != nil {
				t.Fatal(err)
			}
			prompt, err := renderAgentAssignment(call)
			if err != nil {
				t.Fatal(err)
			}
			if _, _, _, err = conn.startAgentTurn(call, thread, prompt); err != nil {
				t.Fatal(err)
			}
			var requests []map[string]any
			scan := bufio.NewScanner(strings.NewReader(writer.String()))
			for scan.Scan() {
				var req map[string]any
				if err = json.Unmarshal(scan.Bytes(), &req); err != nil {
					t.Fatal(err)
				}
				requests = append(requests, req)
			}
			if len(requests) != 5 {
				t.Fatalf("unexpected RPC calls %d", len(requests))
			}
			method := "turn/start"
			if role == codingagent.Review {
				method = "review/start"
			}
			if requests[2]["method"] != "thread/start" || requests[3]["method"] != "thread/inject_items" || requests[4]["method"] != method {
				t.Fatal("fresh execution/review flow changed")
			}
			turn := requests[4]["params"].(map[string]any)
			literal := ""
			if role == codingagent.Review {
				target := turn["target"].(map[string]any)
				if target["type"] != "custom" || turn["delivery"] != "inline" {
					t.Fatal("review did not use the native custom review target")
				}
				literal = target["instructions"].(string)
			} else {
				literal = turn["input"].([]any)[0].(map[string]any)["text"].(string)
			}
			want := task.Instructions()
			if role == codingagent.Review {
				want = "We just did this:\n\n" + task.Goal + "\n\n" + call.ExecutionReport.Response + "\n\nYou are to make sure that this was executed and implemented correctly."
			}
			if literal != want {
				t.Fatal("provider-bound current obligation is incomplete")
			}
			background := requests[3]["params"].(map[string]any)["items"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
			expected := []string{"CSV", "answer_evidence_id"}
			if role == codingagent.Execute {
				expected = append(expected, task.Objective, task.OwnerContext[0].Text, "owner-17", task.Acceptance[0].Verification[0])
			} else if strings.Contains(background, "\"task\"") || strings.Contains(background, task.Acceptance[0].Verification[0]) {
				t.Fatal("review inherited ledger restrictions")
			}
			for _, text := range expected {
				if !strings.Contains(background, text) {
					t.Fatal("provider-bound context lost", text)
				}
			}
			if role == codingagent.Review && !strings.Contains(literal, call.ExecutionReport.Response) {
				t.Fatal("review lost original execution evidence")
			}
			if strings.Contains(writer.String(), "thread/resume") {
				t.Fatal("review or execution reused another session")
			}
		})
	}
}
