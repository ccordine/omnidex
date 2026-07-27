package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

type ProjectPlanningChatConfig = model.ProjectPlanningConfig

type ProjectPlanningCardDraft struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Column      string   `json:"column"`
	Checklist   []string `json:"checklist"`
}

type ProjectPlanningLLMResponse struct {
	Reply       string                     `json:"reply"`
	Suggestions []ScrumCoachSuggestion     `json:"suggestions"`
	CardDrafts  []ProjectPlanningCardDraft `json:"card_drafts"`
}

func defaultProjectPlanningChatConfig() ProjectPlanningChatConfig {
	return ProjectPlanningChatConfig{ReasoningMode: "instant"}
}

func validateProjectPlanningChatConfig(config ProjectPlanningChatConfig) (ProjectPlanningChatConfig, error) {
	config.Model = strings.TrimSpace(config.Model)
	config.ReasoningMode = strings.ToLower(strings.TrimSpace(config.ReasoningMode))
	switch config.ReasoningMode {
	case "instant", "thinking":
		return config, nil
	default:
		return ProjectPlanningChatConfig{}, fmt.Errorf("unsupported project planning reasoning mode %q", config.ReasoningMode)
	}
}

func normalizeProjectPlanningMode(message, mode string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" || mode == "chat" {
		if strings.HasPrefix(strings.TrimSpace(message), "/") {
			parts := strings.Fields(message)
			switch strings.ToLower(parts[0]) {
			case "/plan":
				return "plan", nil
			case "/research", "/researching":
				return "research", nil
			case "/scan":
				return "scan", nil
			case "/cards":
				return "cards", nil
			case "/batch":
				return "batch", nil
			default:
				return "", fmt.Errorf("unsupported project planning command %q", parts[0])
			}
		}
		return "chat", nil
	}
	switch mode {
	case "plan", "research", "scan", "cards", "batch":
		return mode, nil
	default:
		return "", fmt.Errorf("unsupported project planning mode %q", mode)
	}
}

func projectPlanningModeSystem(mode string) (string, error) {
	base := strings.Join([]string{
		"You are the Omni project planner — a productivity assistant for an entire software project.",
		"You help discuss goals, refine the backlog, draft scrum cards, spot risks, and organize work.",
		"You never execute code, run builds, or modify files directly.",
		"When suggesting cards, populate card_drafts with concrete backlog items.",
		"Respond with JSON only (no markdown fences):",
		`{"reply":"markdown conversation","suggestions":[{"level":"info|warn|tip","text":"..."}],"card_drafts":[{"title":"...","description":"...","column":"backlog|ready|assigned|in_progress|review|blocked|error|done","checklist":["..."]}]}`,
	}, "\n")
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "scan":
		return base + "\nMode: scan — review the project board and emit proactive observations (stale cards, gaps, priorities). Keep reply brief.", nil
	case "plan":
		return base + "\nMode: plan — structure upcoming work: milestones, card breakdowns, dependencies, and sequencing.", nil
	case "research":
		return base + "\nMode: research — synthesize web research snippets and memory into actionable planning guidance. Cite uncertainties.", nil
	case "cards":
		return base + "\nMode: cards — focus on drafting well-scoped scrum cards. Populate card_drafts richly.", nil
	case "batch":
		return base + "\nMode: batch — synthesize research into 3–8 reviewable cards covering setup, implementation, and verification where relevant.", nil
	case "chat":
		return base + "\nMode: chat — collaborative project planning dialogue.", nil
	default:
		return "", fmt.Errorf("unsupported project planning mode %q", mode)
	}
}

func parseProjectPlanningLLMResponse(raw string) (ProjectPlanningLLMResponse, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ProjectPlanningLLMResponse{}, fmt.Errorf("project planner returned an empty response")
	}
	var response ProjectPlanningLLMResponse
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return ProjectPlanningLLMResponse{}, fmt.Errorf("decode project planner response: %w", err)
	}
	if err := requireJSONEOF(decoder, "project planner response"); err != nil {
		return ProjectPlanningLLMResponse{}, err
	}
	response.Reply = strings.TrimSpace(response.Reply)
	for i := range response.Suggestions {
		suggestion := &response.Suggestions[i]
		suggestion.Level = strings.ToLower(strings.TrimSpace(suggestion.Level))
		suggestion.Text = strings.TrimSpace(suggestion.Text)
		switch suggestion.Level {
		case "info", "warn", "tip":
		default:
			return ProjectPlanningLLMResponse{}, fmt.Errorf("project planner suggestion %d has unsupported level %q", i, suggestion.Level)
		}
		if suggestion.Text == "" {
			return ProjectPlanningLLMResponse{}, fmt.Errorf("project planner suggestion %d text is required", i)
		}
	}
	for i := range response.CardDrafts {
		draft := &response.CardDrafts[i]
		draft.Title = strings.TrimSpace(draft.Title)
		draft.Description = strings.TrimSpace(draft.Description)
		draft.Column = normalizeScrumColumn(draft.Column)
		if draft.Title == "" || draft.Column == "" {
			return ProjectPlanningLLMResponse{}, fmt.Errorf("project planner draft %d requires title and supported column", i)
		}
		for itemIndex := range draft.Checklist {
			draft.Checklist[itemIndex] = strings.TrimSpace(draft.Checklist[itemIndex])
			if draft.Checklist[itemIndex] == "" {
				return ProjectPlanningLLMResponse{}, fmt.Errorf("project planner draft %d checklist item %d is empty", i, itemIndex)
			}
		}
	}
	if response.Reply == "" && len(response.Suggestions) == 0 && len(response.CardDrafts) == 0 {
		return ProjectPlanningLLMResponse{}, fmt.Errorf("project planner response contains no usable content")
	}
	return response, nil
}

func (s *Server) projectPlanningModel(project model.Project, config ProjectPlanningChatConfig) (string, error) {
	if modelName := strings.TrimSpace(config.Model); modelName != "" {
		return modelName, nil
	}
	resolved, _, err := s.resolveModelConfig(project, ScrumCard{})
	if err != nil {
		return "", err
	}
	keys := []string{"planner_model", "default_model"}
	if config.ReasoningMode == "thinking" {
		keys = []string{"thinking_model", "reasoning_model"}
	}
	for _, key := range keys {
		if modelName := resolved.Get(key); modelName != "" {
			return modelName, nil
		}
	}
	return "", fmt.Errorf("project planning %s model is not configured (expected %s)", config.ReasoningMode, strings.Join(keys, " or "))
}
