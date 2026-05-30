package scrumcardllm

import (
	"fmt"
	"strings"
)

type ChecklistItem struct {
	Text string
	Done bool
}

type CardContext struct {
	ID           string
	Title        string
	Description  string
	Column       string
	RefFiles     []string
	Checklist    []ChecklistItem
	TestCriteria []ChecklistItem
	Tags         []string
	CardPrompt   string
	CardTicket   string
}

type BoardContext struct {
	Name              string
	ProjectDirectory  string
}

func TagsSuggestPrompts(board BoardContext, card CardContext, knownTags []string) (system, user string) {
	existingLine := "Known tags: " + strings.Join(knownTags, ", ")
	contextLines := []string{
		"Scrum card: " + card.Title,
		"Description: " + card.Description,
		"Project: " + board.Name,
		existingLine,
		"Current card tags: " + strings.Join(card.Tags, ", "),
	}
	for _, item := range card.TestCriteria {
		if strings.TrimSpace(item.Text) != "" {
			contextLines = append(contextLines, "Test: "+item.Text)
		}
	}
	system = strings.Join([]string{
		"You suggest concise lowercase tags for scrum cards and project memory.",
		"Tags should describe domain, tech stack, feature area, and work type.",
		"Respond with JSON only (no markdown fences):",
		`{"tags":["tag-one","tag-two"],"notes":"brief rationale"}`,
	}, "\n")
	return system, strings.Join(contextLines, "\n")
}

func CardTicketPrompts(board BoardContext, card CardContext, req TicketRequest) (system, user string) {
	system = strings.Join([]string{
		"You are a technical project manager drafting work tickets.",
		"Return markdown with sections: Summary, Description, Acceptance Criteria (checklist), Test Criteria, Technical Notes.",
		"Test Criteria should list verifiable tests the implementer must satisfy.",
		"Be concise and actionable. Do not wrap the response in code fences.",
	}, "\n")

	if req.Iterate {
		notes := strings.TrimSpace(req.IterateNotes)
		if notes == "" {
			notes = strings.TrimSpace(req.Prompt)
		}
		current := strings.TrimSpace(req.Ticket)
		if current == "" {
			current = strings.TrimSpace(card.CardTicket)
		}
		user = strings.Join([]string{
			"Refine this existing work ticket based on the user's notes.",
			"Keep the same markdown sections but improve clarity and completeness.",
			"",
			"Current ticket:",
			current,
			"",
			"Refinement notes:",
			firstNonEmpty(notes, "Tighten scope, acceptance criteria, and test criteria."),
		}, "\n")
		return system, user
	}

	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		prompt = strings.TrimSpace(req.CardPrompt)
	}
	if prompt == "" {
		prompt = strings.TrimSpace(card.CardPrompt)
	}
	if prompt == "" {
		prompt = "Draft a work ticket for this scrum card."
	}
	contextLines := []string{
		"Scrum card: " + card.Title,
		"Column: " + card.Column,
		"Project directory: " + board.ProjectDirectory,
		"Description: " + card.Description,
		"Reference files: " + strings.Join(card.RefFiles, ", "),
	}
	for _, item := range card.Checklist {
		state := "[ ]"
		if item.Done {
			state = "[x]"
		}
		contextLines = append(contextLines, fmt.Sprintf("%s %s", state, item.Text))
	}
	for _, item := range card.TestCriteria {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		contextLines = append(contextLines, "Test: "+item.Text)
	}
	if len(card.Tags) > 0 {
		contextLines = append(contextLines, "Tags: "+strings.Join(card.Tags, ", "))
	}
	contextLines = append(contextLines, "Author prompt: "+prompt)
	return system, strings.Join(contextLines, "\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
