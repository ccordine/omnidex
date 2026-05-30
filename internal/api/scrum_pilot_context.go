package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
)

const (
	scrumPilotSummarizeMinMessages   = 6
	scrumPilotSummarizeMinChars      = 4000
	scrumPilotTranscriptMaxChars     = 24000
	scrumPilotRecentTurnsMax         = 6
	scrumPilotRecentTurnMaxChars     = 700
	scrumPilotSummarizeTimeout       = scrumPilotChatLLMTimeout / 2
)

type scrumPilotPromptContext struct {
	ChannelSummary string
	RecentTurns    []string
	MemoryLines    []string
}

func (s *Server) scrumPilotMemoryContext(ctx context.Context, card ScrumCard, projectID int64, query string) []string {
	if s.repo == nil || s.llmClient == nil {
		return nil
	}
	tags := scrumPilotMemoryTags(card, projectID)
	var embedding []float64
	if value, err := s.llmClient.Embedding(ctx, query); err == nil {
		embedding = value
	}
	matches, err := s.repo.FindRelevantMemory(ctx, embedding, tags, 8)
	if err != nil {
		return nil
	}
	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		content := strings.TrimSpace(match.Content)
		if content == "" {
			continue
		}
		lines = append(lines, trimScrumPilotText(content, 900))
	}
	return lines
}

func scrumPilotMemoryTags(card ScrumCard, projectID int64) []string {
	tags := append([]string{}, card.Tags...)
	tags = append(tags, "scrum", "card-channel", card.ID)
	if projectID > 0 {
		tags = append(tags, fmt.Sprintf("project:%d", projectID))
	}
	return mergeTags(nil, tags)
}

func (s *Server) summarizeScrumPilotChannel(ctx context.Context, board ScrumBoard, card ScrumCard, userMessage string, memoryLines []string) scrumPilotPromptContext {
	out := scrumPilotPromptContext{MemoryLines: memoryLines}
	if len(card.Chat) == 0 {
		return out
	}
	recent := selectScrumPilotRecentTurns(card.Chat)
	out.RecentTurns = recent

	transcript := scrumChannelTranscriptForSummarizer(card.Chat)
	if !shouldSummarizeScrumPilotChannel(card.Chat, transcript) {
		out.ChannelSummary = summarizeScrumChannelForPilot(card.Chat)
		if out.ChannelSummary == "" && len(recent) == 0 {
			out.RecentTurns = selectScrumPilotChatHistory(card.Chat)
		}
		return out
	}
	if s.llmClient == nil {
		out.ChannelSummary = summarizeScrumChannelForPilot(card.Chat)
		if len(out.RecentTurns) == 0 {
			out.RecentTurns = selectScrumPilotChatHistory(card.Chat)
		}
		return out
	}

	sumCtx, cancel := context.WithTimeout(ctx, scrumPilotSummarizeTimeout)
	defer cancel()
	prompt := buildScrumPilotSummarizerPrompt(board, card, userMessage, transcript, memoryLines)
	raw, err := s.scrumLLMGenerate(sumCtx, scrumPilotSummarizerSystem(), prompt)
	if err != nil {
		out.ChannelSummary = summarizeScrumChannelForPilot(card.Chat)
		if len(out.RecentTurns) == 0 {
			out.RecentTurns = selectScrumPilotChatHistory(card.Chat)
		}
		return out
	}
	minimal, err := omni.ParseMinimalContext(extractJSONBlob(raw))
	if err != nil {
		out.ChannelSummary = summarizeScrumChannelForPilot(card.Chat)
		if len(out.RecentTurns) == 0 {
			out.RecentTurns = selectScrumPilotChatHistory(card.Chat)
		}
		return out
	}
	out.ChannelSummary = formatScrumMinimalContextForPilot(minimal)
	return out
}

func shouldSummarizeScrumPilotChannel(chat []ScrumChatMessage, transcript string) bool {
	if len(chat) >= scrumPilotSummarizeMinMessages {
		return true
	}
	return len(transcript) >= scrumPilotSummarizeMinChars
}

func scrumPilotSummarizerSystem() string {
	return strings.Join([]string{
		"You are the channel context minifier for Omni scrum card pilot chat.",
		"Compress agent channel transcripts into terse caveman-style minimal context.",
		"Return JSON only with keys summary, facts, constraints, open_items.",
		"Do not answer the user. Do not suggest shell commands.",
	}, " ")
}

