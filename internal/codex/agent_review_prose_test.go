package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestNativeReviewPreservesOrdinaryFinalText(t *testing.T) {
	for _, phase := range []string{"", "final_answer"} {
		text := "I checked the container over SSH.\n\n**Question:** Which version should be running? →"
		var result codingagent.Result
		if err := collectAgentResultItem(&result, codingagent.Review, "agentMessage", phase, text); err != nil {
			t.Fatal(err)
		}
		var response string
		if err := json.Unmarshal(result.Content, &response); err != nil || response != text {
			t.Fatalf("ordinary review changed: %s, %v", result.Content, err)
		}
	}
}

func TestNativeReviewHasNoStructuredPromptAndParserHasOnlyResponseAndCriteria(t *testing.T) {
	task := codingagent.Task{ID: "task", Goal: "Review this exact task", Acceptance: []string{"The declared behavior works."}, ReadFiles: []string{"private-context.go"}}
	review, err := renderAgentAssignment(codingagent.Assignment{Role: codingagent.Review, Task: &task})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"JSON Schema", "reference table", "criterion_1", "Return only"} {
		if strings.Contains(review, forbidden) {
			t.Fatalf("review retains structured output instructions: %s", review)
		}
	}
	call := codingagent.Assignment{Role: codingagent.ReviewResponse, Task: &task, Response: "No defects found after verification."}
	parser, err := renderAgentContext(call)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(parser, task.Goal) || strings.Contains(parser, "private-context.go") || !strings.Contains(parser, task.Acceptance[0]) || strings.Contains(parser, call.Response) {
		t.Fatalf("parser context is not limited to the response and criterion mapping: %s", parser)
	}
	prompt, err := renderAgentAssignment(call)
	if err != nil || !strings.HasSuffix(prompt, call.Response) || strings.Contains(prompt, task.Goal) || strings.Contains(prompt, "private-context.go") || strings.Contains(prompt, task.Acceptance[0]) {
		t.Fatalf("parser current input lost its source or inherited shared task context: %s %v", prompt, err)
	}
}
