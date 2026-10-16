package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestExecutionOwnerReplyRetainsTaskGenerationAndCorrectAttemptLifecycle(t *testing.T) {
	for _, test := range []struct{ native, review bool }{{}, {native: true}, {review: true}} {
		native := test.native
		name := "final prose question"
		if native {
			name = "live native permission"
		}
		if test.review {
			name = "review prose question"
		}
		t.Run(name, func(t *testing.T) {
			pool, repository := freshCodingPlanRepository(t)
			ctx := context.Background()
			job, err := repository.EnqueueCodingJobMode(ctx, "Perform the approved task.", t.TempDir(), codingagent.Agents)
			if err != nil {
				t.Fatal(err)
			}
			planner, err := repository.ClaimNextStep(ctx, "input-planner")
			if err != nil || planner == nil {
				t.Fatalf("claim planning: %v", err)
			}
			fixture := codingAgentPlanReviewFixture(t, repository, planner, job.Instruction)
			freezeCodingAgentPlanFixture(t, repository, job, fixture)
			executor, err := repository.ClaimNextStep(ctx, "input-executor")
			if err != nil || executor == nil {
				t.Fatalf("claim execution: %v", err)
			}
			task, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
			if err != nil || task == nil {
				t.Fatalf("select task: %v", err)
			}
			store := repository.CodingAgentEvidence(executor.Authority)
			role, phase := codingagent.Execute, "execution_response"
			if test.review {
				role, phase = codingagent.Review, "review_response"
			}
			callID, err := store.Append(ctx, codingagent.Entry{Generation: job.CurrentGeneration, Phase: "call_opened", Role: role, TaskID: task.ID})
			if err != nil {
				t.Fatal(err)
			}
			var inputID int64
			if native {
				request := codingagent.NativeInputRequest{ID: json.RawMessage(`"request-1"`), Method: "item/commandExecution/requestApproval",
					ThreadID: "task-thread", TurnID: "task-turn", Prompt: "Permission requested. Reply approve or deny.",
					Choices: []codingagent.NativeInputChoice{{Label: "approve", Response: json.RawMessage(`{"decision":"accept"}`)}, {Label: "deny", Response: json.RawMessage(`{"decision":"decline"}`)}}}
				inputID, err = repository.WaitForCodingNativeInput(ctx, executor.Authority, request, callID, task.ID)
			} else {
				_, err = store.Append(ctx, codingagent.Entry{Generation: job.CurrentGeneration, Phase: phase, Role: role, TaskID: task.ID,
					CallID: callID, ResponseKind: codingagent.ExecutionQuestion, Notes: []string{"Which target?"}})
				if err == nil {
					err = repository.WaitForCodingExecutionInput(ctx, executor.Authority, task.ID)
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := repository.RequireActiveStepAttempt(ctx, executor.Authority); (err == nil) != native {
				t.Fatalf("waiting attempt authority: native=%v error=%v", native, err)
			}
			if native {
				if _, err := repository.RenewStepAttempt(ctx, executor.Authority); err != nil {
					t.Fatalf("native prompt lost its process lease: %v", err)
				}
			}
			root, identity := codingPlanWorkspaceAuthority(t, job)
			command := SubmitJobFeedbackCommand{OperationID: codingPlanOperationID(t, "input-reply", job.ID), JobID: job.ID,
				ExpectedGeneration: &job.CurrentGeneration, ExpectedWaitingAttempt: &executor.Authority.Attempt, Feedback: "owner-target", WorkspaceRoot: root, WorkspaceIdentity: identity}
			if native {
				command.ExpectedInputID = &inputID
				if _, err := repository.SubmitJobFeedback(ctx, command); !errors.Is(err, ErrStepNotWritable) {
					t.Fatalf("ambiguous permission reply accepted: %v", err)
				}
				command.Feedback = "approve"
			}
			result, err := repository.SubmitJobFeedback(ctx, command)
			if err != nil || result.Job.ID != job.ID || result.Job.CurrentGeneration != job.CurrentGeneration || result.Job.Status != model.JobStatusRunning {
				t.Fatalf("reply changed job/ledger generation: %+v, %v", result, err)
			}
			replay, err := repository.SubmitJobFeedback(ctx, command)
			if err != nil || replay.Applied {
				t.Fatalf("reply replay duplicated effect: %+v, %v", replay, err)
			}
			claimed, err := repository.ClaimNextStep(ctx, "reply-executor")
			if err != nil {
				t.Fatal(err)
			}
			if native {
				if claimed != nil {
					t.Fatal("another worker claimed a still-live native turn")
				}
				response, found, err := repository.CodingNativeInputReply(ctx, executor.Authority, inputID)
				if err != nil || !found || string(response) != `{"decision": "accept"}` {
					t.Fatalf("native owner response lost: %s, %v, %v", response, found, err)
				}
			} else if claimed == nil || claimed.Authority.Attempt != executor.Authority.Attempt+1 {
				t.Fatalf("closed prose question did not open the next exact attempt: %#v", claimed)
			}
			if test.review {
				var replyRole string
				if err := pool.QueryRow(ctx, `SELECT payload_json->>'role' FROM evidence WHERE job_id=$1 AND payload_json->>'phase'='execution_reply' ORDER BY id DESC LIMIT 1`, job.ID).Scan(&replyRole); err != nil || replyRole != "review" {
					t.Fatalf("review reply was routed to execution: role=%q error=%v", replyRole, err)
				}
			}
			var accepted int
			if err := pool.QueryRow(ctx, `SELECT count(*) FROM coding_agent_tasks WHERE state='accepted'`).Scan(&accepted); err != nil || accepted != 0 {
				t.Fatalf("input reply accepted unfinished work: %d, %v", accepted, err)
			}
		})
	}
}

func TestNativeOwnerWaitRetainsExplicitCancellation(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJobMode(ctx, "Perform the approved task.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "cancel-native-planner")
	if err != nil || planner == nil {
		t.Fatalf("claim planning: %v", err)
	}
	fixture := codingAgentPlanReviewFixture(t, repository, planner, job.Instruction)
	freezeCodingAgentPlanFixture(t, repository, job, fixture)
	executor, err := repository.ClaimNextStep(ctx, "cancel-native-executor")
	if err != nil || executor == nil {
		t.Fatalf("claim execution: %v", err)
	}
	task, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil || task == nil {
		t.Fatalf("select task: %v", err)
	}
	callID, err := repository.CodingAgentEvidence(executor.Authority).Append(ctx, codingagent.Entry{Generation: job.CurrentGeneration, Phase: "call_opened", Role: codingagent.Execute, TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repository.WaitForCodingNativeInput(ctx, executor.Authority, codingagent.NativeInputRequest{ID: json.RawMessage(`1`), ThreadID: "thread", TurnID: "turn", Method: "item/tool/requestUserInput", Prompt: "Which target?"}, callID, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	root, identity := codingPlanWorkspaceAuthority(t, job)
	_, err = repository.CancelJob(ctx, CancelJobCommand{OperationID: codingPlanOperationID(t, "cancel-native", job.ID), JobID: job.ID, Reason: "owner cancelled", WorkspaceRoot: root, WorkspaceIdentity: identity})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.RequireActiveStepAttempt(ctx, executor.Authority); !errors.Is(err, ErrStaleStepAttempt) {
		t.Fatalf("cancelled native request retained authority: %v", err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM job_step_attempts WHERE step_id=$1 AND attempt=$2`, executor.Step.ID, executor.Authority.Attempt).Scan(&status); err != nil || status != string(model.StepAttemptCanceled) {
		t.Fatalf("native attempt not cancelled: %s, %v", status, err)
	}
}
