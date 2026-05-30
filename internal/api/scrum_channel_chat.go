package api

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

func appendScrumChatMessage(existing []ScrumChatMessage, role, content string) []ScrumChatMessage {
	content = strings.TrimSpace(sanitizeScrumChannelText(content))
	if content == "" {
		return existing
	}
	role = normalizeScrumChannelRole(role)
	return append(existing, ScrumChatMessage{
		Role:      role,
		Content:   content,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

func normalizeScrumChannelRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "user", "assistant", "system", "error", "tool", "thinking", "status":
		return strings.ToLower(strings.TrimSpace(role))
	default:
		return "system"
	}
}

func appendScrumChannelEvent(card ScrumCard, role, content string) ScrumCard {
	card.Chat = appendScrumChatMessage(card.Chat, role, content)
	card.ConsoleLog = appendScrumConsole(card.ConsoleLog, content)
	return card
}

func stripAssistantStreamMarker(content string) string {
	return StripAgentStreamMarker(content)
}

func updateLastAssistantChat(chat []ScrumChatMessage, delta string) []ScrumChatMessage {
	if len(chat) == 0 || strings.TrimSpace(delta) == "" {
		return chat
	}
	for i := len(chat) - 1; i >= 0; i-- {
		if chat[i].Role != "assistant" {
			continue
		}
		base := stripAssistantStreamMarker(chat[i].Content)
		chat[i].Content = strings.TrimRight(base, "\n") + delta
		chat[i].CreatedAt = time.Now().UTC().Format(time.RFC3339)
		return chat
	}
	return appendScrumChatMessage(chat, "assistant", delta)
}

func setAssistantStreamMarker(chat []ScrumChatMessage, syncedLen int) []ScrumChatMessage {
	if len(chat) == 0 || syncedLen <= 0 {
		return chat
	}
	for i := len(chat) - 1; i >= 0; i-- {
		if chat[i].Role != "assistant" {
			continue
		}
		base := stripAssistantStreamMarker(chat[i].Content)
		chat[i].Content = strings.TrimRight(base, "\n") + "\n" + agentStreamMarker(syncedLen) + "\n"
		return chat
	}
	return chat
}

func agentStreamMarker(syncedLen int) string {
	return fmt.Sprintf("[[agent-stream-len:%d]]", syncedLen)
}

func sanitizeScrumChannelText(s string) string {
	return queue.SanitizeUTF8Text(s)
}

func sanitizeScrumChannelBytes(b []byte) []byte {
	return queue.SanitizeUTF8Bytes(b)
}

func syncRunningJobChannelChat(card ScrumCard, job model.JobDetails) (ScrumCard, bool) {
	output := collectScrumAgentOutput(job)
	syncedLen := syncedAgentStreamLenFromChat(card.Chat)
	updated := card
	changed := false

	if strings.TrimSpace(output) != "" && syncedLen < len(output) {
		delta := output[syncedLen:]
		if strings.TrimSpace(delta) != "" {
			updated.Chat = appendParsedAgentStreamLines(updated.Chat, delta)
			updated.Chat = setChannelSyncMarker(updated.Chat, len(output))
			changed = true
		}
	}

	if syncedCtx, ok := syncRunningJobStepContexts(updated, job); ok {
		updated = syncedCtx
		changed = true
	}
	if !changed {
		return card, false
	}
	return updated, true
}

func syncRunningJobStepContexts(card ScrumCard, job model.JobDetails) (ScrumCard, bool) {
	if len(job.Contexts) == 0 {
		return card, false
	}
	syncedID := syncedStepContextID(card.Chat)
	updated := card
	changed := false
	maxID := syncedID
	for _, ctxValue := range job.Contexts {
		if ctxValue.ID <= syncedID {
			continue
		}
		for _, msg := range stepContextToActivity(ctxValue) {
			if shouldSkipDuplicateChannelMessage(updated.Chat, msg) {
				continue
			}
			updated.Chat = appendOrMergeChannelMessage(updated.Chat, msg)
			changed = true
		}
		if ctxValue.ID > maxID {
			maxID = ctxValue.ID
		}
	}
	if !changed {
		return card, false
	}
	updated.Chat = setStepContextSyncMarker(updated.Chat, maxID)
	return updated, true
}

func hydrateCardChannelChat(card ScrumCard) ScrumCard {
	if len(card.Chat) > 0 {
		return card
	}
	displayLog := strings.TrimSpace(StripAgentStreamMarker(card.ConsoleLog))
	if displayLog == "" {
		return card
	}
	updated := card
	for _, block := range splitConsoleLogBlocks(displayLog) {
		role := "system"
		if strings.HasPrefix(strings.ToLower(block), "agent stream:") || strings.HasPrefix(strings.ToLower(block), "agent output:") {
			role = "assistant"
		}
		updated.Chat = appendScrumChatMessage(updated.Chat, role, block)
	}
	return updated
}

func splitConsoleLogBlocks(displayLog string) []string {
	lines := strings.Split(displayLog, "\n")
	blocks := make([]string, 0)
	current := make([]string, 0)
	flush := func() {
		if len(current) == 0 {
			return
		}
		block := strings.TrimSpace(strings.Join(current, "\n"))
		if block != "" {
			blocks = append(blocks, block)
		}
		current = current[:0]
	}
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return blocks
}

func scrumCardChannelChanged(before, after ScrumCard) bool {
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

func displayScrumChannelMessages(card ScrumCard) []ScrumChatMessage {
	card = hydrateCardChannelChat(card)
	out := make([]ScrumChatMessage, 0, len(card.Chat))
	for _, msg := range sortScrumChatChronological(card.Chat) {
		content := strings.TrimSpace(stripAssistantStreamMarker(msg.Content))
		if content == "" || strings.HasPrefix(content, "[[agent-stream-len:") || strings.HasPrefix(content, "[[context-sync:") {
			continue
		}
		role := normalizeScrumChannelRole(msg.Role)
		switch role {
		case "status":
			continue
		case "tool":
			if _, ok := parseChannelActivity(content); !ok && isScrumChannelNoiseContent(role, content) {
				continue
			}
		case "assistant":
			if isAgentToolLikeAssistant(content) || isScrumChannelNoiseContent(role, content) {
				continue
			}
		case "system":
			if isScrumChannelNoiseContent(role, content) {
				continue
			}
		}
		out = append(out, ScrumChatMessage{
			Role:      role,
			Content:   content,
			CreatedAt: msg.CreatedAt,
		})
	}
	return collapseScrumChannelDisplayMessages(out)
}

func isScrumChannelNoiseContent(role, content string) bool {
	lower := strings.ToLower(strings.TrimSpace(content))
	switch lower {
	case "external agent session completed", "agent finished", "agent running…", "agent running...", "agent running":
		return true
	}
	if strings.HasPrefix(lower, "job status:") {
		return true
	}
	if role == "system" {
		if strings.HasPrefix(lower, "execution agent:") ||
			strings.HasPrefix(lower, "agent config source:") ||
			strings.HasPrefix(lower, "models:") ||
			strings.HasPrefix(lower, "channel steer sent") ||
			strings.HasPrefix(lower, "channel message sent") {
			return true
		}
	}
	return false
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
		switch role {
		case "assistant":
			last.Content = mergeAssistantStreamContent(last.Content, msg.Content)
		case "thinking":
			last.Content = mergePilotThoughtText(last.Content, msg.Content)
		case "tool":
			if lastActivity, ok := parseChannelActivity(last.Content); ok {
				if nextActivity, ok := parseChannelActivity(msg.Content); ok && sameChannelActivity(lastActivity, nextActivity) {
					continue
				}
			}
			out = append(out, msg)
		default:
			out = append(out, msg)
			continue
		}
		if strings.TrimSpace(msg.CreatedAt) != "" {
			last.CreatedAt = msg.CreatedAt
		}
		out[lastIdx] = last
	}
	return out
}
