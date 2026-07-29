package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/websearch"
)

func buildProjectPlanningUserPrompt(
	project model.Project,
	board ScrumBoard,
	config ProjectPlanningChatConfig,
	mode, message string,
	memoryLines, mapLines, researchLines []string,
	history []ScrumChatMessage,
) string {
	lines := []string{
		"Project: " + project.Name,
		"Directory: " + board.ProjectDirectory,
		"State: " + strings.TrimSpace(project.ProjectState),
	}
	if description := strings.TrimSpace(project.Description); description != "" {
		lines = append(lines, "Description: "+description)
	}
	lines = append(lines, "Reasoning mode: "+config.ReasoningMode, "Board summary:")
	lines = append(lines, summarizeScrumBoard(board)...)
	lines = append(lines, summarizeScrumFlowBoard(board)...)
	if len(mapLines) > 0 {
		lines = append(lines, "Codebase map:", strings.Join(mapLines, "\n"))
	}
	if len(memoryLines) > 0 {
		lines = append(lines, "Relevant memory:", strings.Join(memoryLines, "\n---\n"))
	}
	if len(researchLines) > 0 {
		lines = append(lines, "Web research:", strings.Join(researchLines, "\n---\n"))
	}
	for _, message := range history {
		lines = append(lines, message.Role+": "+message.Content)
	}
	if strings.TrimSpace(message) != "" {
		lines = append(lines, "user: "+strings.TrimSpace(message))
	}
	return strings.Join(append(lines, "Mode: "+mode), "\n")
}

func summarizeScrumBoard(board ScrumBoard) []string {
	if len(board.Cards) == 0 {
		return []string{"(no cards yet)"}
	}
	byColumn := map[string][]ScrumCard{}
	for _, card := range board.Cards {
		column := normalizeScrumColumn(card.Column)
		if column == "" {
			column = "backlog"
		}
		byColumn[column] = append(byColumn[column], card)
	}
	out := make([]string, 0, len(board.Cards))
	for _, column := range board.Columns {
		cards := byColumn[column]
		if len(cards) == 0 {
			continue
		}
		out = append(out, fmt.Sprintf("[%s] %d cards", column, len(cards)))
		for _, card := range cards {
			line := "- " + strings.TrimSpace(card.Title)
			if card.PlayState == scrumPlayRunning {
				line += " (running)"
			}
			if description := strings.TrimSpace(card.Description); description != "" {
				line += ": " + trimForPrompt(description, 120)
			}
			out = append(out, line)
		}
	}
	return out
}

func summarizeScrumFlowBoard(board ScrumBoard) []string {
	out := []string{}
	for _, card := range board.Cards {
		metrics := parseScrumFlowMetrics(card.FlowMetrics)
		if metrics.CompletionStatus != "likely_incomplete" && metrics.AssignedReturns == 0 && metrics.IncompleteScore < 25 {
			continue
		}
		line := fmt.Sprintf("- %s [%s] status=%s score=%d", strings.TrimSpace(card.Title), normalizeScrumColumn(card.Column), metrics.CompletionStatus, metrics.IncompleteScore)
		if metrics.AssignedReturns > 0 {
			line += fmt.Sprintf(" assigned_returns=%d", metrics.AssignedReturns)
		}
		if metrics.ChannelMessages+metrics.PlanningMessages > 0 {
			line += fmt.Sprintf(" messages=%d", metrics.ChannelMessages+metrics.PlanningMessages)
		}
		if len(metrics.Signals) > 0 {
			line += " signals: " + strings.Join(metrics.Signals, "; ")
		}
		out = append(out, line)
	}
	if len(out) == 0 {
		return nil
	}
	return append([]string{"Flow metrics (incomplete / churn signals):"}, out...)
}

func trimForPrompt(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "…"
}

func (s *Server) projectPlanningMemoryContext(ctx context.Context, project model.Project, query string) ([]string, error) {
	if s == nil || s.repo == nil || s.llmClient == nil || ctx == nil {
		return nil, fmt.Errorf("project planning memory requires PostgreSQL, context, and an embedding client")
	}
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("project planning memory query is required")
	}
	embedding, err := s.llmClient.Embedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed project planning memory query: %w", err)
	}
	if len(embedding) == 0 {
		return nil, fmt.Errorf("embed project planning memory query: provider returned an empty vector")
	}
	tags := []string{fmt.Sprintf("project:%d", project.ID), "scrum", "project-chat"}
	matches, err := s.repo.FindRelevantMemory(ctx, embedding, tags, 8)
	if err != nil {
		return nil, fmt.Errorf("find project planning memory: %w", err)
	}
	lines := make([]string, 0, len(matches))
	for _, match := range matches {
		if content := strings.TrimSpace(match.Content); content != "" {
			lines = append(lines, content)
		}
	}
	return lines, nil
}

