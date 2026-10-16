package codex

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/codingagent"
)

// These are unrelated framework fixtures, never a workload implementation or
// instructions fed back into a pilot. Installed-provider qualification verifies
// the semantic boundary instead of asserting only the prompt's wording.
func TestInstalledReviewParserKeepsOptionalDiagnosticsOutsideAcceptance(t *testing.T) {
	client, model := installedAgentClient(t)
	for _, fixture := range []struct {
		name, criterion, response string
		kind                      codingagent.ExecutionResponseKind
	}{
		{"checksum optional profiler", "The command reports the correct SHA-256 digest.", "The required digest check passed against a known SHA-256 vector. No defects. An optional profiler could not run because its executable is absent; required review is complete.", codingagent.ExecutionDone},
		{"checksum required unavailable", "The command reports the correct SHA-256 digest.", "The command could not run, so I could not verify the required digest behavior. The required review remains incomplete.", codingagent.ExecutionError},
		{"restore optional telemetry", "Restoring the backup recovers the original file bytes.", "I restored the backup and compared every original byte successfully. Required verification is complete with no defects. An unrelated telemetry probe failed because no telemetry endpoint is configured.", codingagent.ExecutionDone},
		{"restore required unavailable", "Restoring the backup recovers the original file bytes.", "The restore environment is unavailable. I could not check the required recovery of original bytes; verification is incomplete.", codingagent.ExecutionError},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			task := codingagent.Task{ID: "one-task", Acceptance: []string{fixture.criterion}}
			call := codingagent.Assignment{JobID: 17, Generation: 1, Role: codingagent.ReviewResponse,
				Root: t.TempDir(), Task: &task, Response: fixture.response, Route: codingagent.Route{Provider: "codex", Model: model}}
			result, err := client.Run(ctx, call, nil)
			if err != nil {
				t.Fatal(err)
			}
			var review codingagent.ReviewResponseResult
			if err := json.Unmarshal(result.Content, &review); err != nil || review.Kind != fixture.kind || len(review.Findings) != 0 || review.Verification == "" {
				t.Fatalf("review classification crossed acceptance scope: %s %v", result.Content, err)
			}
			t.Logf("model=%s thread=%s turn=%s parsed=%s", model, result.ThreadID, result.TurnID, result.Content)
		})
	}
}

func TestInstalledPlanningParserSeparatesPlanningPhaseFromTaskDelivery(t *testing.T) {
	client, model := installedAgentClient(t)
	for _, fixture := range []struct{ name, response, forbidden string }{
		{"archive", "For this planning conversation, update only the plan and preserve application files. Future implementation task: build an archive command. Requirement r1: create a ZIP containing the supplied input file. Task t1: archive creation, linked to r1. Acceptance ac1 for t1/r1: the archive contains the original bytes. Required check ch1 for r1/ac1: create an archive and compare extracted bytes.", "update only the plan"},
		{"currency", "Planning-only direction for this turn: do not implement anything yet. Future implementation task: currency conversion. Requirement r1: convert using the supplied exchange rate. Task t1: conversion calculation, linked to r1. Acceptance ac1 for t1/r1: the computed amount matches the supplied rate. Required check ch1 for r1/ac1: calculate known amount/rate pairs and compare results.", "do not implement anything yet"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
			defer cancel()
			route := codingagent.Route{Provider: "codex", Model: model, Effort: "low", ServiceTier: "default"}
			turn := codingagent.PlanningTurn{OperationID: "initial", OwnerTurnID: "owner-initial", Text: "Plan the requested task."}
			journal := &replayPlanningJournal{entries: []codingagent.Entry{{Phase: "planning_response", Notes: []string{turn.OperationID, turn.Text, fixture.response}}}}
			controller := codingagent.Controller{JobID: 17, Generation: 1, Root: t.TempDir(),
				Configuration: codingagent.Configuration{Mode: codingagent.Agents, Planner: route, LedgerParser: route, Executor: route, Reviewer: route},
				Providers:     map[string]codingagent.Provider{"codex": replayPlanningProvider{client: client, t: t}}, Evidence: journal, Environment: replayPlanningEnvironment{},
				Requirements: []codingagent.Requirement{{ID: "objective", Outcome: "Plan the requested " + fixture.name + " task."}}}
			proposal, err := controller.ProposePlanningLedger(ctx, turn)
			if err != nil {
				t.Fatal(err)
			}
			if len(proposal.Ledger.Tasks) != 1 {
				t.Fatalf("planning-only direction became extra work: %+v", proposal.Ledger)
			}
			if err := proposal.Ledger.ValidateDraft(); err != nil || len(proposal.Ledger.AcceptanceCriteria) == 0 || len(proposal.Ledger.Checks) == 0 {
				t.Fatalf("future task requirements lost: %+v %v", proposal.Ledger, err)
			}
			task := proposal.Ledger.Tasks[0]
			if strings.Contains(strings.ToLower(task.Goal+" "+task.DeliveryObligation), fixture.forbidden) {
				t.Fatalf("planning phase restriction leaked into execution: %+v", task)
			}
			turn = codingagent.PlanningTurn{OperationID: "mutation", OwnerTurnID: "owner-mutation", ObservedRevision: proposal.Ledger.Revision, Text: "Add the requested verification reporting to the task."}
			response := "For this planning conversation, " + fixture.forbidden + ". Future task " + string(task.ID) + " delivery obligation: execute the required verification check and report its observed result. Preserve existing requirements and acceptance criteria."
			journal.entries = []codingagent.Entry{{Phase: "planning_response", Notes: []string{turn.OperationID, turn.Text, response}}}
			mutation, err := controller.PlanPlanningMutation(ctx, proposal.Ledger, turn)
			if err != nil || len(mutation.Operations) != 1 || mutation.Operations[0].TargetID != task.ID || mutation.Operations[0].Item.Task == nil {
				t.Fatalf("future task amendment lost: %+v %v", mutation, err)
			}
			amendment, _ := json.Marshal(mutation.Operations[0].Item.Task)
			if strings.Contains(strings.ToLower(string(amendment)), fixture.forbidden) {
				t.Fatalf("planning restriction leaked into task amendment: %s", amendment)
			}
			t.Logf("initial and mutation retained one future task, its criteria and checks, and excluded the planning restriction")
		})
	}
}

