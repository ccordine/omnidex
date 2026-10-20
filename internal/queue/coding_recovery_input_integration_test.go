package queue

import (
	"errors"
	"github.com/gryph/omnidex/internal/codingagent"
	"testing"
)

func TestFrozenRecoveryCannotAnswerOutstandingInput(t *testing.T) {
	for _, kind := range []string{"question", "permission"} {
		t.Run(kind, func(t *testing.T) {
			_, repo := freshUserIntentRepository(t)
			ctx := t.Context()
			command, fixture := userIntentFixture(t, repo, "Retain unanswered input.")
			start, _ := saveIntent(t, repo, command, fixture.created.Revision, "Approve and start.", statePatch("/items/0/review_state", "agreed"), statePatch("/plan_state", "frozen"))
			if _, err := repo.ExecuteUserIntent(ctx, start.OperationID, command.ChannelID, command.WorkspaceIdentity); err != nil {
				t.Fatal(err)
			}
			claim, err := repo.ClaimNextStep(ctx, "input-fixture")
			if err != nil || claim == nil {
				t.Fatal(err)
			}
			task, err := repo.NextIncompleteCodingLedgerTask(ctx, claim.Authority)
			if err != nil || task == nil {
				t.Fatal(err)
			}
			provider := &recoveryRuntime{mode: kind, effects: map[codingagent.Role]int{}, parses: map[codingagent.Role]int{}}
			controller := codingagent.Controller{JobID: claim.Job.ID, Generation: claim.Job.CurrentGeneration, Root: command.WorkspaceRoot, Configuration: repo.codingAgents, Environment: provider, Evidence: repo.CodingAgentEvidence(claim.Authority), Providers: map[string]codingagent.Provider{"codex": provider}}
			var input *codingagent.ExecutionInputRequired
			if err := controller.RunTask(ctx, *task); !errors.As(err, &input) {
				t.Fatalf("missing input %v", err)
			}
			if err := repo.WaitForCodingExecutionInput(ctx, claim.Authority, task.ID); err != nil {
				t.Fatal(err)
			}
			ledger, _, err := repo.CurrentCodingLedgerForJob(ctx, claim.Job.ID)
			if err != nil {
				t.Fatal(err)
			}
			resume := ResumeJobCommand{OperationID: recoveryOperationID(t), JobID: claim.Job.ID, ExpectedGeneration: claim.Job.CurrentGeneration, ObservedLedgerRevision: ledger.Revision, Feedback: "Continue.", WorkspaceRoot: command.WorkspaceRoot, WorkspaceIdentity: command.WorkspaceIdentity}
			if _, err := repo.ResumeJob(ctx, resume); err == nil {
				t.Fatal("resume bypassed unanswered input")
			}
			state, err := repo.ChannelSessionState(ctx, command.ChannelID, command.WorkspaceIdentity)
			if err != nil || state.LatestJob == nil || state.LatestJob.Status != "waiting_input" {
				t.Fatalf("input boundary changed %+v %v", state, err)
			}
		})
	}
}
