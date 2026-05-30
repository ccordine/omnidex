package projectdebugger

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

type LLMClient interface {
	Generate(ctx context.Context, model, prompt string) (string, error)
}

const debuggerSystemPrompt = `You are the Omni project debugger — a code-quality analyst for a software project.
Your job is to scan project context (codebase map, board, risks, tests) and identify concrete bugs, defects, tech debt, and reliability problems.
You never execute code or modify files directly.
Focus on actionable findings: missing error handling, stale modules, test gaps, security risks, broken flows, inconsistent patterns.
Avoid duplicating existing open backlog items when their titles clearly match.
Respond with JSON only (no markdown fences):
{"summary":"brief scan overview","bug_tickets":[{"title":"...","description":"markdown details with file refs when known","severity":"critical|high|medium|low","column":"backlog","checklist":["verify step"],"ref_files":["path/to/file.go"],"tags":["bug","debugger"]}],"suggestions":["optional process tip"]}
Emit 3-8 bug_tickets when issues exist; emit fewer if the project looks healthy. Prefer backlog column.`

func MapContextLines(payload map[string]any) []string {
	if payload == nil {
		return []string{"(codebase map not available)"}
	}
	exists, _ := payload["exists"].(bool)
	if !exists {
		return []string{"(codebase map not scanned yet — infer from board and description only)"}
	}
	lines := []string{}
	if root, ok := payload["root"].(string); ok && strings.TrimSpace(root) != "" {
		lines = append(lines, "root: "+root)
	}
	if count, ok := payload["file_count"].(float64); ok {
		lines = append(lines, fmt.Sprintf("files: %d", int(count)))
	}
	if modules, ok := payload["modules"].([]any); ok {
		for i, raw := range modules {
			if i >= 8 {
				break
			}
			mod, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			path, _ := mod["path"].(string)
			purpose, _ := mod["purpose"].(string)
			if path == "" {
				continue
			}
			line := path
			if purpose != "" {
				line += " — " + trimForPrompt(purpose, 100)
			}
			lines = append(lines, line)
		}
	}
	if risks, ok := payload["risks"].([]any); ok && len(risks) > 0 {
		lines = append(lines, "known risks:")
		for i, raw := range risks {
			if i >= 6 {
				break
			}
			risk, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			area, _ := risk["area"].(string)
			text, _ := risk["risk"].(string)
			if area != "" || text != "" {
				lines = append(lines, fmt.Sprintf("- %s: %s", area, trimForPrompt(text, 120)))
			}
		}
	}
	if tests, ok := payload["tests"].([]any); ok {
		lines = append(lines, fmt.Sprintf("test files indexed: %d", len(tests)))
	}
	if open, ok := payload["open_questions"].([]any); ok && len(open) > 0 {
		lines = append(lines, "open questions:")
		for i, raw := range open {
			if i >= 4 {
				break
			}
			if q, ok := raw.(string); ok && strings.TrimSpace(q) != "" {
				lines = append(lines, "- "+trimForPrompt(q, 120))
			}
		}
	}
	if tree, ok := payload["tree_preview"].(string); ok && strings.TrimSpace(tree) != "" {
		lines = append(lines, "tree preview:", trimForPrompt(tree, 1800))
	}
	return lines
}

func BoardSummaryLines(cards []BoardCard) []string {
	if len(cards) == 0 {
		return []string{"(no scrum cards yet)"}
	}
	byColumn := map[string][]BoardCard{}
	for _, card := range cards {
		col := strings.TrimSpace(card.Column)
		if col == "" {
			col = "backlog"
		}
		byColumn[col] = append(byColumn[col], card)
	}
	out := make([]string, 0, len(cards)+4)
	for col, items := range byColumn {
		out = append(out, fmt.Sprintf("[%s] %d cards", col, len(items)))
		for _, card := range items {
			line := "- " + strings.TrimSpace(card.Title)
			if card.PlayState == "running" {
				line += " (running)"
			}
			if desc := strings.TrimSpace(card.Description); desc != "" {
				line += ": " + trimForPrompt(desc, 120)
			}
			if len(card.Tags) > 0 {
				line += " [" + strings.Join(card.Tags, ", ") + "]"
			}
			out = append(out, line)
		}
	}
	return out
}

func BuildPrompt(in Input) (system, user string) {
	lines := []string{
		"Project: " + in.ProjectName,
		"Directory: " + in.ProjectLocation,
		"State: " + strings.TrimSpace(in.ProjectState),
		"Execution agent: " + strings.TrimSpace(in.AgentSystem),
	}
	if desc := strings.TrimSpace(in.ProjectDescription); desc != "" {
		lines = append(lines, "Description: "+desc)
	}
	mapLines := MapContextLines(in.MapPayload)
	lines = append(lines, "Codebase map:", strings.Join(mapLines, "\n"))
	lines = append(lines, "Scrum board:", strings.Join(BoardSummaryLines(in.BoardCards), "\n"))
	lines = append(lines, "Task: scan for bugs, defects, reliability issues, and missing tests. Emit bug_tickets for the backlog.")
	return debuggerSystemPrompt, strings.Join(lines, "\n")
}

