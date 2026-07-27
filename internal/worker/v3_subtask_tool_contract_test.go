package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/artifacts"
	"github.com/gryph/omnidex/internal/workspace"
)

func TestSubtaskToolResponseContractUsesConcreteEnvelopes(t *testing.T) {
	contract := subtaskToolResponseContract("subtask_executor")
	for _, required := range []string{
		`"status":"continue"`,
		`"status":"success"`,
		`"status":"blocked"`,
		`"status":"fail"`,
	} {
		if !strings.Contains(contract, required) {
			t.Fatalf("response contract missing %q:\n%s", required, contract)
		}
	}
	if strings.Contains(contract, "continue|success") || strings.Contains(contract, "blocked|fail") {
		t.Fatalf("response contract retained an ambiguous pipe-delimited status:\n%s", contract)
	}
}

func TestSubtaskToolDecisionRequiresOneActionBeforeFeedback(t *testing.T) {
	raw := `{
		"role_id":"subtask_executor",
		"status":"continue",
		"tool_calls":[
			{"name":"workspace.write","input":{"path":"main.go","operation":"replace","content":"first"}},
			{"name":"command.run","input":{"program":"go","args":["test","./..."]}}
		],
		"final":"",
		"error":""
	}`
	decision, err := parseSubtaskToolDecision(raw, "subtask_executor")
	if err == nil || !strings.Contains(err.Error(), "exactly one tool call") {
		t.Fatalf("parseSubtaskToolDecision() err=%v, want immediate-feedback contract rejection", err)
	}
	if len(decision.ToolCalls) != 2 {
		t.Fatalf("parseSubtaskToolDecision() retained %d tool calls, want rejected decision details for direct feedback", len(decision.ToolCalls))
	}
}

func TestRejectedMultiActionDecisionProducesRecoverableSpecificFeedback(t *testing.T) {
	raw := `{
		"role_id":"subtask_executor",
		"status":"continue",
		"tool_calls":[
			{"name":"workspace.write","input":{"path":"main.go","operation":"replace","content":"bad payload is not retained"}},
			{"name":"command.run","input":{"program":"go","args":["test","./..."]}}
		],
		"final":"",
		"error":""
	}`
	decision, decisionErr := parseSubtaskToolDecision(raw, "subtask_executor")
	if decisionErr == nil {
		t.Fatal("multi-action decision unexpectedly passed validation")
	}
	record := rejectedSubtaskDecisionRecord(decision, decisionErr, "subtask_executor", "Build Pocket Tasks")
	if record.Result.Accepted {
		t.Fatal("rejected decision was marked accepted")
	}
	if got := record.Result.Output["tool_call_count"]; got != 2 {
		t.Fatalf("tool_call_count=%v, want 2", got)
	}
	if encoded, err := marshalToolRecords([]subtaskToolRecord{record}); err != nil {
		t.Fatal(err)
	} else if strings.Contains(encoded, "bad payload is not retained") {
		t.Fatalf("rejected decision leaked its payload back into model context:\n%s", encoded)
	}

	directive := subtaskToolCorrectionDirective([]subtaskToolRecord{record})
	for _, required := range []string{
		"DECISION REJECTED",
		"returned 2 tool calls",
		"No tool ran and the workspace was not changed by that response.",
		"Return exactly one workspace.write call now.",
		"Do not include command.run in the same response",
		"After fresh server feedback",
	} {
		if !strings.Contains(directive, required) {
			t.Fatalf("decision correction missing %q:\n%s", required, directive)
		}
	}
}

