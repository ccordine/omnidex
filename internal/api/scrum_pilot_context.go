package api

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/gryph/omnidex/internal/model"
)

const (
	scrumPilotMaxRelevantChunks   = 8
	scrumPilotChunkCavemanChars   = 140
	scrumPilotThinkingCavemanChars = 96
	scrumPilotThoughtMergeMax     = 280
	scrumPilotChannelBudgetChars  = 750
	scrumPilotMemoryMaxChunks     = 3
	scrumPilotMemoryMaxChars      = 180
	scrumPilotEmbedCandidateMax   = 16
)

type scrumPilotPromptContext struct {
	ChannelSummary  string
	ChannelFacts    []string
	MemoryLines     []string
	SelectedChunks  int
}

type pilotChannelChunk struct {
	Role      string
	Text      string
	CreatedAt time.Time
	Index     int
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
	matches, err := s.repo.FindRelevantMemory(ctx, embedding, tags, scrumPilotMemoryMaxChunks)
	if err != nil {
		return nil
	}
	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		content := cavemanPilotText(match.Content, scrumPilotMemoryMaxChars)
		if content == "" {
			continue
		}
		lines = append(lines, content)
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

// summarizeScrumPilotChannel searches the chronological dialog timeline for query-relevant
// chunks, then caveman-compresses them. Tool/status/system rows never enter this path.
func (s *Server) summarizeScrumPilotChannel(ctx context.Context, _ ScrumBoard, card ScrumCard, query string, memoryLines []string) scrumPilotPromptContext {
	out := scrumPilotPromptContext{MemoryLines: memoryLines}
	timeline := buildPilotChannelTimeline(card.Chat)
	if len(timeline) == 0 {
		return out
	}
	chunks := pilotChunksFromTimeline(timeline)
	selected := s.selectRelevantPilotChunks(ctx, query, chunks)
	out.ChannelSummary = summarizeScrumChannelForPilot(card.Chat, timeline)
	out.ChannelFacts = buildPilotCavemanContext(selected, scrumPilotChannelBudgetChars)
	out.SelectedChunks = len(selected)
	return out
}

func pilotChunksFromTimeline(timeline []ScrumChatMessage) []pilotChannelChunk {
	out := make([]pilotChannelChunk, 0, len(timeline))
	for i, msg := range timeline {
		role := normalizeScrumChannelRole(msg.Role)
		text := strings.TrimSpace(msg.Content)
		if text == "" || (role == "assistant" && isAgentToolLikeAssistant(text)) {
			continue
		}
		out = append(out, pilotChannelChunk{
			Role:      role,
			Text:      text,
			CreatedAt: scrumChatMessageTime(msg),
			Index:     i,
		})
	}
	return out
}

func (s *Server) selectRelevantPilotChunks(ctx context.Context, query string, chunks []pilotChannelChunk) []pilotChannelChunk {
	if len(chunks) == 0 {
		return nil
	}
	if len(chunks) <= scrumPilotMaxRelevantChunks {
		return sortPilotChunks(chunks)
	}
	query = strings.TrimSpace(query)
	selectedIdx := map[int]struct{}{}
	selected := make([]pilotChannelChunk, 0, scrumPilotMaxRelevantChunks)

	last := chunks[len(chunks)-1]
	selected = append(selected, last)
	selectedIdx[last.Index] = struct{}{}

	for i := len(chunks) - 1; i >= 0 && len(selected) < scrumPilotMaxRelevantChunks; i-- {
		if chunks[i].Role != "user" {
			continue
		}
		if _, ok := selectedIdx[chunks[i].Index]; ok {
			break
		}
		selected = append(selected, chunks[i])
		selectedIdx[chunks[i].Index] = struct{}{}
		break
	}

	remaining := scrumPilotMaxRelevantChunks - len(selected)
	if remaining <= 0 {
		return sortPilotChunks(selected)
	}

	queryTokens := pilotQueryTokens(query)
	type scored struct {
		chunk pilotChannelChunk
		score float64
	}
	candidates := make([]scored, 0, len(chunks))
	for _, chunk := range chunks {
		if _, ok := selectedIdx[chunk.Index]; ok {
			continue
		}
		score := pilotKeywordScore(queryTokens, pilotQueryTokens(chunk.Text))
		if score <= 0 {
			switch chunk.Role {
			case "error":
				score = 0.25
			case "thinking":
				score = 0.15
			}
		}
		candidates = append(candidates, scored{chunk: chunk, score: score})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return candidates[i].chunk.Index > candidates[j].chunk.Index
		}
		return candidates[i].score > candidates[j].score
	})

	if s.llmClient != nil && query != "" {
		var queryEmbedding []float64
		if emb, err := s.llmClient.Embedding(ctx, query); err == nil && len(emb) > 0 {
			queryEmbedding = emb
		}
		if len(queryEmbedding) > 0 {
			limit := scrumPilotEmbedCandidateMax
			if limit > len(candidates) {
				limit = len(candidates)
			}
			for i := 0; i < limit; i++ {
				emb, err := s.llmClient.Embedding(ctx, candidates[i].chunk.Text)
				if err != nil || len(emb) == 0 {
					continue
				}
				candidates[i].score += pilotEmbeddingSimilarity(queryEmbedding, emb)
			}
			sort.SliceStable(candidates, func(i, j int) bool {
				if candidates[i].score == candidates[j].score {
					return candidates[i].chunk.Index > candidates[j].chunk.Index
				}
				return candidates[i].score > candidates[j].score
			})
		}
	}

	for _, item := range candidates {
		if remaining <= 0 {
			break
		}
		if _, ok := selectedIdx[item.chunk.Index]; ok {
			continue
		}
		selected = append(selected, item.chunk)
		selectedIdx[item.chunk.Index] = struct{}{}
		remaining--
	}
	return sortPilotChunks(selected)
}

