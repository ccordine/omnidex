package api

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

func scrumCardChannelChanged(before, after ScrumCard) bool {
	if before.SyncJobID != after.SyncJobID ||
		before.AgentStreamChatCursor != after.AgentStreamChatCursor ||
		before.AgentStreamConsoleCursor != after.AgentStreamConsoleCursor ||
		before.StepContextCursor != after.StepContextCursor {
		return true
	}
	if before.ConsoleLog != after.ConsoleLog {
		return true
	}
	if len(before.Chat) != len(after.Chat) {
		return true
	}
	if len(before.Chat) == 0 {
		return false
	}
	lastBefore := before.Chat[len(before.Chat)-1]
	lastAfter := after.Chat[len(after.Chat)-1]
	return lastBefore.Content != lastAfter.Content || lastBefore.Role != lastAfter.Role
}

func scrumChatMessageTime(msg ScrumChatMessage) time.Time {
	raw := strings.TrimSpace(msg.CreatedAt)
	if raw == "" {
		return time.Time{}
	}
	if at, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return at.UTC()
	}
	if at, err := time.Parse(time.RFC3339, raw); err == nil {
		return at.UTC()
	}
	return time.Time{}
}

// sortScrumChatChronological orders channel rows by when they happened, not loop/sync order.
func sortScrumChatChronological(chat []ScrumChatMessage) []ScrumChatMessage {
	if len(chat) <= 1 {
		return chat
	}
	type indexed struct {
		msg ScrumChatMessage
		idx int
		at  time.Time
	}
	items := make([]indexed, len(chat))
	for i, msg := range chat {
		items[i] = indexed{msg: msg, idx: i, at: scrumChatMessageTime(msg)}
	}
	sort.SliceStable(items, func(i, j int) bool {
		left, right := items[i], items[j]
		if !left.at.Equal(right.at) {
			if left.at.IsZero() {
				return false
			}
			if right.at.IsZero() {
				return true
			}
			return left.at.Before(right.at)
		}
		return left.idx < right.idx
	})
	out := make([]ScrumChatMessage, len(items))
	for i, item := range items {
		out[i] = item.msg
	}
	return out
}

func displayScrumChannelMessages(card ScrumCard) ([]ScrumChatMessage, error) {
	card = hydrateCardChannelChat(card)
	out := make([]ScrumChatMessage, 0, len(card.Chat))
	seenIDs := map[string]int{}
	for _, msg := range sortScrumChatChronological(card.Chat) {
		content := msg.Content
		if strings.TrimSpace(content) == "" {
			continue
		}
		role := normalizeScrumChannelRole(msg.Role)
		if role == "" {
			return nil, fmt.Errorf("Scrum channel message %q has unsupported role %q", scrumChatMessageID(msg), msg.Role)
		}
		id := scrumChatMessageID(msg)
		if seen := seenIDs[id]; seen > 0 {
			seenIDs[id] = seen + 1
			id = fmt.Sprintf("%s_%d", id, seen+1)
		} else {
			seenIDs[id] = 1
		}
		out = append(out, ScrumChatMessage{
			ID:          id,
			Role:        role,
			Content:     content,
			CreatedAt:   msg.CreatedAt,
			Status:      msg.Status,
			OperationID: msg.OperationID,
		})
	}
	return collapseScrumChannelDisplayMessages(out), nil
}

func collapseScrumChannelDisplayMessages(messages []ScrumChatMessage) []ScrumChatMessage {
	if len(messages) == 0 {
		return messages
	}
	out := make([]ScrumChatMessage, 0, len(messages))
	for _, msg := range messages {
		if len(out) == 0 {
			out = append(out, msg)
			continue
		}
		lastIdx := len(out) - 1
		last := out[lastIdx]
		role := normalizeScrumChannelRole(msg.Role)
		lastRole := normalizeScrumChannelRole(last.Role)
		if role != lastRole {
			out = append(out, msg)
			continue
		}
		if role == "tool" {
			if lastActivity, ok := parseChannelActivity(last.Content); ok {
				if nextActivity, ok := parseChannelActivity(msg.Content); ok && sameChannelActivity(lastActivity, nextActivity) {
					last.Content = formatChannelActivity(mergeChannelActivity(lastActivity, nextActivity))
					if strings.TrimSpace(msg.CreatedAt) != "" {
						last.CreatedAt = msg.CreatedAt
					}
					out[lastIdx] = last
					continue
				}
			}
		}
		out = append(out, msg)
	}
	return out
}