func TestRejectedToolFeedbackIsDirectAndOmitsBadPayload(t *testing.T) {
	records := []subtaskToolRecord{{
		Call: artifacts.ToolCallArtifact{Tool: "workspace.write", Input: map[string]any{"content": "BAD CONTENT MUST NOT BE REPEATED"}},
		Result: artifacts.ToolResultArtifact{
			Tool: "workspace.write", Accepted: false,
			Summary: "workspace.write create target already exists: go.mod",
			Error:   "workspace.write create target already exists: go.mod",
		},
	}}

	encoded, err := marshalToolRecords(records)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encoded, "BAD CONTENT MUST NOT BE REPEATED") {
		t.Fatalf("prompt records repeated a rejected payload:\n%s", encoded)
	}
	directive := subtaskToolCorrectionDirective(records)
	for _, required := range []string{
		"CORRECTION REQUIRED",
		"Your last workspace.write call was rejected.",
		"Reason: workspace.write create target already exists: go.mod",
		"Do not repeat that call.",
		"No workspace mutation or successful command resulted from it.",
		"Next action:",
	} {
		if !strings.Contains(directive, required) {
			t.Fatalf("correction directive missing %q:\n%s", required, directive)
		}
	}
}

func TestFailedCommandFeedbackIncludesObservedOutputAndNextAction(t *testing.T) {
	records := []subtaskToolRecord{{
		Call: artifacts.ToolCallArtifact{Tool: "command.run", Input: map[string]any{
			"program": "go", "args": []string{"test", "./..."},
		}},
		Result: artifacts.ToolResultArtifact{
			Tool:     "command.run",
			Accepted: true,
			Summary:  "command go exit_code=1",
			Output: map[string]any{
				"succeeded": false,
				"exit_code": 1,
				"stderr":    "main.go:12: undefined: Task",
				"stdout":    "FAIL pocket_tasks",
			},
		},
	}}

	directive := subtaskToolCorrectionDirective(records)
	for _, required := range []string{
		"Your last verification command failed",
		"main.go:12: undefined: Task",
		"FAIL pocket_tasks",
		"Write the corrected complete file that fixes the observed failure, then run go test ./... again.",
	} {
		if !strings.Contains(directive, required) {
			t.Fatalf("command correction missing %q:\n%s", required, directive)
		}
	}
}

func TestRepeatedRejectedToolCallIsDetected(t *testing.T) {
	call := artifacts.ToolCallArtifact{Tool: "workspace.write", Input: map[string]any{"content": "same bad content"}}
	rejected := artifacts.ToolResultArtifact{Tool: "workspace.write", Accepted: false, Error: "create target already exists"}
	records := []subtaskToolRecord{{Call: call, Result: rejected}, {Call: call, Result: rejected}}
	if !repeatedRejectedToolCall(records) {
		t.Fatal("identical consecutive rejected calls were not detected")
	}
	records[1].Call.Input = map[string]any{"content": "different content"}
	if repeatedRejectedToolCall(records) {
		t.Fatal("materially different correction was incorrectly treated as a loop")
	}
}

func TestPromptToolHistoryKeepsOnlyRecentCompactResults(t *testing.T) {
	records := make([]subtaskToolRecord, 0, 5)
	for index := 0; index < 5; index++ {
		records = append(records, subtaskToolRecord{
			Call:   artifacts.ToolCallArtifact{Tool: "command.run", Input: map[string]any{"program": "go", "args": []string{"test", "./..."}}},
			Result: artifacts.ToolResultArtifact{Tool: "command.run", Accepted: true, Summary: fmt.Sprintf("result-%d", index), Output: map[string]any{"succeeded": index == 4}},
		})
	}
	encoded, err := marshalToolRecords(records)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(encoded, "result-0") || strings.Contains(encoded, "result-1") {
		t.Fatalf("prompt retained stale tool history:\n%s", encoded)
	}
	for _, expected := range []string{"result-2", "result-3", "result-4"} {
		if !strings.Contains(encoded, expected) {
			t.Fatalf("prompt omitted recent result %q:\n%s", expected, encoded)
		}
	}
}

