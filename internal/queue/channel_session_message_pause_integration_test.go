package queue

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestAgentExecutionRejectsActiveMessageOutsideLedgerBeforeQuestionPause(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	planner, session := messagePausePlannerClaim(t, repository)
	planCommand := codingAgentPlanReviewFixture(t, repository, planner, "The approved interaction works.")
	freezeCodingAgentPlanFixture(t, repository, planner.Job, planCommand)
	executor, err := repository.ClaimNextStep(ctx, "message-question-pause-fixture")
	if err != nil || executor == nil || executor.Step.Action != "v3_coding" {
		t.Fatalf("claim execution step: %#v %v", executor, err)
	}
	questions := []string{"Choose an approved detail."}
	if _, err := repository.CodingAgentEvidence(executor.Authority).Append(ctx, codingagent.Entry{
		Generation: executor.Authority.Generation, Phase: "questions", Questions: questions,
	}); err != nil {
		t.Fatal(err)
	}
	message := observedSessionQuestion(t, session, executor.Job.ID, executor.Authority.Generation)
	if _, err := repository.SubmitChannelSessionTurn(ctx, message); !errors.Is(err, ErrChannelSessionTurnInvalid) {
		t.Fatalf("active message bypassed the revision-bound ledger: %v", err)
	}
	if err := repository.PauseCodingAgentStep(ctx, executor.Authority, questions); err != nil {
		t.Fatalf("rejected active message disrupted the native question pause: %v", err)
	}
	pause, err := repository.ReadCodingAgentPause(ctx, executor.Job.ID, executor.Authority.Generation)
	if err != nil || pause.StepID != executor.Step.ID || pause.WaitingAttempt != executor.Authority.Attempt ||
		!slices.Equal(pause.Questions, questions) {
		t.Fatalf("question pause changed its original authority: %#v %v", pause, err)
	}
}

func messagePausePlannerClaim(t *testing.T, repository *Repository) (*model.ClaimedStep, ChannelSessionTurnCommand) {
	t.Helper()
	ctx := context.Background()
	repository.WithCodingAgents(codingAgentTestConfiguration())
	command := channelSessionCodingCommand(t, repository)
	command.CodingMode = codingagent.Agents
	_, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "message-pause-planner-fixture")
	if err != nil || planner == nil || planner.Step.Action != "v3_coding_plan" {
		t.Fatalf("claim plan step: %#v %v", planner, err)
	}
	return planner, command
}