func sortPilotChunks(chunks []pilotChannelChunk) []pilotChannelChunk {
	if len(chunks) <= 1 {
		return chunks
	}
	sort.SliceStable(chunks, func(i, j int) bool {
		left, right := chunks[i], chunks[j]
		if !left.CreatedAt.Equal(right.CreatedAt) {
			if left.CreatedAt.IsZero() {
				return false
			}
			if right.CreatedAt.IsZero() {
				return true
			}
			return left.CreatedAt.Before(right.CreatedAt)
		}
		return left.Index < right.Index
	})
	return chunks
}

func buildPilotCavemanContext(chunks []pilotChannelChunk, budget int) []string {
	if len(chunks) == 0 || budget <= 0 {
		return nil
	}
	lines := []string{"Channel timeline (chronological, caveman):"}
	used := len(lines[0]) + 1
	for _, chunk := range chunks {
		prefix := pilotCavemanPrefix(chunk.Role)
		limit := scrumPilotChunkCavemanChars
		if chunk.Role == "thinking" {
			limit = scrumPilotThinkingCavemanChars
		}
		body := cavemanPilotText(chunk.Text, limit)
		if body == "" {
			continue
		}
		line := prefix + body
		if used+len(line)+1 > budget {
			break
		}
		lines = append(lines, line)
		used += len(line) + 1
	}
	if len(lines) == 1 {
		return nil
	}
	return lines
}

func pilotCavemanPrefix(role string) string {
	switch normalizeScrumChannelRole(role) {
	case "user":
		return "u: "
	case "assistant":
		return "agent: "
	case "thinking":
		return "think: "
	case "error":
		return "err: "
	default:
		return role + ": "
	}
}

func cavemanPilotText(raw string, maxChars int) string {
	text := strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
	if text == "" {
		return ""
	}
	if maxChars <= 0 || len(text) <= maxChars {
		return text
	}
	marker := " … "
	if maxChars <= len(marker)+12 {
		return text[:maxChars]
	}
	head := (maxChars - len(marker)) * 2 / 3
	tail := maxChars - len(marker) - head
	if tail < 0 {
		tail = 0
	}
	if head <= 0 {
		return text[:maxChars]
	}
	return text[:head] + marker + text[len(text)-tail:]
}

func pilotQueryTokens(text string) map[string]struct{} {
	text = strings.ToLower(text)
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		if len(field) < 2 {
			continue
		}
		out[field] = struct{}{}
	}
	return out
}

func pilotKeywordScore(query, doc map[string]struct{}) float64 {
	if len(query) == 0 || len(doc) == 0 {
		return 0
	}
	matches := 0
	for token := range query {
		if _, ok := doc[token]; ok {
			matches++
		}
	}
	return float64(matches) / float64(len(query))
}

