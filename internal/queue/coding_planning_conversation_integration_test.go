package queue

import (
	"context"
	"errors"
	"testing"

	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func TestPlanningQueuedRepliesRemainOnTheSameAttemptAndResolveExactlyOnce(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	command := channelSessionCodingCommand(t, repository)
	command.CodingMode = codingagent.Agents
	initial, err := repository.SubmitChannelSessionTurn(ctx, command)
	if err != nil {
		t.Fatal(err)
	}
	claim, err := repository.ClaimNextStep(ctx, "planning-conversation")
	if err != nil || claim == nil {
		t.Fatalf("claim: %v", err)
	}
	first := observedSessionQuestion(t, command, initial.Job.ID, initial.Job.CurrentGeneration)
	first.Text = "Add the monthly total."
	second := observedSessionQuestion(t, command, initial.Job.ID, initial.Job.CurrentGeneration)
	second.Text = "Prioritize input validation before the README."
	for _, reply := range []ChannelSessionTurnCommand{first, second} {
		if _, err := repository.SubmitChannelSessionTurn(ctx, reply); err != nil {
			t.Fatal(err)
		}
		if replay, err := repository.SubmitChannelSessionTurn(ctx, reply); err != nil || replay.Applied {
			t.Fatalf("intake replay: %+v %v", replay, err)
		}
	}
	fixture := codingAgentPlanReviewFixture(t, repository, claim, initial.Job.Instruction)
	if _, err := repository.ResolvePlanningSessionMessage(ctx, claim.Authority, first.OperationID); err == nil {
		t.Fatal("resolved planning message without its durable planner response")
	}
	for _, reply := range []ChannelSessionTurnCommand{first, second} {
		next, err := repository.NextChannelSessionMessage(ctx, claim.Authority)
		if err != nil || next == nil || next.OperationID != reply.OperationID {
			t.Fatalf("FIFO: %+v %v", next, err)
		}
		if err := repository.MarkCodingLedgerReviewReady(ctx, claim.Authority, fixture.created.LedgerID, fixture.created.Revision); !errors.Is(err, ErrPendingChannelSessionMessage) {
			t.Fatalf("waiting lost pending reply: %v", err)
		}
		if _, err := repository.CodingAgentEvidence(claim.Authority).Append(ctx, codingagent.Entry{
			Generation: claim.Authority.Generation, Phase: "planning_response", Role: codingagent.Plan,
			Notes: []string{string(reply.OperationID), reply.Text, "The plan now reflects that request."},
		}); err != nil {
			t.Fatal(err)
		}
		result, err := repository.ResolvePlanningSessionMessage(ctx, claim.Authority, reply.OperationID)
		if err != nil || !result.Applied || result.Replanned || result.ResponseMessage != nil || result.Job.CurrentGeneration != claim.Authority.Generation {
			t.Fatalf("resolve: %+v %v", result, err)
		}
		if replay, err := repository.ResolvePlanningSessionMessage(ctx, claim.Authority, reply.OperationID); err != nil || replay.Applied {
			t.Fatalf("resolution replay: %+v %v", replay, err)
		}
	}
	var responses int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM ai_channel_messages WHERE channel_id=$1 AND role='assistant'`, command.ChannelID).Scan(&responses); err != nil || responses != 2 {
		t.Fatalf("duplicate planner replies: %d %v", responses, err)
	}
	if err := repository.MarkCodingLedgerReviewReady(ctx, claim.Authority, fixture.created.LedgerID, fixture.created.Revision); err != nil {
		t.Fatal(err)
	}
	snapshot, err := repository.ChannelSessionSnapshot(ctx, command.ChannelID, MaxChannelSessionMessages, command.WorkspaceIdentity)
	if err != nil || len(snapshot.Turns) != 3 {
		t.Fatalf("snapshot: %+v %v", snapshot, err)
	}
	for _, turn := range snapshot.Turns[1:] {
		if turn.MessageState != model.ChannelSessionMessagePlanning {
			t.Fatalf("planning reply not resolved: %+v", turn)
		}
	}
}
