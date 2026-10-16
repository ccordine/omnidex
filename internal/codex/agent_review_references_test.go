package codex

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/llm"
)

func TestAgentReviewReceiptFailureIsRetryableAtProviderBoundary(t *testing.T) {
	task := codingagent.Task{ID: "task-a", Acceptance: []string{"Behavior remains exact."}}
	_, err := mapAgentReviewRunResult(
		codingagent.Assignment{Role: codingagent.Review, Task: &task},
		json.RawMessage(`{"findings":[],"overall_explanation":"","overall_correctness":"patch is incorrect"}`),
	)
	var retryable interface{ Retryable() bool }
	if err == nil || !errors.As(err, &retryable) || !retryable.Retryable() ||
		!strings.Contains(err.Error(), "decode task-local agent review") {
		t.Fatal("an incomplete task-local review session became a terminal task failure")
	}
}

func TestAgentReviewReferencesRestoreExactQuotedPortableFindings(t *testing.T) {
	task := codingagent.Task{ID: "task\"quoted\\identity", Acceptance: []string{"Preserve \"exact\" values, slash \\, and newline\nbytes."}}
	call := codingagent.Assignment{Role: codingagent.Review, Task: &task}
	raw := json.RawMessage(`{"findings":[{"task_id":"task_1","requirement":"criterion_1","file":"","observation":"Observed independent failure."}],"questions":[]}`)
	mapped, err := mapAgentReviewResult(call, raw)
	if err != nil {
		t.Fatal(err)
	}
	var result codingagent.ReviewResult
	if err := json.Unmarshal(mapped, &result); err != nil {
		t.Fatal(err)
	}
	want := codingagent.Finding{TaskID: task.ID, Requirement: task.Acceptance[0], Observation: "Observed independent failure."}
	if !reflect.DeepEqual(result.Findings, []codingagent.Finding{want}) || result.Questions == nil {
		t.Fatalf("mapped review changed exact authority: %+v", result)
	}
	if err := codingagent.ValidateReview(map[string]codingagent.Task{task.ID: task}, result); err != nil {
		t.Fatalf("portable validator rejected mapped exact criterion: %v", err)
	}
}

func TestAgentReviewReferencesPreserveTypedVerification(t *testing.T) {
	task := codingagent.Task{ID: "task-a", Acceptance: []string{"Behavior remains exact."}}
	call := codingagent.Assignment{Role: codingagent.Review, Task: &task}
	mapped, err := mapAgentReviewResult(call, json.RawMessage(`{"findings":[],"questions":[],"verification":"Ran the declared check and an independent edge case."}`))
	if err != nil {
		t.Fatal(err)
	}
	var review codingagent.ReviewResult
	if err := json.Unmarshal(mapped, &review); err != nil {
		t.Fatal(err)
	}
	if review.Verification != "Ran the declared check and an independent edge case." {
		t.Fatalf("task-local review verification lost: %+v", review)
	}
}

func TestAgentReviewReferenceDecoderRejectsUnknownOrMalformedNativeFindings(t *testing.T) {
	task := codingagent.Task{ID: "original-task", Acceptance: []string{"Original criterion."}, RequirementIDs: []string{"parent-a"}}
	call := codingagent.Assignment{Role: codingagent.Review, Task: &task,
		Requirements: []codingagent.Requirement{{ID: "parent-a", Outcome: "Parent outcome."}}}
	for _, raw := range []string{
		`{"findings":[{"task_id":"original-task","requirement":"criterion_1","file":"","observation":"Defect."}],"questions":[]}`,
		`{"findings":[{"task_id":"task_1","requirement":"Original criterion.","file":"","observation":"Defect."}],"questions":[]}`,
		`{"findings":[{"task_id":"task_1","requirement":"parent_1","file":"","observation":"Defect."}],"questions":[]}`,
		`{"findings":[{"task_id":"task_1","requirement":"criterion_9","file":"","observation":"Defect."}],"questions":[]}`,
		`{"findings":[{"task_id":"task_1","requirement":"criterion_1","observation":"Defect."}],"questions":[]}`,
		`{"findings":[{"task_id":"task_1","requirement":"criterion_1","file":"","observation":"Defect.","extra":true}],"questions":[]}`,
		`{"findings":[],"questions":[],"extra":true}`,
		`{"findings":null,"questions":[]}`,
		`{"findings":[],"questions":null}`,
		`{"findings":[],"questions":[]} {}`,
	} {
		if _, err := mapAgentReviewResult(call, json.RawMessage(raw)); err == nil {
			t.Errorf("invalid task-local review references accepted: %s", raw)
		}
	}
}

func TestAgentReviewPromptIncludesOnlyItsExactCriterionReferenceTable(t *testing.T) {
	task := codingagent.Task{ID: `task "identity"`, Acceptance: []string{`Preserve "exact" criterion bytes.`}}
	call := codingagent.Assignment{Role: codingagent.Review, Task: &task}
	prompt, err := renderAgentAssignment(call)
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
	if decoded.Task.ID != task.ID || !reflect.DeepEqual(decoded.Task.Acceptance, task.Acceptance) || decoded.Objective != nil || len(decoded.Requirements) != 0 {
		t.Fatal("task review prompt changed its exact authority or gained parent context")
	}
}

func TestAgentReviewPromptBoundsReferenceTableTogetherWithAssignment(t *testing.T) {
	task := codingagent.Task{ID: "task", Acceptance: []string{strings.Repeat("x", llm.MaxExactPreparedModelInputBytes/2)}}
	call := codingagent.Assignment{Role: codingagent.Review, Task: &task}
	encoded, err := json.Marshal(call)
	if err != nil || len(encoded) >= llm.MaxExactPreparedModelInputBytes {
		t.Fatalf("fixture must fit assignment-only bound: bytes=%d error=%v", len(encoded), err)
	}
	if _, err := renderAgentAssignment(call); err == nil {
		t.Fatal("combined reference table and assignment exceeded context without rejection")
	}
}

func TestAgentReviewMappingLeavesNonReviewResultsUnchanged(t *testing.T) {
	for _, role := range []codingagent.Role{codingagent.Plan, codingagent.Execute} {
		raw := json.RawMessage(`{"questions":[]}`)
		mapped, err := mapAgentReviewResult(codingagent.Assignment{Role: role}, raw)
		if err != nil || string(mapped) != string(raw) {
			t.Fatalf("%s mapping changed non-review response: %s %v", role, mapped, err)
		}
	}
}
