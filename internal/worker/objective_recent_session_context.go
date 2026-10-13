package worker

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type recentAssistantContextProjection struct {
	Records             []queue.ContextSearchRecord
	UserMessageIDs      []int64
	SessionOperationIDs []model.LifecycleOperationID
}

type recentAssistantContextRecord struct {
	replyID     int64
	userID      int64
	operationID model.LifecycleOperationID
	record      queue.ContextSearchRecord
}

// Original task results retain their real user-message authority. Queued
// questions retain their operation authority; only reply IDs order recency.
func projectRecentAssistantContext(set queue.ConversationCandidateAuthoritySet) (recentAssistantContextProjection, error) {
	projection := recentAssistantContextProjection{
		Records: []queue.ContextSearchRecord{}, UserMessageIDs: []int64{},
		SessionOperationIDs: []model.LifecycleOperationID{},
	}
	original, err := projectOriginalAssistantContext(set)
	if err != nil {
		return projection, err
	}
	candidates := append([]recentAssistantContextRecord(nil), original...)
	seenOperations := make(map[model.LifecycleOperationID]struct{}, len(set.SessionExchanges))
	for _, exchange := range set.SessionExchanges {
		if err := validateRecentSessionExchange(exchange); err != nil {
			return projection, err
		}
		if _, duplicate := seenOperations[exchange.OperationID]; duplicate {
			return projection, fmt.Errorf("recent queued exchange has duplicate operation authority")
		}
		seenOperations[exchange.OperationID] = struct{}{}
		candidates = append(candidates, recentAssistantContextRecord{
			replyID: exchange.ResponseMessageID, operationID: exchange.OperationID,
			record: exchange.ContextRecord(),
		})
	}
	seenReplies := make(map[int64]struct{}, len(candidates))
	for _, candidate := range candidates {
		if _, duplicate := seenReplies[candidate.replyID]; duplicate {
			return projection, fmt.Errorf("recent assistant reply has competing source authorities")
		}
		seenReplies[candidate.replyID] = struct{}{}
	}
	slices.SortFunc(candidates, func(first, second recentAssistantContextRecord) int {
		return cmp.Compare(second.replyID, first.replyID)
	})
	for _, candidate := range candidates[:min(len(candidates), contextRecentRecordLimit)] {
		projection.Records = append(projection.Records, candidate.record)
		if candidate.userID > 0 {
			projection.UserMessageIDs = append(projection.UserMessageIDs, candidate.userID)
		} else {
			projection.SessionOperationIDs = append(projection.SessionOperationIDs, candidate.operationID)
		}
	}
	return projection, nil
}

func projectOriginalAssistantContext(set queue.ConversationCandidateAuthoritySet) ([]recentAssistantContextRecord, error) {
	records, err := recentConversationContextRecords(set.Turns)
	if err != nil {
		return nil, err
	}
	if len(records) != len(set.AssistantResults) {
		return nil, fmt.Errorf("recent assistant context differs from exact task result authority")
	}
	results := make(map[int64]queue.ConversationAssistantResultAuthority, len(set.AssistantResults))
	for _, result := range set.AssistantResults {
		if result.UserMessageID < 1 || result.MessageID <= result.UserMessageID || result.JobID < 1 || result.SpeakerName != "" {
			return nil, fmt.Errorf("recent original assistant result has invalid authority")
		}
		if _, duplicate := results[result.UserMessageID]; duplicate {
			return nil, fmt.Errorf("recent original task has competing completed results")
		}
		results[result.UserMessageID] = result
	}
	bySource := make(map[string]queue.ConversationAssistantResultAuthority, len(records))
	for _, turn := range set.Turns {
		if turn.RoleplayUserTurn != nil || turn.SpeakerName != "" {
			return nil, fmt.Errorf("assistant context cannot carry fictional speaker authority")
		}
		if turn.Role != queue.ConversationCandidateAssistant {
			continue
		}
		result, exists := results[turn.PairedUserMessageID]
		if !exists || result.MessageID != turn.MessageID || result.Content != turn.Content {
			return nil, fmt.Errorf("recent assistant turn differs from its exact completed task result")
		}
		bySource[fmt.Sprintf("channel-message-%d-through-%d", result.UserMessageID, result.MessageID)] = result
	}
	projected := make([]recentAssistantContextRecord, 0, len(records))
	for _, record := range records {
		result, exists := bySource[record.SourceID]
		if !exists {
			return nil, fmt.Errorf("recent task context lacks an exact completed result source")
		}
		projected = append(projected, recentAssistantContextRecord{
			replyID: result.MessageID, userID: result.UserMessageID, record: record,
		})
	}
	return projected, nil
}

func validateRecentSessionExchange(exchange queue.ConversationSessionExchangeAuthority) error {
	if _, err := model.ParseLifecycleOperationID(string(exchange.OperationID)); err != nil {
		return err
	}
	if exchange.JobID < 1 || exchange.Generation < 1 || exchange.ResponseMessageID < 1 {
		return fmt.Errorf("recent queued exchange lacks exact job, generation or reply authority")
	}
	switch exchange.ResolutionKind {
	case model.ChannelSessionMessageAnswered, model.ChannelSessionMessageStory, model.ChannelSessionMessageFailed:
	default:
		return fmt.Errorf("recent queued exchange has no terminal reply authority")
	}
	if err := model.ValidateChannelMessage(model.ChannelMessageRoleUser, exchange.QuestionText); err != nil {
		return err
	}
	return model.ValidateChannelMessage(model.ChannelMessageRoleAssistant, exchange.ResponseText)
}
