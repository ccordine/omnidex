package queue

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestPlanningNativeQuestionAndContextInterruptionUseOwnerConversation(t *testing.T) {
	for _, scenario := range []struct{ native, direct bool }{{native: true}, {}, {direct: true}} {
		native := scenario.native
		name := "context exhaustion before first ledger"
		if native {
			name = "native question"
		}
		if scenario.direct {
			name += " direct feedback"
		}
		t.Run(name, func(t *testing.T) {
			_, repository := freshCodingPlanRepository(t)
			ctx := context.Background()
			command := channelSessionCodingCommand(t, repository)
			command.CodingMode = codingagent.Agents
			initial, err := repository.SubmitChannelSessionTurn(ctx, command)
			if err != nil {
				t.Fatal(err)
			}
			claim, err := repository.ClaimNextStep(ctx, "planning-input-worker")
			if err != nil || claim == nil || claim.Step.Action != "v3_coding_plan" {
				t.Fatalf("claim: %#v %v", claim, err)
			}
			store := repository.CodingAgentEvidence(claim.Authority)
			callID, err := store.Append(ctx, codingagent.Entry{Generation: 1, Phase: "call_opened", Role: codingagent.Plan})
			if err != nil {
				t.Fatal(err)
			}
			var inputID int64
			if native {
				inputID, err = repository.WaitForCodingNativeInput(ctx, claim.Authority, codingagent.NativeInputRequest{ID: json.RawMessage(`1`), Method: "item/tool/requestUserInput", ThreadID: "planning-thread", TurnID: "planning-turn", Prompt: "Which system?", Questions: []codingagent.NativeInputQuestion{{ID: "system", Text: "Which system?"}}}, callID, "")
			} else {
				_, err = store.Append(ctx, codingagent.Entry{Generation: 1, Phase: "planning_incomplete", Role: codingagent.Plan, CallID: callID, ResponseKind: codingagent.ExecutionQuestion, Notes: []string{"Planning context is exhausted. How should planning continue?"}})
				if err == nil {
					err = repository.WaitForCodingExecutionInput(ctx, claim.Authority, "")
				}
			}
			if err != nil {
				t.Fatal(err)
			}
			details, err := repository.CurrentJobDetails(ctx, initial.Job.ID)
			if err != nil {
				t.Fatal(err)
			}
			command.OperationID, command.Text = codingPlanOperationID(t, "planning-input-reply", initial.Job.ID), "Use the already approved system."
			if !native {
				raw, err := os.ReadFile("../../testdata/planning-gauntlet/continuation-authority.json")
				if err != nil {
					t.Fatal(err)
				}
				var capture struct {
					OwnerReply string `json:"owner_reply"`
				}
				if err := json.Unmarshal(raw, &capture); err != nil || capture.OwnerReply == "" {
					t.Fatalf("captured ordinary continuation reply: %v", err)
				}
				command.Text = capture.OwnerReply
			}
			if native {
				if details.CodingInputDisposition != model.CodingInputReply {
					t.Fatalf("native planning question was not a reply: %+v", details)
				}
				if details.ExecutionInputID == nil || *details.ExecutionInputID != inputID {
					t.Fatalf("planning prompt identity not projected: %+v", details)
				}
				command.ExpectedInputID = &inputID
				if _, err := repository.RenewStepAttempt(ctx, claim.Authority); err != nil {
					t.Fatal(err)
				}
			}
			var result ChannelSessionTurnResult
			if scenario.direct {
				feedback, submitErr := repository.SubmitJobFeedback(ctx, SubmitJobFeedbackCommand{OperationID: command.OperationID, JobID: initial.Job.ID,
					ExpectedGeneration: &initial.Job.CurrentGeneration, ExpectedWaitingAttempt: &claim.Authority.Attempt,
					Feedback: command.Text, WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity})
				err = submitErr
				result = ChannelSessionTurnResult{Job: feedback.Job, Disposition: model.ChannelSessionTurnReplanned}
			} else {
				result, err = repository.SubmitChannelSessionTurn(ctx, command)
			}
			if err != nil || result.Job.ID != initial.Job.ID {
				t.Fatalf("reply: %+v %v", result, err)
			}
			if native {
				if result.Disposition != model.ChannelSessionTurnFeedback || result.Job.CurrentGeneration != 1 {
					t.Fatalf("native question triggered a replan: %+v", result)
				}
				_, found, err := repository.CodingNativeInputReply(ctx, claim.Authority, inputID)
				if err != nil || !found {
					t.Fatalf("reply did not reach native planning: %v %v", found, err)
				}
				if err := repository.RequireActiveStepAttempt(ctx, claim.Authority); err != nil {
					t.Fatal(err)
				}
			} else if result.Disposition != model.ChannelSessionTurnReplanned || result.Job.CurrentGeneration != 2 {
				t.Fatalf("interrupted planning did not return to a fresh planning generation: %+v", result)
			}
			if scenario.direct {
				if replay, err := repository.SubmitJobFeedback(ctx, SubmitJobFeedbackCommand{OperationID: command.OperationID, JobID: initial.Job.ID,
					ExpectedGeneration: &initial.Job.CurrentGeneration, ExpectedWaitingAttempt: &claim.Authority.Attempt,
					Feedback: command.Text, WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}); err != nil || replay.Applied {
					t.Fatalf("direct reply replay: %+v %v", replay, err)
				}
			} else if replay, err := repository.SubmitChannelSessionTurn(ctx, command); err != nil || replay.Applied {
				t.Fatalf("reply replay: %+v %v", replay, err)
			}
			if !native {
				continuity, err := repository.ObjectiveContinuityAuthorities(ctx, result.Job, replanCodingBoundary)
				if err != nil || continuity.Session == nil || len(continuity.Session.Turns) != 1 || continuity.Replan == nil || continuity.Replan.Feedback != command.Text {
					t.Fatalf("one owner reply became competing continuation authorities: %+v %v", continuity, err)
				}
				feedback, err := continuity.CodingFeedback()
				if err != nil || len(feedback) != 1 || feedback[0] != "user redirect:\n"+command.Text {
					t.Fatalf("continuation duplicated or lost exact owner context: %v %v", feedback, err)
				}
			}
		})
	}
}

