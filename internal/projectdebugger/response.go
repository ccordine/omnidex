package projectdebugger

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

var scrumColumns = []string{"backlog", "ready", "assigned", "in_progress", "review", "blocked", "error", "done"}

var projectDebuggerTags = map[string]struct{}{
	"bug": {}, "cleanup": {}, "refactor": {}, "optimization": {},
	"reliability": {}, "security": {}, "test-gap": {}, "analysis": {},
}

func ParseScanResponse(raw string) (ScanResponse, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ScanResponse{}, fmt.Errorf("project debugger returned an empty response")
	}
	var response ScanResponse
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return ScanResponse{}, fmt.Errorf("decode project debugger response: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return ScanResponse{}, fmt.Errorf("project debugger response contains trailing JSON")
		}
		return ScanResponse{}, fmt.Errorf("project debugger response contains trailing data: %w", err)
	}
	response.Summary = strings.TrimSpace(response.Summary)
	if response.Summary == "" {
		return ScanResponse{}, fmt.Errorf("project debugger response summary is required")
	}
	if len(response.BugTickets) > 8 {
		return ScanResponse{}, fmt.Errorf("project debugger returned %d tickets; maximum is 8", len(response.BugTickets))
	}
	titles := map[string]struct{}{}
	for i := range response.BugTickets {
		if err := validateBugTicket(&response.BugTickets[i], titles); err != nil {
			return ScanResponse{}, fmt.Errorf("project debugger ticket %d: %w", i, err)
		}
	}
	for i := range response.Suggestions {
		response.Suggestions[i] = strings.TrimSpace(response.Suggestions[i])
		if response.Suggestions[i] == "" {
			return ScanResponse{}, fmt.Errorf("project debugger suggestion %d is empty", i)
		}
	}
	return response, nil
}

func validateBugTicket(ticket *BugTicket, titles map[string]struct{}) error {
	ticket.Title = strings.TrimSpace(ticket.Title)
	ticket.Description = strings.TrimSpace(ticket.Description)
	ticket.Severity = strings.ToLower(strings.TrimSpace(ticket.Severity))
	ticket.Column = strings.ToLower(strings.TrimSpace(ticket.Column))
	if ticket.Title == "" || ticket.Description == "" {
		return fmt.Errorf("title and description are required")
	}
	key := strings.ToLower(ticket.Title)
	if _, duplicate := titles[key]; duplicate {
		return fmt.Errorf("duplicate title %q", ticket.Title)
	}
	titles[key] = struct{}{}
	switch ticket.Severity {
	case "critical", "high", "medium", "low":
	default:
		return fmt.Errorf("unsupported severity %q", ticket.Severity)
	}
	if ticket.Column != "backlog" {
		return fmt.Errorf("column must be backlog, received %q", ticket.Column)
	}
	if len(ticket.Checklist) == 0 {
		return fmt.Errorf("at least one verification checklist item is required")
	}
	for i := range ticket.Checklist {
		ticket.Checklist[i] = strings.TrimSpace(ticket.Checklist[i])
		if ticket.Checklist[i] == "" {
			return fmt.Errorf("checklist item %d is empty", i)
		}
	}
	for i := range ticket.RefFiles {
		ticket.RefFiles[i] = strings.TrimSpace(ticket.RefFiles[i])
		if ticket.RefFiles[i] == "" {
			return fmt.Errorf("reference file %d is empty", i)
		}
	}
	seenTags := map[string]struct{}{}
	for i := range ticket.Tags {
		ticket.Tags[i] = strings.ToLower(strings.TrimSpace(ticket.Tags[i]))
		if _, allowed := projectDebuggerTags[ticket.Tags[i]]; !allowed || ticket.Tags[i] == "analysis" {
			return fmt.Errorf("unsupported tag %q", ticket.Tags[i])
		}
		if _, duplicate := seenTags[ticket.Tags[i]]; duplicate {
			return fmt.Errorf("duplicate tag %q", ticket.Tags[i])
		}
		seenTags[ticket.Tags[i]] = struct{}{}
	}
	ticket.Tags = append(ticket.Tags, "analysis")
	return nil
}

func validScrumColumn(column string) bool {
	for _, allowed := range scrumColumns {
		if column == allowed {
			return true
		}
	}
	return false
}

func CardPlanningPrompt(title, description string) (string, error) {
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	if title == "" {
		return "", fmt.Errorf("card planning prompt requires a title")
	}
	parts := []string{"Title: " + title}
	if description != "" {
		parts = append(parts, "Description:", description)
	}
	return strings.Join(parts, "\n"), nil
}

func FormatTicketDescription(ticket BugTicket) string {
	parts := []string{strings.TrimSpace(ticket.Description), "Severity: **" + strings.TrimSpace(ticket.Severity) + "**"}
	if len(ticket.RefFiles) > 0 {
		lines := make([]string, 0, len(ticket.RefFiles))
		for _, file := range ticket.RefFiles {
			lines = append(lines, "- `"+file+"`")
		}
		parts = append(parts, "Related files:\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(append(parts, "_Created by Analyze_"), "\n\n")
}

func ChecklistJSON(items []string) ([]byte, error) {
	type checklistItem struct {
		Text string `json:"text"`
		Done bool   `json:"done"`
	}
	out := make([]checklistItem, 0, len(items))
	for _, text := range items {
		text = strings.TrimSpace(text)
		if text == "" {
			return nil, fmt.Errorf("project debugger checklist contains an empty item")
		}
		out = append(out, checklistItem{Text: text})
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("encode project debugger checklist: %w", err)
	}
	return raw, nil
}

func TagsJSON(tags []string) ([]byte, error) {
	raw, err := json.Marshal(tags)
	if err != nil {
		return nil, fmt.Errorf("encode project debugger tags: %w", err)
	}
	return raw, nil
}

func RefFilesJSON(files []string) ([]byte, error) {
	raw, err := json.Marshal(files)
	if err != nil {
		return nil, fmt.Errorf("encode project debugger reference files: %w", err)
	}
	return raw, nil
}
