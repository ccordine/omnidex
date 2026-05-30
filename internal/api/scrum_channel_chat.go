package api

import (
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
)

func appendScrumChatMessage(existing []ScrumChatMessage, role, content string) []ScrumChatMessage {
	content = strings.TrimSpace(content)
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

func syncRunningJobChannelChat(card ScrumCard, job model.JobDetails) (ScrumCard, bool) {
	output := collectScrumAgentOutput(job)
	if strings.TrimSpace(output) == "" {
		return card, false
	}
	syncedLen := syncedAgentStreamLenFromChat(card.Chat)
	if syncedLen >= len(output) {
		return card, false
	}
	delta := output[syncedLen:]
	if strings.TrimSpace(delta) == "" {
		return card, false
	}

	updated := card
	beforeLen := len(updated.Chat)
	updated.Chat = appendParsedAgentStreamLines(updated.Chat, delta)
	if len(updated.Chat) == beforeLen {
		return card, false
	}
	updated.Chat = setChannelSyncMarker(updated.Chat, len(output))
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

func displayScrumChannelMessages(card ScrumCard) []ScrumChatMessage {
	card = hydrateCardChannelChat(card)
	out := make([]ScrumChatMessage, 0, len(card.Chat))
	for _, msg := range card.Chat {
		content := strings.TrimSpace(stripAssistantStreamMarker(msg.Content))
		if content == "" || strings.HasPrefix(content, "[[agent-stream-len:") {
			continue
		}
		out = append(out, ScrumChatMessage{
			Role:      normalizeScrumChannelRole(msg.Role),
			Content:   content,
			CreatedAt: msg.CreatedAt,
		})
	}
	return out
}