func buildScrumPilotSummarizerPrompt(board ScrumBoard, card ScrumCard, userMessage, transcript string, memoryLines []string) string {
	payload := map[string]any{
		"role":               "channel_summary_specialist",
		"card_title":         strings.TrimSpace(card.Title),
		"card_column":        normalizeScrumColumn(card.Column),
		"project_directory":  strings.TrimSpace(board.ProjectDirectory),
		"user_message":       strings.TrimSpace(userMessage),
		"channel_transcript": transcript,
		"instructions": []string{
			"Load the smallest context inventory needed to steer this card.",
			"Strip verbose tool dumps, thinking fluff, and duplicate status noise.",
			"Keep paths, decisions, failures, blockers, and evidence that matter for the user's latest message.",
			"summary: 1-2 short sentences on current work state.",
			"facts: terse bullet facts (what happened, files touched, outcomes).",
			"constraints: limits the pilot must respect.",
			"open_items: unanswered questions or next steps still pending.",
			"Return empty arrays when a section has nothing useful.",
		},
	}
	if len(memoryLines) > 0 {
		payload["relevant_memory"] = memoryLines
	}
	if desc := trimScrumPilotText(card.Description, 1200); desc != "" {
		payload["card_description"] = desc
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return transcript
	}
	return string(blob)
}

func scrumChannelTranscriptForSummarizer(chat []ScrumChatMessage) string {
	if len(chat) == 0 {
		return ""
	}
	lines := make([]string, 0, len(chat))
	total := 0
	for _, msg := range chat {
		content := strings.TrimSpace(msg.Content)
		if content == "" || strings.Contains(content, "[[agent-stream-len:") {
			continue
		}
		role := normalizeScrumChannelRole(msg.Role)
		switch role {
		case "tool", "thinking":
			content = trimScrumPilotText(content, 480)
		case "status", "system", "error":
			content = trimScrumPilotText(content, 360)
		default:
			content = trimScrumPilotText(content, 1200)
		}
		if content == "" {
			continue
		}
		line := role + ": " + content
		if total+len(line) > scrumPilotTranscriptMaxChars {
			lines = append(lines, "...earlier channel transcript omitted...")
			break
		}
		lines = append(lines, line)
		total += len(line)
	}
	return strings.Join(lines, "\n")
}

func selectScrumPilotRecentTurns(chat []ScrumChatMessage) []string {
	if len(chat) == 0 {
		return nil
	}
	selected := make([]string, 0, scrumPilotRecentTurnsMax)
	for i := len(chat) - 1; i >= 0 && len(selected) < scrumPilotRecentTurnsMax; i-- {
		msg := chat[i]
		content := strings.TrimSpace(msg.Content)
		if content == "" || strings.Contains(content, "[[agent-stream-len:") {
			continue
		}
		role := normalizeScrumChannelRole(msg.Role)
		switch role {
		case "user", "assistant":
			content = trimScrumPilotText(content, scrumPilotRecentTurnMaxChars)
		default:
			continue
		}
		if content == "" {
			continue
		}
		selected = append(selected, role+": "+content)
	}
	for i, j := 0, len(selected)-1; i < j; i, j = i+1, j-1 {
		selected[i], selected[j] = selected[j], selected[i]
	}
	if len(selected) == 0 {
		return nil
	}
	return append([]string{"Recent user/assistant turns:"}, selected...)
}

func formatScrumMinimalContextForPilot(ctx omni.MinimalContext) string {
	lines := []string{"Channel context (minified):"}
	if summary := strings.TrimSpace(ctx.Summary); summary != "" {
		lines = append(lines, "Summary: "+summary)
	}
	if block := formatScrumMinimalList("Facts", ctx.Facts); block != "" {
		lines = append(lines, block)
	}
	if block := formatScrumMinimalList("Constraints", ctx.Constraints); block != "" {
		lines = append(lines, block)
	}
	if block := formatScrumMinimalList("Open", ctx.OpenItems); block != "" {
		lines = append(lines, block)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func formatScrumMinimalList(label string, values []string) string {
	if len(values) == 0 {
		return ""
	}
	lines := []string{label + ":"}
	for _, value := range values {
		value = trimScrumPilotText(strings.TrimSpace(value), 320)
		if value == "" {
			continue
		}
		lines = append(lines, "- "+value)
	}
	if len(lines) == 1 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func extractJSONBlob(raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "{") {
		return raw
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return raw[start : end+1]
	}
	return raw
}

func (s *Server) persistScrumPilotMemory(ctx context.Context, card ScrumCard, projectID int64, userMessage, reply string) {
	if s.repo == nil || s.llmClient == nil {
		return
	}
	tags := scrumPilotMemoryTags(card, projectID)
	sourceBase := "scrum-pilot:" + card.ID
	userContent := trimScrumPilotText(strings.TrimSpace(userMessage), 1200)
	replyContent := trimScrumPilotText(strings.TrimSpace(reply), 1200)
	if userContent != "" {
		embedding, _ := s.llmClient.Embedding(ctx, userContent)
		_, _ = s.repo.AddMemoryChunk(ctx, sourceBase+":user", model.MemoryKindEpisodic, "user: "+userContent, tags, embedding)
	}
	if replyContent != "" {
		embedding, _ := s.llmClient.Embedding(ctx, replyContent)
		_, _ = s.repo.AddMemoryChunk(ctx, sourceBase+":assistant", model.MemoryKindEpisodic, "assistant: "+replyContent, tags, embedding)
	}
}
