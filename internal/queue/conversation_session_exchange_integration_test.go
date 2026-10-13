package queue

import (
	"context"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
)

func TestConversationCandidatesKeepQueuedRepliesBoundToTheirOperations(t *testing.T) {
	for _, kind := range []model.ChannelSessionMessageState{model.ChannelSessionMessageAnswered, model.ChannelSessionMessageStory, model.ChannelSessionMessageFailed} {
		t.Run(string(kind), func(t *testing.T) {
			_, repository := freshCodingPlanRepository(t)
			ctx := context.Background()
			claim, question := activeSessionMessageFixture(t, repository)
			var reply ChannelSessionMessageResult
			var err error
			if kind == model.ChannelSessionMessageFailed {
				reply, err = repository.FailChannelSessionMessage(ctx, FailChannelSessionMessageCommand{Authority: claim.Authority, OperationID: question.OperationID, Error: "The queued response provider is unavailable."})
			} else {
				reply, err = repository.ResolveChannelSessionMessage(ctx, ResolveChannelSessionMessageCommand{Authority: claim.Authority, OperationID: question.OperationID, Kind: assemblyline.ConversationObjectiveKind(kind), Response: "The queued durable response."})
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repository.CancelJob(ctx, CancelJobCommand{OperationID: codingPlanOperationID(t, "cancel-session-history", claim.Job.ID), JobID: claim.Job.ID, Reason: "End only the original task.", WorkspaceRoot: question.WorkspaceRoot, WorkspaceIdentity: question.WorkspaceIdentity}); err != nil {
				t.Fatal(err)
			}
			next := nextSessionHistoryJob(t, repository, question)
			authorities, err := repository.ConversationCandidateAuthorities(ctx, next)
			if err != nil {
				t.Fatalf("queued reply was mistaken for the canceled task's result: %v", err)
			}
			if len(authorities.Turns) != 0 || len(authorities.AssistantResults) != 0 || len(authorities.SessionExchanges) != 1 {
				t.Fatalf("queued history invented a real user/task-result authority: %#v", authorities)
			}
			exchange := authorities.SessionExchanges[0]
			if exchange.OperationID != question.OperationID || exchange.JobID != claim.Job.ID || exchange.Generation != claim.Authority.Generation || exchange.QuestionText != question.Text || exchange.ResponseMessageID != reply.ResponseMessage.ID || exchange.ResponseText != reply.ResponseMessage.Content || exchange.ResolutionKind != kind {
				t.Fatalf("queued exchange lost its exact immutable source: %#v", exchange)
			}
			additional, err := repository.HasAdditionalConversationSearchAuthority(ctx, next, nil, question.OperationID)
			if err != nil || additional {
				t.Fatalf("represented queued exchange remained additional: %t %v", additional, err)
			}
			additional, err = repository.HasAdditionalConversationSearchAuthority(ctx, next, nil)
			if err != nil || !additional {
				t.Fatalf("unrepresented queued exchange is not searchable: %t %v", additional, err)
			}
			records, err := repository.SearchConversationContextRecords(ctx, next, []string{"queued"}, 6)
			if err != nil || len(records) != 1 || records[0].SourceID != exchange.SourceID() {
				t.Fatalf("queued search lost operation-owned identity: %#v %v", records, err)
			}
		})
	}
}

func TestConversationCandidatesKeepMainResultAfterInterposedQueuedReply(t *testing.T) {
	_, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	claim, question := activeSessionMessageFixture(t, repository)
	if _, err := repository.ResolveChannelSessionMessage(ctx, ResolveChannelSessionMessageCommand{Authority: claim.Authority, OperationID: question.OperationID, Kind: assemblyline.ObjectiveKindAnswer, Response: "The queued durable answer."}); err != nil {
		t.Fatal(err)
	}
	const result = "The durable main result."
	if err := repository.CompleteStepWithEvidence(ctx, CompleteStepEvidenceCommand{CompleteStepCommand: CompleteStepCommand{OperationID: codingPlanOperationID(t, "complete-session-history", claim.Job.ID), Authority: claim.Authority, StepID: claim.Step.ID, Output: result, ContextKey: "objective_result"}}); err != nil {
		t.Fatal(err)
	}
	next := nextSessionHistoryJob(t, repository, question)
	authorities, err := repository.ConversationCandidateAuthorities(ctx, next)
	if err != nil || len(authorities.Turns) != 2 || len(authorities.AssistantResults) != 1 || len(authorities.SessionExchanges) != 1 {
		t.Fatalf("interposed queued reply displaced exact main result: %#v %v", authorities, err)
	}
	main := authorities.AssistantResults[0]
	if main.JobID != claim.Job.ID || main.Content != result || main.UserMessageID != authorities.Turns[0].MessageID || authorities.Turns[0].Content != claim.Job.Instruction {
		t.Fatalf("main result lost its original user/job binding: %#v", authorities)
	}
	additional, err := repository.HasAdditionalConversationSearchAuthority(ctx, next, []int64{main.UserMessageID}, question.OperationID)
	if err != nil || additional {
		t.Fatalf("interposed reply poisoned completed exchange validation: %t %v", additional, err)
	}
	records, err := repository.SearchConversationContextRecords(ctx, next, []string{"durable"}, 6)
	if err != nil || len(records) != 2 {
		t.Fatalf("main and queued exchanges were not separately searchable: %#v %v", records, err)
	}
	seen := map[string]bool{}
	for _, record := range records {
		seen[record.SourceID] = true
	}
	if !seen[authorities.SessionExchanges[0].SourceID()] || !seen[fmt.Sprintf("channel-message-%d-through-%d", main.UserMessageID, main.MessageID)] {
		t.Fatalf("searched exchanges changed source identities: %#v", records)
	}
}

func TestConversationCandidatesRejectUnboundAssistantDespiteQueuedReply(t *testing.T) {
	pool, repository := freshCodingPlanRepository(t)
	ctx := context.Background()
	claim, question := activeSessionMessageFixture(t, repository)
	if _, err := repository.ResolveChannelSessionMessage(ctx, ResolveChannelSessionMessageCommand{Authority: claim.Authority, OperationID: question.OperationID, Kind: assemblyline.ObjectiveKindAnswer, Response: "A bound queued answer."}); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.CancelJob(ctx, CancelJobCommand{OperationID: codingPlanOperationID(t, "cancel-unbound-history", claim.Job.ID), JobID: claim.Job.ID, Reason: "End the original task.", WorkspaceRoot: question.WorkspaceRoot, WorkspaceIdentity: question.WorkspaceIdentity}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO ai_channel_messages(channel_id,role,content) VALUES($1,'assistant','This has no completed-task or queued-message authority.')`, question.ChannelID); err != nil {
		t.Fatal(err)
	}
	next := nextSessionHistoryJob(t, repository, question)
	if _, err := repository.ConversationCandidateAuthorities(ctx, next); err == nil {
		t.Fatal("arbitrary assistant text acquired authority through queued history")
	}
}

func nextSessionHistoryJob(t *testing.T, repository *Repository, prior ChannelSessionTurnCommand) model.Job {
	t.Helper()
	prior.OperationID = codingPlanOperationID(t, "next-session-history", *prior.ExpectedJobID)
	prior.Text = "Explain the next ordinary question without modifying files."
	prior.ExpectedJobID, prior.ExpectedGeneration = nil, nil
	result, err := repository.SubmitChannelSessionTurn(context.Background(), prior)
	if err != nil {
		t.Fatal(err)
	}
	return result.Job
}
