package projectdebugger

import (
	"context"
	"fmt"
	"strings"
)

type LLMClient interface {
	Generate(ctx context.Context, model, prompt string) (string, error)
}

const debuggerSystemPrompt = `You are the Omni project analyzer — a code-quality analyst for a software project.
Your job is to scan project context (codebase map, board, risks, tests) and identify concrete backlog work: bugs, mistakes, cleanup, refactors, optimization points, reliability problems, security risks, and missing tests.
You never execute code or modify files directly.
Focus on actionable findings: obvious errors, subtle broken flows, missing error handling, stale modules, test gaps, security risks, inconsistent patterns, needless duplication, brittle abstractions, and performance issues.
Avoid duplicating existing open backlog items when their titles clearly match.
Respond with JSON only (no markdown fences):
{"summary":"brief analysis overview","bug_tickets":[{"title":"...","description":"markdown details with file refs when known","severity":"critical|high|medium|low","column":"backlog","checklist":["verify step"],"ref_files":["path/to/file.go"],"tags":["bug|cleanup|refactor|optimization|reliability|security|test-gap"]}],"suggestions":["optional process tip"]}
The bug_tickets array is the backlog-card list. Emit 3-8 backlog cards when issues exist; emit zero when the available evidence supports no finding.`

func MapContextLines(payload map[string]any) ([]string, error) {
	if payload == nil {
		return nil, fmt.Errorf("project debugger codebase map is required")
	}
	exists, ok := payload["exists"].(bool)
	if !ok {
		return nil, fmt.Errorf("project debugger codebase map has invalid exists value")
	}
	if !exists {
		return nil, fmt.Errorf("project debugger codebase map has not been scanned")
	}
	root, ok := payload["root"].(string)
	if !ok || strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("project debugger codebase map root is required")
	}
	fileCount, ok := payload["file_count"].(int)
	if !ok || fileCount < 0 {
		return nil, fmt.Errorf("project debugger codebase map file count is invalid")
	}
	modules, ok := payload["modules"].([]any)
	if !ok {
		return nil, fmt.Errorf("project debugger codebase map modules are invalid")
	}
	risks, ok := payload["risks"].([]any)
	if !ok {
		return nil, fmt.Errorf("project debugger codebase map risks are invalid")
	}
	tests, ok := payload["tests"].([]any)
	if !ok {
		return nil, fmt.Errorf("project debugger codebase map tests are invalid")
	}
	openQuestions, ok := payload["open_questions"].([]any)
	if !ok {
		return nil, fmt.Errorf("project debugger codebase map open questions are invalid")
	}
	tree, ok := payload["tree_preview"].(string)
	if !ok {
		return nil, fmt.Errorf("project debugger codebase map tree preview is invalid")
	}

	lines := []string{"root: " + strings.TrimSpace(root), fmt.Sprintf("files: %d", fileCount)}
	for i, raw := range modules {
		if i >= 8 {
			break
		}
		module, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("project debugger codebase map module %d is invalid", i)
		}
		path, pathOK := module["path"].(string)
		purpose, purposeOK := module["purpose"].(string)
		if !pathOK || !purposeOK || strings.TrimSpace(path) == "" {
			return nil, fmt.Errorf("project debugger codebase map module %d fields are invalid", i)
		}
		line := strings.TrimSpace(path)
		if strings.TrimSpace(purpose) != "" {
			line += " — " + trimForPrompt(purpose, 100)
		}
		lines = append(lines, line)
	}
	if len(risks) > 0 {
		lines = append(lines, "known risks:")
	}
	for i, raw := range risks {
		if i >= 6 {
			break
		}
		risk, ok := raw.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("project debugger codebase map risk %d is invalid", i)
		}
		area, areaOK := risk["area"].(string)
		text, riskOK := risk["risk"].(string)
		if !areaOK || !riskOK || (strings.TrimSpace(area) == "" && strings.TrimSpace(text) == "") {
			return nil, fmt.Errorf("project debugger codebase map risk %d fields are invalid", i)
		}
		lines = append(lines, fmt.Sprintf("- %s: %s", strings.TrimSpace(area), trimForPrompt(text, 120)))
	}
	lines = append(lines, fmt.Sprintf("test files indexed: %d", len(tests)))
	if len(openQuestions) > 0 {
		lines = append(lines, "open questions:")
	}
	for i, raw := range openQuestions {
		if i >= 4 {
			break
		}
		question, ok := raw.(string)
		if !ok || strings.TrimSpace(question) == "" {
			return nil, fmt.Errorf("project debugger codebase map open question %d is invalid", i)
		}
		lines = append(lines, "- "+trimForPrompt(question, 120))
	}
	if strings.TrimSpace(tree) != "" {
		lines = append(lines, "tree preview:", trimForPrompt(tree, 1800))
	}
	return lines, nil
}

