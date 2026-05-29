package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
)

func (a *App) handleSearchTurn(session *Session, query string) (Turn, string, error) {
	turnID := fmt.Sprintf("turn_%06d", len(session.Turns)+1)
	userInput := "/search " + query
	events := []Event{a.newEvent("search_started", "Quick search started", map[string]string{"query": query})}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	sections := []string{}
	deps := a.buildThinkingToolDeps()
	if deps.MemorySearch != nil {
		events = append(events, a.newEvent("search_memory_started", "Searching session memory", map[string]string{"query": query}))
		memoryText, err := deps.MemorySearch(ctx, query)
		if err != nil {
			events = append(events, a.newEvent("search_memory_failed", "Memory search failed", map[string]string{"error": truncateOutput(err.Error())}))
		} else if strings.TrimSpace(memoryText) != "" && !strings.HasPrefix(memoryText, "no memory matches") {
			sections = append(sections, "Memory matches:\n"+memoryText)
			events = append(events, a.newEvent("search_memory_completed", "Memory search completed", map[string]string{"query": query}))
		}
	}

	if a.web != nil {
		events = append(events, a.newEvent("search_web_started", "Web search started", map[string]string{"query": query}))
		results, err := a.web.SearchAll(ctx, query)
		if err != nil {
			events = append(events, a.newEvent("search_web_failed", "Web search failed", map[string]string{"error": truncateOutput(err.Error())}))
		} else {
			events = append(events, a.newEvent("search_web_completed", "Web search completed", webSearchTimelineDetails(query, results)))
			contextText := websearch.BuildContext(results, 5000)
			if strings.TrimSpace(contextText) != "" {
				sections = append(sections, "Web results:\n"+contextText)
			}
		}
	} else {
		events = append(events, a.newEvent("search_web_skipped", "Web search unavailable", nil))
	}

	response := strings.TrimSpace(strings.Join(sections, "\n\n"))
	if response == "" {
		response = "No memory or web results found for: " + query
	} else if a.ollama != nil {
		summaryCtx, summaryCancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer summaryCancel()
		summary, err := a.summarizeSearchResults(summaryCtx, query, response)
		if err == nil && strings.TrimSpace(summary) != "" {
			response = summary
		}
	}

	response = a.reviewFinalResponse(context.Background(), userInput, response, []string{"slash_search"}, func(eventType, summary string, details map[string]string) {
		events = append(events, a.newEvent(eventType, summary, details))
	})
	events = append(events, a.newEvent("search_completed", "Quick search completed", map[string]string{"query": query}))

	turn := Turn{
		ID:                   turnID,
		UserInput:            userInput,
		IntentClassification: IntentExecution,
		Confidence:           1.0,
		ReasonCodes:          []string{"slash_search"},
		Response:             response,
		Events:               events,
		CreatedAt:            nowUTC(),
	}
	return turn, response, nil
}

func (a *App) summarizeSearchResults(ctx context.Context, query, evidence string) (string, error) {
	req := OllamaChatRequest{
		Messages: []OllamaMessage{
			{Role: "system", Content: MinimalOutputContract + " Summarize search evidence into a concise user-facing answer. Cite key facts only."},
			{Role: "user", Content: "Query: " + query + "\n\nEvidence:\n" + truncateForStructuredContext(evidence, 6000)},
		},
	}
	resp, err := a.ollama.ChatRaw(ctx, req)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(resp.Content), nil
}

