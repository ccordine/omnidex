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
			conn := &connection{input: writer, output: bufio.NewScanner(bytes.NewBufferString(responses))}
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
			if requests[2]["method"] != "thread/start" || requests[3]["method"] != "thread/inject_items" || requests[4]["method"] != "turn/start" {
				t.Fatal("fresh execution/review flow changed")
			}
			turn := requests[4]["params"].(map[string]any)
			literal := turn["input"].([]any)[0].(map[string]any)["text"].(string)
			if literal != task.Instructions() {
				t.Fatal("provider-bound current obligation is incomplete")
			}
			background := requests[3]["params"].(map[string]any)["items"].([]any)[0].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
			for _, text := range []string{task.Objective, task.OwnerContext[0].Text, "owner-17", task.Acceptance[0].Verification[0], "CSV", "answer_evidence_id"} {
				if !strings.Contains(background, text) {
					t.Fatal("provider-bound context lost", text)
				}
			}
			if role == codingagent.Review && !strings.Contains(background, call.ExecutionReport.Response) {
				t.Fatal("review lost original execution evidence")
			}
			if strings.Contains(writer.String(), "thread/resume") || strings.Contains(writer.String(), "review/start") {
				t.Fatal("isolated ordinary sessions changed")
			}
		})
	}
}
