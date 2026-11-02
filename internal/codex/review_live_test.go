package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
)

func TestInstalledSoftwareReview(t *testing.T) {
	if os.Getenv("OMNI_TEST_SOFTWARE_REVIEW") != "1" {
		t.Skip("installed software review not selected")
	}
	client, model := installedAgentClient(t)
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	call := codingagent.Assignment{
		JobID: 65, Generation: 1, Root: root, Role: codingagent.Review,
		Route:           codingagent.Route{Provider: "codex", Model: model, Effort: "xhigh", ServiceTier: "default"},
		Task:            &codingagent.Task{Goal: "Omnidex review now checks whether execution correctly implemented the intended change in the resulting software, without ledger-scoping or source-only review instructions.", Objective: "The review prompt was simplified to the executed change description and execution report followed by: You are to make sure that this was executed and implemented correctly. The existing response parser, acceptance and queue behavior are unchanged."},
		ExecutionReport: &codingagent.ExecutionReport{CallID: 1, Round: 1, Response: "Changed internal/codex/agent_prompt.go and internal/codingagent/prompt_context.go and affected protocol expectations. Review no longer receives the ledger task contract or custom developer instructions. Existing full disk access remains. Focused codex/codingagent checks and the core build passed; the changed core is installed. The worktree contains retained approved changes."},
	}
	reportName := "review-software-pilot-20261102.txt"
	if os.Getenv("OMNI_TEST_SOFTWARE_REVIEW_GOAL") == "1" {
		call.Objective = &codingagent.Objective{Instruction: call.Task.Goal, Clarifications: []string{"The intended change is unrestricted software review; keep the response parser, handoff and acceptance process unchanged."}}
		call.Task = nil
		reportName = "review-software-goal-pilot-20261102.txt"
	}
	result, err := client.Run(t.Context(), call, func(codingagent.NativeEvent) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed {
		t.Fatal("native review did not finish")
	}
	var response string
	if err := json.Unmarshal(result.Content, &response); err != nil || response == "" {
		t.Fatal("native review returned no report", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", reportName), []byte(response), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("Native review completed: thread=%s turn=%s; report saved under bin.", result.ThreadID, result.TurnID)
}
