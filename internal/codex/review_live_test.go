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
		Task:            &codingagent.Task{Goal: "Omnidex review invokes Codex's actual /review operation."},
		ExecutionReport: &codingagent.ExecutionReport{CallID: 1, Round: 1, Response: "Changed review dispatch from ordinary turn/start to native review/start using the same settled review instruction as custom target instructions. The execution dispatcher, review prompt, parser, and ledger loop are unchanged. Native review exit reports are collected and retained-turn recovery reads the same report. Reviewer model, effort, and permissions are inherited from the independent thread. Affected protocol and lifecycle tests passed. The worktree contains the integrated change."},
	}
	reportName := "native-review-dispatch-pilot-20261102.txt"
	if os.Getenv("OMNI_TEST_SOFTWARE_REVIEW_GOAL") == "1" {
		call.Objective = &codingagent.Objective{Instruction: call.Task.Goal, Clarifications: []string{"The intended change is unrestricted software review; keep the response parser, handoff and acceptance process unchanged."}}
		call.Task = nil
		reportName = "native-review-dispatch-goal-pilot-20261102.txt"
	}
	enteredReview, exitedReview := false, false
	result, err := client.Run(t.Context(), call, func(event codingagent.NativeEvent) error {
		if len(event.Frame) == 0 {
			return nil
		}
		var frame struct {
			Method string `json:"method"`
			Params struct {
				Item struct {
					Type string `json:"type"`
				} `json:"item"`
			} `json:"params"`
		}
		if err := json.Unmarshal(event.Frame, &frame); err != nil {
			return err
		}
		if frame.Method == "item/started" || frame.Method == "item/completed" {
			enteredReview = enteredReview || frame.Params.Item.Type == "enteredReviewMode"
			exitedReview = exitedReview || frame.Params.Item.Type == "exitedReviewMode"
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Completed {
		t.Fatal("native review did not finish")
	}
	if !enteredReview || !exitedReview {
		t.Fatal("Codex did not enter and exit native /review mode")
	}
	var response string
	if err := json.Unmarshal(result.Content, &response); err != nil || response == "" {
		t.Fatal("native review returned no report", err)
	}
	if err := os.WriteFile(filepath.Join(root, "bin", reportName), []byte(response), 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("Codex entered and exited /review; native turn completed: thread=%s turn=%s; report saved under bin.", result.ThreadID, result.TurnID)
}
