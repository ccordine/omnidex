package api

import (
	"fmt"
	"strings"
)

const (
	scrumPilotChatMaxTotalChars = 4500
	scrumPilotCardDescMaxChars  = 360
	scrumPilotChecklistMaxItems = 10
	scrumPilotChecklistItemMax  = 72
	scrumPilotRefFilesMax       = 6
)

func buildScrumPilotChatPrompt(board ScrumBoard, card ScrumCard, userMessage string, ctx scrumPilotPromptContext) string {
	lines := []string{
		"Scrum card: " + strings.TrimSpace(card.Title),
		"Column: " + normalizeScrumColumn(card.Column),
		"Project directory: " + strings.TrimSpace(board.ProjectDirectory),
	}
	if desc := cavemanPilotText(card.Description, scrumPilotCardDescMaxChars); desc != "" {
		lines = append(lines, "Description: "+desc)
	}
	if len(card.RefFiles) > 0 {
		refs := card.RefFiles
		if len(refs) > scrumPilotRefFilesMax {
			refs = refs[:scrumPilotRefFilesMax]
		}
		lines = append(lines, "Reference files: "+strings.Join(refs, ", "))
	}
	checklistCount := 0
	for _, item := range card.Checklist {
		if checklistCount >= scrumPilotChecklistMaxItems {
			break
		}
		state := "[ ]"
		if item.Done {
			state = "[x]"
		}
		text := cavemanPilotText(item.Text, scrumPilotChecklistItemMax)
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s %s", state, text))
		checklistCount++
	}
	if len(ctx.MemoryLines) > 0 {
		lines = append(lines, "Memory:")
		for _, line := range ctx.MemoryLines {
			lines = append(lines, "- "+line)
		}
	}
	if summary := strings.TrimSpace(ctx.ChannelSummary); summary != "" {
		lines = append(lines, summary)
	}
	if len(ctx.ChannelFacts) > 0 {
		lines = append(lines, ctx.ChannelFacts...)
	}
	lines = append(lines, "user: "+strings.TrimSpace(userMessage))
	return trimScrumPilotPrompt(strings.Join(lines, "\n"), scrumPilotChatMaxTotalChars)
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