func TestLiveSubtaskWorkspaceContextRefreshesFromDisk(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module pocket_tasks\n\ngo 1.26.3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	scanner, err := workspace.New(true, root, 100, 6000).Scoped(root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := liveSubtaskWorkspaceContext(scanner, "Build a Go task tracker")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(first, "go 1.26.3") {
		t.Fatalf("initial context omitted exact manifest state:\n%s", first)
	}
	if err := os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	second, err := liveSubtaskWorkspaceContext(scanner, "Build a Go task tracker")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(second, "main.go") || !strings.Contains(second, "func main()") {
		t.Fatalf("refreshed context omitted newly written source:\n%s", second)
	}
}

func TestSubtaskCompletionEvidenceRequiresCurrentWriteTestsAndDocs(t *testing.T) {
	objective := artifacts.Objective{
		ID:                   "build_app",
		Description:          "Build the application",
		RequiresAction:       true,
		RequiredCapabilities: []string{capabilityWorkspaceWrite, capabilityCommandExecute},
		AcceptanceCriteria:   []string{"Includes focused unit tests", "Includes a concise README"},
	}
	writeRecord := subtaskToolRecord{
		Call:   artifacts.ToolCallArtifact{Tool: "workspace.write"},
		Result: artifacts.ToolResultArtifact{Tool: "workspace.write", Accepted: true},
	}
	noTests := subtaskToolRecord{
		Call:   artifacts.ToolCallArtifact{Tool: "command.run", Input: map[string]any{"program": "go", "args": []string{"test", "./..."}}},
		Result: artifacts.ToolResultArtifact{Tool: "command.run", Accepted: true, Output: map[string]any{"succeeded": true, "stdout": "? pocket_tasks [no test files]"}},
	}
	if err := validateSubtaskCompletionEvidence(objective, []subtaskToolRecord{writeRecord, noTests}, "File tree:\n- go.mod\n- main.go"); err == nil || !strings.Contains(err.Error(), "no tests") || !strings.Contains(err.Error(), "README") {
		t.Fatalf("completion gate err=%v, want test and README gaps", err)
	}
	verified := subtaskToolRecord{
		Call:   artifacts.ToolCallArtifact{Tool: "command.run", Input: map[string]any{"program": "go", "args": []string{"test", "./..."}}},
		Result: artifacts.ToolResultArtifact{Tool: "command.run", Accepted: true, Output: map[string]any{"succeeded": true, "stdout": "ok pocket_tasks 0.01s"}},
	}
	workspaceState := "File tree:\n- README.md\n- go.mod\n- main.go\n- task_test.go"
	if err := validateSubtaskCompletionEvidence(objective, []subtaskToolRecord{writeRecord, verified}, workspaceState); err != nil {
		t.Fatalf("complete evidence rejected: %v", err)
	}
}

func TestSubtaskCompletionEvidenceRequiresVerificationAfterLatestWrite(t *testing.T) {
	objective := artifacts.Objective{
		ID:                   "build_app",
		Description:          "Build the application",
		RequiresAction:       true,
		RequiredCapabilities: []string{capabilityWorkspaceWrite, capabilityCommandExecute},
		AcceptanceCriteria:   []string{"Application works"},
	}
	verified := subtaskToolRecord{
		Call:   artifacts.ToolCallArtifact{Tool: "command.run", Input: map[string]any{"program": "go", "args": []string{"test", "./..."}}},
		Result: artifacts.ToolResultArtifact{Tool: "command.run", Accepted: true, Output: map[string]any{"succeeded": true, "stdout": "ok pocket_tasks"}},
	}
	writeRecord := subtaskToolRecord{
		Call:   artifacts.ToolCallArtifact{Tool: "workspace.write"},
		Result: artifacts.ToolResultArtifact{Tool: "workspace.write", Accepted: true},
	}
	err := validateSubtaskCompletionEvidence(objective, []subtaskToolRecord{verified, writeRecord}, "File tree:\n- main.go")
	if err == nil || !strings.Contains(err.Error(), "after the latest workspace write") {
		t.Fatalf("completion gate err=%v, want stale verification rejection", err)
	}
}
