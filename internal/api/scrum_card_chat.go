package api

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const (
	scrumPilotChatMaxTotalChars  = 12000
	scrumPilotChatMaxMessageChars = 900
	scrumPilotChatMaxMessages    = 16
	scrumPilotChatLLMTimeout     = 4 * time.Minute
)

func scrumCardChatLLMContext(parent context.Context) (context.Context, context.CancelFunc) {
	timeout := scrumPilotChatLLMTimeout
	return context.WithTimeout(context.WithoutCancel(parent), timeout)
}

func buildScrumPilotChatPrompt(board ScrumBoard, card ScrumCard, userMessage string, ctx scrumPilotPromptContext) string {
	lines := []string{
		"Scrum card: " + strings.TrimSpace(card.Title),
		"Column: " + normalizeScrumColumn(card.Column),
		"Project directory: " + strings.TrimSpace(board.ProjectDirectory),
	}
	if desc := trimScrumPilotText(card.Description, 1800); desc != "" {
		lines = append(lines, "Description: "+desc)
	}
	if len(card.RefFiles) > 0 {
		refs := card.RefFiles
		if len(refs) > 12 {
			refs = refs[:12]
		}
		lines = append(lines, "Reference files: "+strings.Join(refs, ", "))
	}
	for _, item := range card.Checklist {
		state := "[ ]"
		if item.Done {
			state = "[x]"
		}
		lines = append(lines, fmt.Sprintf("%s %s", state, strings.TrimSpace(item.Text)))
	}
	if len(ctx.MemoryLines) > 0 {
		lines = append(lines, "Relevant memory:")
		for _, line := range ctx.MemoryLines {
			lines = append(lines, "- "+line)
		}
	}
	if summary := strings.TrimSpace(ctx.ChannelSummary); summary != "" {
		lines = append(lines, summary)
	} else if summary := summarizeScrumChannelForPilot(card.Chat); summary != "" {
		lines = append(lines, summary)
	}
	if len(ctx.RecentTurns) > 0 {
		lines = append(lines, ctx.RecentTurns...)
	} else {
		for _, line := range selectScrumPilotChatHistory(card.Chat) {
			lines = append(lines, line)
		}
	}
	lines = append(lines, "user: "+strings.TrimSpace(userMessage))
	return trimScrumPilotPrompt(strings.Join(lines, "\n"), scrumPilotChatMaxTotalChars)
}

func selectScrumPilotChatHistory(chat []ScrumChatMessage) []string {
	if len(chat) == 0 {
		return nil
	}
	selected := make([]string, 0, scrumPilotChatMaxMessages)
	total := 0
	for i := len(chat) - 1; i >= 0 && len(selected) < scrumPilotChatMaxMessages; i-- {
		msg := chat[i]
		content := strings.TrimSpace(msg.Content)
		if content == "" || strings.Contains(content, "[[agent-stream-len:") {
			continue
		}
		role := normalizeScrumChannelRole(msg.Role)
		switch role {
		case "tool", "thinking":
			content = trimScrumPilotText(content, 220)
			if content == "" {
				continue
			}
			content = "[" + role + "] " + content
		case "status", "system", "error":
			if len(content) > 240 {
				content = trimScrumPilotText(content, 240)
			}
		default:
			content = trimScrumPilotText(content, scrumPilotChatMaxMessageChars)
		}
		if content == "" {
			continue
		}
		line := role + ": " + content
		if total+len(line) > scrumPilotChatMaxTotalChars {
			break
		}
		selected = append(selected, line)
		total += len(line)
	}
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}
	if len(selected) == 0 {
		return nil
	}
	return append([]string{"Recent channel (trimmed for context):"}, selected...)
}

func summarizeScrumChannelForPilot(chat []ScrumChatMessage) string {
	if len(chat) == 0 {
		return ""
	}
	roles := map[string]int{}
	var lastStatus string
	for _, msg := range chat {
		role := normalizeScrumChannelRole(msg.Role)
		if strings.TrimSpace(msg.Content) != "" {
			roles[role]++
		}
		if role == "status" || role == "system" || role == "error" {
			lastStatus = trimScrumPilotText(msg.Content, 240)
		}
	}
	parts := []string{fmt.Sprintf("Channel transcript: %d messages", len(chat))}
	if roles["tool"] > 0 {
		parts = append(parts, fmt.Sprintf("%d tool events", roles["tool"]))
	}
	if roles["thinking"] > 0 {
		parts = append(parts, fmt.Sprintf("%d thinking notes", roles["thinking"]))
	}
	if lastStatus != "" {
		parts = append(parts, "Latest status: "+lastStatus)
	}
	return strings.Join(parts, " · ")
}

func trimScrumPilotText(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "…"
}

func trimScrumPilotPrompt(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	head := max * 2 / 3
	tail := max - head - 20
	if tail < 0 {
		tail = 0
	}
	if head < 0 {
		head = max
	}
	if tail == 0 {
		return trimScrumPilotText(text, max)
	}
	return strings.TrimSpace(text[:head]) + "\n…context trimmed…\n" + strings.TrimSpace(text[len(text)-tail:])
}

func formatScrumPilotChatError(err error) string {
	if err == nil {
		return "Pilot chat failed."
	}
	msg := strings.TrimSpace(err.Error())
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "context length") || strings.Contains(lower, "context exceeded") || strings.Contains(lower, "too many tokens"):
		return "Pilot chat failed: context limit exceeded. Channel history was trimmed, but the model still ran out of room — try a shorter message or clear old agent output from the channel."
	case strings.Contains(lower, "context deadline exceeded") || strings.Contains(lower, "timeout"):
		return "Pilot chat timed out. Your message was saved — try again with a shorter question or switch to a faster model."
	default:
		return "Pilot chat failed: " + trimScrumPilotText(msg, 400)
	}
}
