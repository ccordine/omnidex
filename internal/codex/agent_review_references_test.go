package codex

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestReviewParserMalformedOutputFailsExplicitly(t *testing.T) {
	task := codingagent.Task{ID: "task-a", Acceptance: []string{"Behavior remains exact."}}
	_, err := mapAgentReviewResponse(
		codingagent.Assignment{Role: codingagent.ReviewResponse, Task: &task},
		json.RawMessage(`{"kind":"done","findings":[],"overall_explanation":"","overall_correctness":"patch is incorrect"}`),
	)
	if err == nil || !strings.Contains(err.Error(), "review response parser") {
		t.Fatal("malformed parser output was accepted")
	}
}

func TestAgentReviewReferencesRestoreExactQuotedPortableFindings(t *testing.T) {
	task := codingagent.Task{ID: "task\"quoted\\identity", Acceptance: []string{"Preserve \"exact\" values, slash \\, and newline\nbytes."}}
	call := codingagent.Assignment{Role: codingagent.ReviewResponse, Task: &task}
	raw := json.RawMessage(`{"kind":"done","findings":[{"requirement":"criterion_1","file":"","observation":"Observed independent failure."}],"verification":"Inspected the task-local result against the criterion."}`)
	mapped, err := mapAgentReviewResponse(call, raw)
	if err != nil {
		t.Fatal(err)
	}
	var result codingagent.ReviewResponseResult
	if err := json.Unmarshal(mapped, &result); err != nil {
		t.Fatal(err)
	}
	want := codingagent.Finding{TaskID: task.ID, Requirement: task.Acceptance[0], Observation: "Observed independent failure."}
	if !reflect.DeepEqual(result.Findings, []codingagent.Finding{want}) {
		t.Fatalf("mapped review changed exact authority: %+v", result)
	}
	if err := codingagent.ValidateReview(map[string]codingagent.Task{task.ID: task}, result.ReviewResult); err != nil {
		t.Fatalf("portable validator rejected mapped exact criterion: %v", err)
	}
}

func TestAgentReviewReferencesPreserveTypedVerification(t *testing.T) {
	task := codingagent.Task{ID: "task-a", Acceptance: []string{"Behavior remains exact."}}
	call := codingagent.Assignment{Role: codingagent.ReviewResponse, Task: &task}
	mapped, err := mapAgentReviewResponse(call, json.RawMessage(`{"kind":"done","findings":[],"verification":"Ran the declared check and an independent edge case."}`))
	if err != nil {
		t.Fatal(err)
	}
	var review codingagent.ReviewResponseResult
	if err := json.Unmarshal(mapped, &review); err != nil {
		t.Fatal(err)
	}
	if review.Verification != "Ran the declared check and an independent edge case." {
		t.Fatalf("task-local review verification lost: %+v", review)
	}
}

func TestAgentReviewReferenceDecoderRejectsUnknownOrMalformedNativeFindings(t *testing.T) {
	task := codingagent.Task{ID: "original-task", Acceptance: []string{"Original criterion."}, RequirementIDs: []string{"parent-a"}}
	call := codingagent.Assignment{Role: codingagent.ReviewResponse, Task: &task,
		Requirements: []codingagent.Requirement{{ID: "parent-a", Outcome: "Parent outcome."}}}
	for _, raw := range []string{
		`{"kind":"done","findings":[{"task_id":"task_1","requirement":"criterion_1","file":"","observation":"Defect."}],"verification":"checked"}`,
		`{"kind":"done","findings":[{"requirement":"Original criterion.","file":"","observation":"Defect."}],"verification":"checked"}`,
		`{"kind":"done","findings":[{"requirement":"parent_1","file":"","observation":"Defect."}],"verification":"checked"}`,
		`{"kind":"done","findings":[{"requirement":"criterion_9","file":"","observation":"Defect."}],"verification":"checked"}`,
		`{"kind":"done","findings":[{"requirement":"criterion_1","observation":"Defect."}],"verification":"checked"}`,
		`{"kind":"done","findings":[{"requirement":"criterion_1","file":"","observation":"Defect.","extra":true}],"verification":"checked"}`,
		`{"kind":"done","findings":[],"verification":"checked","extra":true}`,
		`{"kind":"done","findings":null,"verification":"checked"}`,
		`{"verification":"checked"}`,
		`{"kind":"done","findings":[],"verification":"checked"} {}`,
	} {
		if _, err := mapAgentReviewResponse(call, json.RawMessage(raw)); err == nil {
			t.Errorf("invalid task-local review references accepted: %s", raw)
		}
	}
}

func TestAgentReviewContextIncludesOnlyItsExactCriterionReferenceTable(t *testing.T) {
	task := codingagent.Task{ID: `task "identity"`, Acceptance: []string{`Preserve "exact" criterion bytes.`}}
	call := codingagent.Assignment{Role: codingagent.ReviewResponse, Task: &task, Response: "An ordinary review response."}
	prompt, err := renderAgentContext(call)
	if err != nil {
		t.Fatal(err)
	}
	_, rest, found := strings.Cut(prompt, "\n\nReview reference table:\n")
	if !found {
		t.Fatal("reviewer lacks the code-owned reference table")
	}
	table, assignment, found := strings.Cut(rest, "\n\nAssignment:\n")
	if !found {
		t.Fatal("reviewer lacks the original assignment boundary")
	}
	var references []agentReviewTaskReference
	if err := json.Unmarshal([]byte(table), &references); err != nil {
		t.Fatal(err)
	}
	want := []agentReviewTaskReference{{Reference: "task_1", TaskID: task.ID,
		Criteria: []agentReviewCriterionReference{{Reference: "criterion_1", Requirement: task.Acceptance[0]}}}}
	if !reflect.DeepEqual(references, want) {
		t.Fatalf("review reference table expanded or altered task authority: %+v", references)
	}
	var decoded codingagent.Assignment
	if err := json.Unmarshal([]byte(assignment), &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Response != "" || decoded.Task != nil || decoded.Objective != nil || len(decoded.Requirements) != 0 {
		t.Fatal("task review prompt changed its exact authority or gained parent context")
	}
	current, err := renderAgentAssignment(call)
	if err != nil || !strings.HasSuffix(current, call.Response) {
		t.Fatal("review parser lost its exact current source response")
	}
}

func TestReviewParserDoesNotInheritTheSemantic128KiBCeiling(t *testing.T) {
	const formerInputCeiling = 128 * 1024
	task := codingagent.Task{ID: "task", Acceptance: []string{strings.Repeat("x", formerInputCeiling)}}
	call := codingagent.Assignment{Role: codingagent.ReviewResponse, Task: &task}
	encoded, err := json.Marshal(call)
	if err != nil || len(encoded) < formerInputCeiling {
		t.Fatalf("fixture must exceed the prompt bound: bytes=%d error=%v", len(encoded), err)
	}
	if _, err := renderAgentContext(call); err != nil {
		t.Fatalf("large review parser input hit an unrelated semantic ceiling: %v", err)
	}
}

func TestAgentReviewMappingLeavesNonReviewResultsUnchanged(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Plan, codingagent.Execute, codingagent.Review} {
		raw := json.RawMessage(`"An ordinary review response."`)
		mapped, err := mapAgentReviewResponse(codingagent.Assignment{Role: role}, raw)
		if err != nil || string(mapped) != string(raw) {
			t.Fatalf("%s mapping changed non-review response: %s %v", role, mapped, err)
		}
	}
}