func (s *Server) projectPlanningMapContext(ctx context.Context, project model.Project) ([]string, error) {
	location := strings.TrimSpace(project.Location)
	if location == "" {
		return nil, fmt.Errorf("project planning codebase map requires a project location")
	}
	payload, err := s.loadProjectCodebaseMapPayload(ctx, location)
	if err != nil {
		return nil, fmt.Errorf("load project planning codebase map: %w", err)
	}
	exists, ok := payload["exists"].(bool)
	if !ok {
		return nil, fmt.Errorf("project planning codebase map has invalid exists value")
	}
	if !exists {
		return []string{"(codebase map not scanned yet)"}, nil
	}
	root, ok := payload["root"].(string)
	if !ok || strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("project planning codebase map has no root")
	}
	fileCount, ok := payload["file_count"].(int)
	if !ok || fileCount < 0 {
		return nil, fmt.Errorf("project planning codebase map has invalid file count")
	}
	modules, ok := payload["modules"].([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("project planning codebase map has invalid modules")
	}
	entrypoints, ok := payload["entrypoints"].([]map[string]any)
	if !ok {
		return nil, fmt.Errorf("project planning codebase map has invalid entrypoints")
	}
	lines := []string{"root: " + root, fmt.Sprintf("files: %d", fileCount)}
	for i, module := range modules {
		if i >= 6 {
			break
		}
		path, pathOK := module["path"].(string)
		purpose, purposeOK := module["purpose"].(string)
		if !pathOK || !purposeOK || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("project planning codebase map module %d is invalid", i)
		}
		line := strings.TrimSpace(path)
		if strings.TrimSpace(purpose) != "" {
			line += " — " + trimForPrompt(purpose, 100)
		}
		lines = append(lines, line)
	}
	if len(entrypoints) > 0 {
		lines = append(lines, "entrypoints:")
	}
	for i, entrypoint := range entrypoints {
		if i >= 4 {
			break
		}
		path, ok := entrypoint["path"].(string)
		if !ok || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("project planning codebase map entrypoint %d is invalid", i)
		}
		lines = append(lines, "- "+strings.TrimSpace(path))
	}
	return lines, nil
}

func (s *Server) projectPlanningResearchContext(ctx context.Context, message, mode string) ([]string, bool, error) {
	if mode != "research" && mode != "batch" {
		return nil, false, nil
	}
	if !s.webSearchEnabled {
		return nil, false, fmt.Errorf("project planning %s mode requires web search to be enabled", mode)
	}
	query := strings.TrimSpace(message)
	if query == "" {
		return nil, false, fmt.Errorf("project planning %s mode requires a research query", mode)
	}
	searcher := websearch.New(s.webSearchProviders, s.webSearchTimeout, 3000, 6000)
	searchContext, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	results, err := searcher.SearchAll(searchContext, query)
	if err != nil {
		return nil, false, fmt.Errorf("project planning web search: %w", err)
	}
	if len(results) == 0 {
		return nil, false, fmt.Errorf("project planning web search returned no results")
	}
	lines := make([]string, 0, minInt(5, len(results)))
	for i, result := range results {
		if i >= 5 {
			break
		}
		snippet := strings.TrimSpace(result.Snippet)
		if snippet == "" {
			snippet = strings.TrimSpace(result.Content)
		}
		if strings.TrimSpace(result.Title) == "" || strings.TrimSpace(result.URL) == "" || snippet == "" {
			return nil, false, fmt.Errorf("project planning web search result %d is incomplete", i)
		}
		lines = append(lines, strings.Join([]string{
			"title: " + strings.TrimSpace(result.Title),
			"url: " + strings.TrimSpace(result.URL),
			"snippet: " + trimForPrompt(snippet, 400),
		}, "\n"))
	}
	return lines, true, nil
}