func pilotEmbeddingSimilarity(a, b []float64) float64 {
	if len(a) == 0 || len(b) == 0 || len(a) != len(b) {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}

// buildPilotChannelTimeline keeps user, thinking, error, and final assistant replies in real
// event order. Tool/status/system noise is dropped. Consecutive thinking bursts merge into one
// self-dialogue block; consecutive assistant stream chunks collapse to the last reply.
func buildPilotChannelTimeline(chat []ScrumChatMessage) []ScrumChatMessage {
	if len(chat) == 0 {
		return nil
	}
	out := make([]ScrumChatMessage, 0, len(chat))
	for _, msg := range sortScrumChatChronological(chat) {
		content := pilotChannelMessageText(msg)
		if content == "" {
			continue
		}
		role := normalizeScrumChannelRole(msg.Role)
		switch role {
		case "user", "error", "thinking":
			entry := ScrumChatMessage{
				Role:      role,
				Content:   content,
				CreatedAt: msg.CreatedAt,
			}
			out = mergePilotTimelineEntry(out, entry)
		case "assistant":
			if isAgentToolLikeAssistant(content) {
				continue
			}
			entry := ScrumChatMessage{
				Role:      "assistant",
				Content:   content,
				CreatedAt: msg.CreatedAt,
			}
			out = mergePilotTimelineEntry(out, entry)
		default:
			// tool, status, system — never enter pilot context
		}
	}
	return out
}

func mergePilotTimelineEntry(out []ScrumChatMessage, msg ScrumChatMessage) []ScrumChatMessage {
	if len(out) == 0 {
		return append(out, msg)
	}
	last := &out[len(out)-1]
	lastRole := normalizeScrumChannelRole(last.Role)
	role := normalizeScrumChannelRole(msg.Role)
	switch {
	case role == "thinking" && lastRole == "thinking":
		last.Content = mergePilotThoughtText(last.Content, msg.Content)
		if strings.TrimSpace(msg.CreatedAt) != "" {
			last.CreatedAt = msg.CreatedAt
		}
	case role == "assistant" && lastRole == "assistant":
		last.Content = msg.Content
		last.CreatedAt = msg.CreatedAt
	default:
		out = append(out, msg)
	}
	return out
}

func mergePilotThoughtText(existing, next string) string {
	existing = strings.TrimSpace(existing)
	next = strings.TrimSpace(next)
	if existing == "" {
		return cavemanPilotText(next, scrumPilotThoughtMergeMax)
	}
	if next == "" {
		return existing
	}
	return cavemanPilotText(existing+" · "+next, scrumPilotThoughtMergeMax)
}

func pilotChannelMessageText(msg ScrumChatMessage) string {
	content := strings.TrimSpace(stripAssistantStreamMarker(msg.Content))
	if content == "" || strings.Contains(content, "[[agent-stream-len:") {
		return ""
	}
	return content
}

func isAgentToolLikeAssistant(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return true
	}
	lower := strings.ToLower(content)
	if strings.HasPrefix(content, "{") || strings.HasPrefix(content, "[{") {
		if strings.Contains(content, `"type"`) || strings.Contains(content, `"tool_call"`) || strings.Contains(content, `"function"`) {
			return true
		}
	}
	if strings.HasPrefix(lower, "agent stream:") || strings.HasPrefix(lower, "agent output:") {
		return true
	}
	if strings.HasPrefix(lower, "[tool]") {
		return true
	}
	if strings.Contains(lower, "tool_call") || strings.Contains(lower, "function_call") {
		return true
	}
	return false
}

func summarizeScrumChannelForPilot(fullChat, timeline []ScrumChatMessage) string {
	if len(timeline) == 0 {
		return ""
	}
	if note := lastAgentOutcomeNote(fullChat); note != "" {
		return cavemanPilotText(note, 220)
	}
	return ""
}

func lastAgentOutcomeNote(chat []ScrumChatMessage) string {
	for i := len(chat) - 1; i >= 0; i-- {
		role := normalizeScrumChannelRole(chat[i].Role)
		content := cavemanPilotText(pilotChannelMessageText(chat[i]), 180)
		if content == "" {
			continue
		}
		switch role {
		case "error":
			return "last err " + content
		case "status":
			lower := strings.ToLower(content)
			if strings.Contains(lower, "finished") || strings.Contains(lower, "running") || strings.Contains(lower, "completed") {
				return "status " + content
			}
		case "assistant":
			if !isAgentToolLikeAssistant(content) {
				return "last agent " + content
			}
		}
	}
	return ""
}

func (s *Server) persistScrumPilotMemory(ctx context.Context, card ScrumCard, projectID int64, userMessage, reply string) {
	if s.repo == nil || s.llmClient == nil {
		return
	}
	tags := scrumPilotMemoryTags(card, projectID)
	sourceBase := "scrum-pilot:" + card.ID
	userContent := cavemanPilotText(strings.TrimSpace(userMessage), 400)
	replyContent := cavemanPilotText(strings.TrimSpace(reply), 400)
	if userContent != "" {
		embedding, _ := s.llmClient.Embedding(ctx, userContent)
		_, _ = s.repo.AddMemoryChunk(ctx, sourceBase+":user", model.MemoryKindEpisodic, "u: "+userContent, tags, embedding)
	}
	if replyContent != "" {
		embedding, _ := s.llmClient.Embedding(ctx, replyContent)
		_, _ = s.repo.AddMemoryChunk(ctx, sourceBase+":assistant", model.MemoryKindEpisodic, "agent: "+replyContent, tags, embedding)
	}
}

// collapsePilotChannelDialog is kept as an alias for tests and legacy callers.
func collapsePilotChannelDialog(chat []ScrumChatMessage) []ScrumChatMessage {
	return buildPilotChannelTimeline(chat)
}
