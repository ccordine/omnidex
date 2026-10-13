package worker

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func TestRecentAssistantContextRetainsTypedQueuedQuestionPair(t *testing.T) {
	exchange := sessionContextExchange(4)
	projection, err := projectRecentAssistantContext(queue.ConversationCandidateAuthoritySet{
		SessionExchanges: []queue.ConversationSessionExchangeAuthority{exchange},
	})
	if err != nil || len(projection.Records) != 1 {
		t.Fatalf("typed queued exchange is unavailable: records=%d err=%v", len(projection.Records), err)
	}
	record := projection.Records[0]
	if record.Namespace != "conversation_exchange" || record.SourceID != exchange.SourceID() ||
		record.Content != "user message:\n"+exchange.QuestionText+"\nassistant response:\n"+exchange.ResponseText {
		t.Fatalf("queued question/reply lost exact source or text: %+v", record)
	}
	if len(projection.UserMessageIDs) != 0 || !reflect.DeepEqual(projection.SessionOperationIDs, []model.LifecycleOperationID{exchange.OperationID}) {
		t.Fatalf("queued exchange invented a user message identity: users=%v operations=%v", projection.UserMessageIDs, projection.SessionOperationIDs)
	}
}

func TestRecentAssistantContextMergesByReplyIdentityWithinBound(t *testing.T) {
	set := originalContextExchange(1, 10)
	for replyID := int64(2); replyID <= 8; replyID++ {
		set.SessionExchanges = append(set.SessionExchanges, sessionContextExchange(replyID))
	}
	projection, err := projectRecentAssistantContext(set)
	if err != nil || len(projection.Records) != contextRecentRecordLimit {
		t.Fatalf("merged suffix violates original bound: records=%d err=%v", len(projection.Records), err)
	}
	if projection.Records[0].SourceID != "channel-message-1-through-10" ||
		projection.Records[1].SourceID != sessionContextExchange(8).SourceID() ||
		projection.Records[len(projection.Records)-1].SourceID != sessionContextExchange(4).SourceID() {
		t.Fatalf("merged suffix does not follow actual reply chronology: %+v", projection.Records)
	}
	if len(projection.SessionOperationIDs) != contextRecentRecordLimit-1 {
		t.Fatalf("search representation includes omitted exchanges: %v", projection.SessionOperationIDs)
	}
}

func TestRecentAssistantContextRejectsMismatchedOrDuplicateAuthority(t *testing.T) {
	original := originalContextExchange(1, 4)
	original.AssistantResults[0].Content = "A different task result."
	if _, err := projectRecentAssistantContext(original); err == nil {
		t.Fatal("mismatched original task result was accepted")
	}
	for name, exchanges := range map[string][]queue.ConversationSessionExchangeAuthority{
		"duplicate operation": {sessionContextExchange(4), sessionContextExchange(4)},
		"pending resolution": {func() queue.ConversationSessionExchangeAuthority {
			e := sessionContextExchange(4)
			e.ResolutionKind = model.ChannelSessionMessagePending
			return e
		}()},
		"missing job": {func() queue.ConversationSessionExchangeAuthority {
			e := sessionContextExchange(4)
			e.JobID = 0
			return e
		}()},
		"invalid operation": {func() queue.ConversationSessionExchangeAuthority {
			e := sessionContextExchange(4)
			e.OperationID = "invalid"
			return e
		}()},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := projectRecentAssistantContext(queue.ConversationCandidateAuthoritySet{SessionExchanges: exchanges}); err == nil {
				t.Fatal("invalid queued exchange entered semantic context")
			}
		})
	}
	set := originalContextExchange(1, 4)
	set.SessionExchanges = []queue.ConversationSessionExchangeAuthority{sessionContextExchange(4)}
	if _, err := projectRecentAssistantContext(set); err == nil {
		t.Fatal("one reply gained two competing authorities")
	}
}

func TestRecentAssistantSearchRepresentationSeparatesUserAndOperationIdentities(t *testing.T) {
	set := originalContextExchange(1, 2)
	set.SessionExchanges = []queue.ConversationSessionExchangeAuthority{sessionContextExchange(4)}
	projection, err := projectRecentAssistantContext(set)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(projection.UserMessageIDs, []int64{1}) ||
		!reflect.DeepEqual(projection.SessionOperationIDs, []model.LifecycleOperationID{set.SessionExchanges[0].OperationID}) {
		t.Fatalf("search representation confuses independent authorities: users=%v operations=%v", projection.UserMessageIDs, projection.SessionOperationIDs)
	}
	failed := sessionContextExchange(6)
	failed.ResolutionKind, failed.ResponseText = model.ChannelSessionMessageFailed, "Message could not be answered: provider unavailable."
	set.SessionExchanges = append(set.SessionExchanges, failed)
	projection, err = projectRecentAssistantContext(set)
	if err != nil || !strings.Contains(projection.Records[0].Content, "assistant failure:\n") {
		t.Fatalf("failed reply was projected as successful task authority: err=%v", err)
	}
}

func sessionContextExchange(replyID int64) queue.ConversationSessionExchangeAuthority {
	return queue.ConversationSessionExchangeAuthority{
		OperationID: model.LifecycleOperationID(fmt.Sprintf("lifecycle_operation_context_%d", replyID)),
		JobID:       17, Generation: 1, QuestionText: "What is the difference between /interrupt and /stop?",
		ResponseMessageID: replyID, ResponseText: "The first pauses; the second cancels.", ResolutionKind: model.ChannelSessionMessageAnswered,
	}
}

func originalContextExchange(userID, replyID int64) queue.ConversationCandidateAuthoritySet {
	const answer = "The original task result."
	return queue.ConversationCandidateAuthoritySet{
		Turns: []queue.ConversationCandidateTurn{
			{MessageID: userID, Role: queue.ConversationCandidateUser, Content: "The original task request."},
			{MessageID: replyID, Role: queue.ConversationCandidateAssistant, PairedUserMessageID: userID, Content: answer},
		},
		AssistantResults: []queue.ConversationAssistantResultAuthority{{UserMessageID: userID, MessageID: replyID, JobID: 11, Content: answer}},
	}
}