func BoardSummaryLines(cards []BoardCard) ([]string, error) {
	if len(cards) == 0 {
		return []string{"(no scrum cards yet)"}, nil
	}
	byColumn := map[string][]BoardCard{}
	for i, card := range cards {
		card.Title = strings.TrimSpace(card.Title)
		card.Column = strings.ToLower(strings.TrimSpace(card.Column))
		if card.Title == "" || !validScrumColumn(card.Column) {
			return nil, fmt.Errorf("project debugger board card %d has invalid title or column", i)
		}
		byColumn[card.Column] = append(byColumn[card.Column], card)
	}
	out := make([]string, 0, len(cards)+4)
	for _, column := range scrumColumns {
		items := byColumn[column]
		if len(items) == 0 {
			continue
		}
		out = append(out, fmt.Sprintf("[%s] %d cards", column, len(items)))
		for _, card := range items {
			line := "- " + card.Title
			if card.PlayState == "running" {
				line += " (running)"
			}
			if description := strings.TrimSpace(card.Description); description != "" {
				line += ": " + trimForPrompt(description, 120)
			}
			if len(card.Tags) > 0 {
				line += " [" + strings.Join(card.Tags, ", ") + "]"
			}
			out = append(out, line)
		}
	}
	return out, nil
}

func BuildPrompt(input Input) (string, string, error) {
	input.ProjectName = strings.TrimSpace(input.ProjectName)
	input.ProjectLocation = strings.TrimSpace(input.ProjectLocation)
	input.AgentSystem = strings.TrimSpace(input.AgentSystem)
	if input.ProjectName == "" || input.ProjectLocation == "" || input.AgentSystem == "" {
		return "", "", fmt.Errorf("project debugger prompt requires project name, location, and execution agent")
	}
	mapLines, err := MapContextLines(input.MapPayload)
	if err != nil {
		return "", "", err
	}
	boardLines, err := BoardSummaryLines(input.BoardCards)
	if err != nil {
		return "", "", err
	}
	lines := []string{
		"Project: " + input.ProjectName,
		"Directory: " + input.ProjectLocation,
		"State: " + strings.TrimSpace(input.ProjectState),
		"Execution agent: " + input.AgentSystem,
	}
	if description := strings.TrimSpace(input.ProjectDescription); description != "" {
		lines = append(lines, "Description: "+description)
	}
	lines = append(lines,
		"Codebase map:", strings.Join(mapLines, "\n"),
		"Scrum board:", strings.Join(boardLines, "\n"),
		"Task: identify evidence-backed bugs, cleanup, refactors, optimizations, reliability issues, security risks, and missing tests. Emit reviewable backlog cards.",
	)
	return debuggerSystemPrompt, strings.Join(lines, "\n"), nil
}

func Run(ctx context.Context, llm LLMClient, input Input) (ScanResponse, error) {
	if ctx == nil || llm == nil {
		return ScanResponse{}, fmt.Errorf("project debugger requires a context and LLM client")
	}
	modelName := strings.TrimSpace(input.Model)
	if modelName == "" {
		return ScanResponse{}, fmt.Errorf("project debugger model is required")
	}
	system, user, err := BuildPrompt(input)
	if err != nil {
		return ScanResponse{}, err
	}
	raw, err := llm.Generate(ctx, modelName, strings.TrimSpace(system+"\n\n"+user))
	if err != nil {
		return ScanResponse{}, err
	}
	return ParseScanResponse(raw)
}

func trimForPrompt(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
}
