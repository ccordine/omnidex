package queue

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestNativeChannelReplyBindsExactPromptAndCannotEnqueueNewWork(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	command := channelSessionCodingCommand(t, repository)
	command.CodingMode = codingagent.Agents
	initial, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	planner, err := repository.ClaimNextStep(ctx, "native-channel-planner")
	if err != nil || planner == nil {
		t.Fatalf("claim planning: %v", err)
	}
	fixture := codingAgentPlanReviewFixture(t, repository, planner, initial.Job.Instruction)
	freezeCodingAgentPlanFixture(t, repository, initial.Job, fixture)
	executor, err := repository.ClaimNextStep(ctx, "native-channel-executor")
	if err != nil || executor == nil {
		t.Fatalf("claim execution: %v", err)
	}
	task, err := repository.NextIncompleteCodingLedgerTask(ctx, executor.Authority)
	if err != nil || task == nil {
		t.Fatalf("select task: %v", err)
	}
	callID, err := repository.CodingAgentEvidence(executor.Authority).Append(ctx, codingagent.Entry{Generation: 1, Phase: "call_opened", Role: codingagent.Execute, TaskID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	id, err := repository.WaitForCodingNativeInput(ctx, executor.Authority, codingagent.NativeInputRequest{
		ID: json.RawMessage(`1`), Method: "item/commandExecution/requestApproval", ThreadID: "thread", TurnID: "turn", Prompt: "Approve this exact command?",
		Choices: []codingagent.NativeInputChoice{{Label: "approve", Response: json.RawMessage(`{"decision":"accept"}`)}},
	}, callID, task.ID)
	if err != nil {
		t.Fatal(err)
	}
	details, err := repository.CurrentJobDetails(ctx, initial.Job.ID)
	if err != nil || details.ExecutionInputID == nil || *details.ExecutionInputID != id {
		t.Fatalf("native prompt identity not projected: %+v, %v", details, err)
	}
	command.OperationID = codingPlanOperationID(t, "native-channel-reply", initial.Job.ID)
	command.Text, command.ExpectedInputID = "approve", &id
	wrongID := id + 1
	invalid := command
	invalid.ExpectedInputID = &wrongID
	if _, err := repository.SubmitChannelSessionTurn(ctx, invalid); !errors.Is(err, ErrChannelSessionTurnInvalid) {
		t.Fatalf("stale native prompt was accepted: %v", err)
	}
	result, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil || result.Disposition != model.ChannelSessionTurnFeedback || result.Job.ID != initial.Job.ID || result.Job.CurrentGeneration != 1 {
		t.Fatalf("native reply changed conversation authority: %+v, %v", result, err)
	}
	replay, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil || replay.Applied {
		t.Fatalf("native reply replay duplicated work: %+v, %v", replay, err)
	}
	command.OperationID = codingPlanOperationID(t, "stale-native-channel-reply", initial.Job.ID)
	if _, err := repository.SubmitChannelSessionTurn(ctx, command); !errors.Is(err, ErrChannelSessionTurnInvalid) {
		t.Fatalf("old native answer was routed into other work: %v", err)
	}
}
