package codex

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/llm"
)

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

func TestAgentReviewReferencesAcceptNativeEnvelopeAndPreserveDetails(t *testing.T) {
	task := codingagent.Task{ID: "task-a", Acceptance: []string{"Behavior remains exact."}}
	call := codingagent.Assignment{Role: codingagent.Review, Task: &task}
	receipt := `{"findings":[{"task_id":"task_1","requirement":"criterion_1","file":"","observation":"Observed mismatch."}],"questions":[]}`
	envelope, err := json.Marshal(map[string]any{
		"findings":            []map[string]any{{"title": "Exact behavior differs", "body": "The returned bytes do not match."}},
		"overall_correctness": "patch is incorrect", "overall_explanation": receipt,
	})
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := mapAgentReviewResult(call, envelope)
	if err != nil {
		t.Fatal(err)
	}
	var review codingagent.ReviewResult
	if err := json.Unmarshal(mapped, &review); err != nil {
		t.Fatal(err)
	}
	if len(review.Findings) != 1 || review.Findings[0].TaskID != "task-a" || review.Findings[0].Requirement != "Behavior remains exact." || !strings.Contains(review.Findings[0].Observation, "returned bytes") {
		t.Fatalf("native review detail or references lost: %+v", review)
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
		t.Fatalf("native review verification lost: %+v", review)
	}
}

func TestAgentReviewReferencesAcceptNativeReviewCommentHeading(t *testing.T) {
	task := codingagent.Task{ID: "schedule", Acceptance: []string{"Total duration is exact."}}
	call := codingagent.Assignment{Role: codingagent.Review, Task: &task}
	raw := json.RawMessage(`{"findings":[{"task_id":"task_1","requirement":"criterion_1","file":"schedule.go:61","observation":"The total wraps."}],"questions":[]}

Review comment:

- [P2] Prevent overflow in total_duration — schedule.go:61`)
	mapped, err := mapAgentReviewResult(call, raw)
	if err != nil {
		t.Fatal(err)
	}
	var review codingagent.ReviewResult
	if err := json.Unmarshal(mapped, &review); err != nil {
		t.Fatal(err)
	}
	if len(review.Findings) != 1 || !strings.Contains(review.Findings[0].Observation, "Prevent overflow") {
		t.Fatalf("native review comment was lost: %+v", review)
	}
}

func TestAgentReviewReferencesRejectUnreferencedNativeFinding(t *testing.T) {
	task := codingagent.Task{ID: "task-a", Acceptance: []string{"Behavior remains exact."}}
	call := codingagent.Assignment{Role: codingagent.Review, Task: &task}
	envelope, err := json.Marshal(map[string]any{
		"findings":            []map[string]any{{"title": "Unbound defect", "body": "Missing from the receipt."}},
		"overall_correctness": "patch is incorrect",
		"overall_explanation": `{"findings":[],"questions":[]}`,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mapAgentReviewResult(call, envelope); err == nil || !strings.Contains(err.Error(), "omitted referenced findings") {
		t.Fatalf("unreferenced native finding accepted: %v", err)
	}
}

func TestAgentOverallReviewReferencesRestoreOnlyBoundParentOutcome(t *testing.T) {
	first := codingagent.Task{ID: `task "first"`, Acceptance: []string{`First "exact" criterion.`}, RequirementIDs: []string{"parent-a"}}
	second := codingagent.Task{ID: `task "second"`, Acceptance: []string{`Second "exact" criterion.`}, RequirementIDs: []string{"parent-b"}}
	parents := []codingagent.Requirement{{ID: "parent-a", Outcome: `First "exact" outcome.`}, {ID: "parent-b", Outcome: `Second "exact" outcome.`}}
	call := codingagent.Assignment{Role: codingagent.OverallReview, Workload: &codingagent.Workload{Tasks: []codingagent.Task{first, second}}, Requirements: parents}
	raw := json.RawMessage(`{"findings":[{"task_id":"task_2","requirement":"parent_2","file":"","observation":"Observed bound outcome failure."}],"questions":["Which supplied option is approved?"]}`)
	mapped, err := mapAgentReviewResult(call, raw)
	if err != nil {
		t.Fatal(err)
	}
	var result codingagent.ReviewResult
	if err := json.Unmarshal(mapped, &result); err != nil {
		t.Fatal(err)
	}
	if result.Findings[0].TaskID != second.ID || result.Findings[0].Requirement != parents[1].Outcome ||
		!reflect.DeepEqual(result.Questions, []string{"Which supplied option is approved?"}) {
		t.Fatalf("overall mapping changed task/outcome/question bytes: %+v", result)
	}
	if err := codingagent.ValidateReview(map[string]codingagent.Task{first.ID: first, second.ID: second}, result, parents...); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"findings":[{"task_id":"task_2","requirement":"parent_1","file":"","observation":"Unbound parent."}],"questions":[]}`,
		`{"findings":[{"task_id":"task_1","requirement":"parent_2","file":"","observation":"Sibling parent."}],"questions":[]}`,
	} {
		if _, err := mapAgentReviewResult(call, json.RawMessage(raw)); err == nil {
			t.Fatal("overall review accepted a parent outside the referenced task's bindings")
		}
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
			t.Errorf("invalid native review references accepted: %s", raw)
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