func TestUnchangedContinuationProposalStillRequiresExplicitOwnerFreeze(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	job, err := repository.EnqueueCodingJobMode(ctx, "Finish only the approved task.", t.TempDir(), codingagent.Agents)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "unchanged-planner")
	if err != nil || planner == nil {
		t.Fatalf("claim: %v", err)
	}
	fixture := codingAgentPlanReviewFixture(t, repository, planner, job.Instruction)
	frozen := freezeCodingAgentPlanFixture(t, repository, job, fixture)
	root, identity := codingPlanWorkspaceAuthority(t, job)
	replanned, err := repository.ReplanJob(ctx, ReplanJobCommand{OperationID: codingPlanOperationID(t, "unchanged-replan", job.ID), JobID: job.ID, Feedback: "Discuss continuation without changing the scope.", ObservedLedgerRevision: frozen.Revision, WorkspaceRoot: root, WorkspaceIdentity: identity})
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "unchanged-continuation-planner")
	if err != nil || claim == nil {
		t.Fatalf("claim: %v", err)
	}
	proposed, err := repository.ProposeUnchangedCodingLedger(ctx, ProposeUnchangedCodingLedgerCommand{LedgerID: frozen.LedgerID, Revision: frozen.Revision, OperationID: "unchanged-proposal", OwnerTurnID: "unchanged-proposal-turn", OwnerTurnText: "Discuss continuation without changing the scope."})
	if err != nil || proposed.State != "proposed" || proposed.Revision != frozen.Revision+1 {
		t.Fatalf("proposal: %+v %v", proposed, err)
	}
	if proposed.Ledger.Tasks[0].Goal != frozen.Ledger.Tasks[0].Goal || len(proposed.Ledger.Tasks) != 1 {
		t.Fatal("unchanged proposal invented work")
	}
	if err := repository.MarkCodingLedgerReviewReady(ctx, claim.Authority, proposed.LedgerID, proposed.Revision); err != nil {
		t.Fatal(err)
	}
	if next, err := repository.ClaimNextStep(ctx, "cannot-claim-unapproved-unchanged"); err != nil || next != nil {
		t.Fatalf("unchanged proposal auto-approved: %#v %v", next, err)
	}
	frozenAgain, err := repository.FreezeCodingLedger(ctx, FreezeCodingLedgerCommand{LedgerID: proposed.LedgerID, Revision: proposed.Revision, OperationID: "unchanged-freeze", OwnerTurnID: "unchanged-freeze-turn", OwnerTurnText: "Approve the unchanged ledger."})
	if err != nil {
		t.Fatal(err)
	}
	if err := repository.CompleteCodingLedgerReview(ctx, replanned.Job.ID, frozenAgain.LedgerID, frozenAgain.Revision); err != nil {
		t.Fatal(err)
	}
}