func ParseScanResponse(raw string) ScanResponse {
	raw = strings.TrimSpace(raw)
	out := ScanResponse{Summary: raw}
	if raw == "" {
		return out
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		var parsed ScanResponse
		if err := json.Unmarshal([]byte(raw[start:end+1]), &parsed); err == nil {
			out = normalizeScanResponse(parsed)
		}
	}
	return out
}

func normalizeScanResponse(in ScanResponse) ScanResponse {
	in.Summary = strings.TrimSpace(in.Summary)
	tickets := make([]BugTicket, 0, len(in.BugTickets))
	seen := map[string]bool{}
	for _, ticket := range in.BugTickets {
		ticket.Title = strings.TrimSpace(ticket.Title)
		if ticket.Title == "" || seen[strings.ToLower(ticket.Title)] {
			continue
		}
		seen[strings.ToLower(ticket.Title)] = true
		ticket.Description = strings.TrimSpace(ticket.Description)
		ticket.Severity = normalizeSeverity(ticket.Severity)
		ticket.Column = normalizeColumn(ticket.Column)
		ticket.Tags = mergeTags(ticket.Tags, []string{"bug", "debugger"})
		tickets = append(tickets, ticket)
	}
	in.BugTickets = tickets
	suggestions := make([]string, 0, len(in.Suggestions))
	for _, item := range in.Suggestions {
		item = strings.TrimSpace(item)
		if item != "" {
			suggestions = append(suggestions, item)
		}
	}
	in.Suggestions = suggestions
	return in
}

func Run(ctx context.Context, llm LLMClient, in Input) (ScanResponse, error) {
	if llm == nil {
		return ScanResponse{}, fmt.Errorf("llm client is required")
	}
	system, user := BuildPrompt(in)
	modelName := strings.TrimSpace(in.Model)
	if modelName == "" {
		modelName = "qwen3:4b-thinking"
	}
	prompt := strings.TrimSpace(system + "\n\n" + user)
	raw, err := llm.Generate(ctx, modelName, prompt)
	if err != nil {
		return ScanResponse{}, err
	}
	return ParseScanResponse(raw), nil
}

func normalizeSeverity(severity string) string {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical", "high", "medium", "low":
		return strings.ToLower(strings.TrimSpace(severity))
	default:
		return "medium"
	}
}

func normalizeColumn(column string) string {
	column = strings.ToLower(strings.TrimSpace(column))
	if column == "" {
		return "backlog"
	}
	return column
}

func mergeTags(base, extra []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(base)+len(extra))
	for _, tag := range append(base, extra...) {
		tag = strings.TrimSpace(tag)
		if tag == "" || seen[strings.ToLower(tag)] {
			continue
		}
		seen[strings.ToLower(tag)] = true
		out = append(out, tag)
	}
	return out
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

func FormatTicketDescription(ticket BugTicket) string {
	parts := []string{}
	if desc := strings.TrimSpace(ticket.Description); desc != "" {
		parts = append(parts, desc)
	}
	if sev := strings.TrimSpace(ticket.Severity); sev != "" {
		parts = append(parts, "Severity: **"+sev+"**")
	}
	if len(ticket.RefFiles) > 0 {
		lines := make([]string, 0, len(ticket.RefFiles))
		for _, file := range ticket.RefFiles {
			file = strings.TrimSpace(file)
			if file != "" {
				lines = append(lines, "- `"+file+"`")
			}
		}
		if len(lines) > 0 {
			parts = append(parts, "Related files:\n"+strings.Join(lines, "\n"))
		}
	}
	parts = append(parts, "_Created by Run Debugger_")
	return strings.Join(parts, "\n\n")
}

func ChecklistJSON(items []string) []byte {
	type item struct {
		Text string `json:"text"`
		Done bool   `json:"done"`
	}
	out := make([]item, 0, len(items))
	for _, text := range items {
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		out = append(out, item{Text: text, Done: false})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return []byte("[]")
	}
	return raw
}

func TagsJSON(tags []string) []byte {
	raw, err := json.Marshal(tags)
	if err != nil {
		return []byte(`["bug","debugger"]`)
	}
	return raw
}

func RefFilesJSON(files []string) []byte {
	clean := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.TrimSpace(file)
		if file != "" {
			clean = append(clean, file)
		}
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return []byte("[]")
	}
	return raw
}