func (a *App) handleThoughtsCommand(session *Session, turnFilter string) (string, error) {
	turnFilter = strings.TrimSpace(turnFilter)
	turnIDs, err := ListThoughtTurnIDs(session.WorkspaceHash)
	if err != nil {
		return "", err
	}
	if len(turnIDs) == 0 {
		root := ThoughtStoreWorkspaceDir(session.WorkspaceHash)
		return fmt.Sprintf("No thought logs for this workspace yet.\nStore: %s", root), nil
	}
	sort.Strings(turnIDs)

	if turnFilter == "" {
		lines := []string{
			"Thought channels for workspace " + session.WorkspaceHash + ":",
			"Store: " + ThoughtStoreWorkspaceDir(session.WorkspaceHash),
			"",
		}
		start := 0
		if len(turnIDs) > 12 {
			start = len(turnIDs) - 12
		}
		for _, turnID := range turnIDs[start:] {
			summary, sumErr := SummarizeThoughtTurn(session.WorkspaceHash, turnID)
			if sumErr != nil {
				lines = append(lines, "- "+turnID+": (unreadable)")
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s: %d channel(s), latest=%s", turnID, summary.ChannelCount, truncateOutput(summary.LatestConclusion)))
		}
		lines = append(lines, "", "Use /thoughts <turn_id> to inspect one turn.")
		return strings.Join(lines, "\n"), nil
	}

	channels, messages, err := LoadThoughtTurnLog(session.WorkspaceHash, turnFilter)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("No thought log for turn %q. Known turns: %s", turnFilter, strings.Join(turnIDs, ", ")), nil
		}
		return "", err
	}
	lines := []string{
		fmt.Sprintf("Thought log %s (%d channel(s), %d message(s)):", turnFilter, len(channels), len(messages)),
		"File: " + ThoughtTurnLogPath(session.WorkspaceHash, turnFilter),
		"",
	}
	for _, channel := range channels {
		lines = append(lines, fmt.Sprintf("[%s] trigger=%s concluded=%t", channel.ID, channel.Trigger, channel.Concluded))
		if strings.TrimSpace(channel.Conclusion) != "" {
			lines = append(lines, "  conclusion: "+truncateOutput(channel.Conclusion))
		}
		if strings.TrimSpace(channel.RecoveryToolTask) != "" {
			lines = append(lines, "  recovery: "+truncateOutput(channel.RecoveryToolTask))
		}
		for _, msg := range messages {
			if msg.ChannelID != channel.ID {
				continue
			}
			prefix := msg.Role
			if msg.ToolName != "" {
				prefix += "/" + msg.ToolName
			}
			lines = append(lines, fmt.Sprintf("  %s: %s", prefix, truncateOutput(msg.Content)))
		}
		lines = append(lines, "")
	}
	return strings.TrimSpace(strings.Join(lines, "\n")), nil
}

func ThoughtStoreRoot() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ".omni/thoughts"
	}
	return filepath.Join(home, ".omni", "thoughts")
}

func ThoughtStoreWorkspaceDir(workspaceHash string) string {
	return filepath.Join(ThoughtStoreRoot(), workspaceHash)
}

func ThoughtTurnLogPath(workspaceHash, turnID string) string {
	return filepath.Join(ThoughtStoreWorkspaceDir(workspaceHash), turnID, "channels.jsonl")
}

type ThoughtTurnSummary struct {
	ChannelCount     int
	LatestConclusion string
}

func ListThoughtTurnIDs(workspaceHash string) ([]string, error) {
	dir := ThoughtStoreWorkspaceDir(workspaceHash)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			out = append(out, entry.Name())
		}
	}
	return out, nil
}

func SummarizeThoughtTurn(workspaceHash, turnID string) (ThoughtTurnSummary, error) {
	channels, _, err := LoadThoughtTurnLog(workspaceHash, turnID)
	if err != nil {
		return ThoughtTurnSummary{}, err
	}
	summary := ThoughtTurnSummary{ChannelCount: len(channels)}
	for i := len(channels) - 1; i >= 0; i-- {
		if text := strings.TrimSpace(channels[i].Conclusion); text != "" {
			summary.LatestConclusion = text
			break
		}
	}
	return summary, nil
}

func LoadThoughtTurnLog(workspaceHash, turnID string) ([]ThoughtChannel, []ThoughtMessage, error) {
	path := ThoughtTurnLogPath(workspaceHash, turnID)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	channelsByID := map[string]ThoughtChannel{}
	var messages []ThoughtMessage
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var record thoughtStoreRecord
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			continue
		}
		switch record.Kind {
		case "channel_open", "channel_close":
			if record.Channel.ID != "" {
				channelsByID[record.Channel.ID] = record.Channel
			}
		case "message":
			if record.Message.ChannelID != "" {
				messages = append(messages, record.Message)
			}
		}
	}
	channels := make([]ThoughtChannel, 0, len(channelsByID))
	for _, channel := range channelsByID {
		channels = append(channels, channel)
	}
	sort.Slice(channels, func(i, j int) bool {
		return channels[i].CreatedAt < channels[j].CreatedAt
	})
	return channels, messages, nil
}
