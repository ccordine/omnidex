package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

type recentConversationExchange struct {
	userID      int64
	assistantID int64
	roleplay    bool
	content     string
}

func recentConversationContextRecords(
	turns []queue.ConversationCandidateTurn,
) ([]queue.ContextSearchRecord, error) {
	exchanges := make([]recentConversationExchange, 0, len(turns)/2+1)
	for _, turn := range turns {
		switch turn.Role {
		case queue.ConversationCandidateUser:
			exchange, err := newRecentConversationExchange(turn)
			if err != nil {
				return nil, err
			}
			exchanges = append(exchanges, exchange)
		case queue.ConversationCandidateAssistant:
			if err := appendRecentAssistantTurn(exchanges, turn); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("recent conversation message %d has invalid role %q", turn.MessageID, turn.Role)
		}
	}
	return projectRecentConversationExchanges(exchanges), nil
}

func newRecentConversationExchange(
	turn queue.ConversationCandidateTurn,
) (recentConversationExchange, error) {
	exchange := recentConversationExchange{userID: turn.MessageID}
	if turn.RoleplayUserTurn == nil {
		if turn.SpeakerName != "" {
			return exchange, fmt.Errorf("recent user message %d has speaker without roleplay authority", turn.MessageID)
		}
		exchange.content = "user message:\n" + turn.Content
		return exchange, nil
	}
	if err := turn.RoleplayUserTurn.Validate(); err != nil {
		return exchange, fmt.Errorf("recent roleplay user message %d: %w", turn.MessageID, err)
	}
	if turn.RoleplayUserTurn.ExactText != turn.Content ||
		turn.RoleplayUserTurn.PersonaName != turn.SpeakerName {
		return exchange, fmt.Errorf("recent roleplay user message %d differs from typed authority", turn.MessageID)
	}
	exchange.roleplay = true
	exchange.content = turn.SpeakerName + " [" + string(turn.RoleplayUserTurn.ContributionKind) +
		"] contribution:\n" + turn.Content
	return exchange, nil
}

func appendRecentAssistantTurn(
	exchanges []recentConversationExchange,
	turn queue.ConversationCandidateTurn,
) error {
	if len(exchanges) == 0 || exchanges[len(exchanges)-1].userID != turn.PairedUserMessageID ||
		exchanges[len(exchanges)-1].assistantID != 0 {
		return fmt.Errorf("recent assistant message %d has no exact adjacent user exchange", turn.MessageID)
	}
	exchange := &exchanges[len(exchanges)-1]
	exchange.assistantID = turn.MessageID
	if turn.RoleplayUserTurn != nil {
		return fmt.Errorf("recent assistant message %d carries user-turn authority", turn.MessageID)
	}
	if exchange.roleplay {
		if strings.TrimSpace(turn.SpeakerName) == "" {
			return fmt.Errorf("recent roleplay assistant message %d has no speaker", turn.MessageID)
		}
		exchange.content += "\n" + turn.SpeakerName + " response:\n"
	} else {
		if turn.SpeakerName != "" {
			return fmt.Errorf("recent assistant message %d has unexpected speaker", turn.MessageID)
		}
		exchange.content += "\nassistant response:\n"
	}
	exchange.content += turn.Content
	return nil
}

func projectRecentConversationExchanges(
	exchanges []recentConversationExchange,
) []queue.ContextSearchRecord {
	records := make([]queue.ContextSearchRecord, 0, min(len(exchanges), contextRecentRecordLimit))
	for index := len(exchanges) - 1; index >= 0 && len(records) < contextRecentRecordLimit; index-- {
		exchange := exchanges[index]
		sourceID := fmt.Sprintf("channel-message-%d", exchange.userID)
		if exchange.assistantID != 0 {
			sourceID += fmt.Sprintf("-through-%d", exchange.assistantID)
		}
		records = append(records, queue.ContextSearchRecord{
			Namespace: "conversation_exchange",
			SourceID:  sourceID,
			Content:   exchange.content,
		})
	}
	return records
}

func completedConversationCandidateTurns(
	turns []queue.ConversationCandidateTurn,
) ([]queue.ConversationCandidateTurn, error) {
	completed := make([]queue.ConversationCandidateTurn, 0, len(turns))
	for index := 0; index < len(turns); index++ {
		turn := turns[index]
		if turn.Role != queue.ConversationCandidateUser {
			return nil, fmt.Errorf("recent assistant message %d has no exact adjacent user exchange", turn.MessageID)
		}
		if index+1 >= len(turns) || turns[index+1].Role != queue.ConversationCandidateAssistant {
			continue
		}
		assistant := turns[index+1]
		if assistant.PairedUserMessageID != turn.MessageID {
			return nil, fmt.Errorf("recent assistant message %d differs from adjacent user authority", assistant.MessageID)
		}
		completed = append(completed, turn, assistant)
		index++
	}
	return completed, nil
}