func TestInstalledPlanningSandboxRejectsWorkspaceWrites(t *testing.T) {
	client, model := installedAgentClient(t)
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	root := t.TempDir()
	connection, err := startExecutionConnection(ctx, client.command, root)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.close()
	call := codingagent.Assignment{Role: codingagent.Plan, Root: root, Route: codingagent.Route{Model: model}}
	if _, _, err := connection.startAgentThread(call, ""); err != nil {
		t.Fatal(err)
	}
	_, _, policy := agentPermissions(call)
	var response struct {
		ExitCode int `json:"exitCode"`
	}
	_, err = connection.call("command/exec", map[string]any{"command": []string{python, "-B", "-c", "from pathlib import Path; Path('forbidden.txt').write_text('unapproved')"}, "cwd": root, "sandboxPolicy": policy, "timeoutMs": 10000}, &response)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(filepath.Join(root, "forbidden.txt")); !os.IsNotExist(err) || response.ExitCode == 0 {
		t.Fatalf("planning sandbox allowed an unapproved write: exit=%d stat=%v", response.ExitCode, err)
	}
}

func TestInstalledTaskRequestsApprovalForHostWrite(t *testing.T) {
	client, model := installedAgentClient(t)
	root := t.TempDir()
	// TMPDIR is itself writable in workspace-write mode. Keep the target in
	// this test's owned parent, outside both the workspace and temporary root.
	targetDirectory, err := os.MkdirTemp(filepath.Dir(os.Getenv("TMPDIR")), "approval-target-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(targetDirectory); err != nil {
			t.Errorf("retire owned approval target: %v", err)
		}
	})
	target := filepath.Join(targetDirectory, "approved.txt")
	task := codingagent.Task{ID: "capability", Goal: fmt.Sprintf("Write exactly the text qualified to the file %q. This is the complete task.", target), Acceptance: []string{"The specified file contains exactly qualified."}}
	call := codingagent.Assignment{JobID: 17, Generation: 1, Role: codingagent.Execute, Root: root, Task: &task, Route: codingagent.Route{Provider: "codex", Model: model, Effort: "low"}}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	approvals := 0
	result, err := client.Run(ctx, call, func(event codingagent.NativeEvent) error {
		if event.Input == nil {
			return nil
		}
		request := event.Input.Request
		if !strings.Contains(request.Prompt, targetDirectory) {
			return fmt.Errorf("approval escaped the owned fixture: %s", request.Prompt)
		}
		var err error
		event.Input.Response, err = request.Reply("approve")
		if err == nil {
			approvals++
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "qualified" || approvals == 0 || !result.Completed {
		t.Fatalf("native task did not request approval and perform its own write: approvals=%d content=%q error=%v result=%s", approvals, data, err, result.Content)
	}
	t.Logf("native owner approvals=%d thread=%s turn=%s", approvals, result.ThreadID, result.TurnID)
}
