package scrumcardllm

import (
	"fmt"
	"strings"
)

const (
	ticketCardDescMaxChars       = 800
	ticketChecklistMaxItems      = 20
	ticketChecklistItemMax       = 500
	ticketTestCriteriaMaxItems   = 20
	ticketTestCriteriaItemMax    = 500
	ticketRefFilesMax            = 10
	ticketRefFileMax             = 200
	ticketAuthorPromptMax        = 800
	ticketExistingDraftMax       = 6000
	ticketUserPromptMaxTotal     = 12000
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
	systemLines := []string{
		"You are a technical project manager drafting work tickets.",
		"Return markdown with sections: Summary, Description, Acceptance Criteria (checklist), Test Criteria, Technical Notes.",
		"Test Criteria should list verifiable tests the implementer must satisfy.",
		"Be concise and actionable. Do not wrap the response in code fences.",
	}
	if req.PlanningMode {
		systemLines = append(systemLines,
			"This is planning mode: produce a draft the user will review, coach, and refine before moving the card to ready/assigned or pressing play.",
			"Ground the plan in the card title and description; call out risks, verification steps, and scope boundaries.",
		)
	}
	system = strings.Join(systemLines, "\n")

	if req.Iterate {
		notes := strings.TrimSpace(req.IterateNotes)
		if notes == "" {
			notes = strings.TrimSpace(req.Prompt)
		}
		current := strings.TrimSpace(req.Ticket)
		if current == "" {
			current = strings.TrimSpace(card.CardTicket)
		}
		current = trimTicketText(current, ticketExistingDraftMax)
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
		"Scrum card: " + trimTicketText(card.Title, 200),
		"Column: " + trimTicketText(card.Column, 80),
		"Project directory: " + trimTicketText(board.ProjectDirectory, 260),
		"Description: " + trimTicketText(card.Description, ticketCardDescMaxChars),
		"Reference files: " + strings.Join(trimTicketRefFiles(card.RefFiles), ", "),
	}
	checklistCount := 0
	for _, item := range card.Checklist {
		if checklistCount >= ticketChecklistMaxItems {
			break
		}
		text := trimTicketText(item.Text, ticketChecklistItemMax)
		if text == "" {
			continue
		}
		state := "[ ]"
		if item.Done {
			state = "[x]"
		}
		contextLines = append(contextLines, fmt.Sprintf("%s %s", state, text))
		checklistCount++
	}
	testCount := 0
	for _, item := range card.TestCriteria {
		if testCount >= ticketTestCriteriaMaxItems {
			break
		}
		text := trimTicketText(item.Text, ticketTestCriteriaItemMax)
		if text == "" {
			continue
		}
		contextLines = append(contextLines, "Test: "+text)
		testCount++
	}
	if len(card.Tags) > 0 {
		contextLines = append(contextLines, "Tags: "+strings.Join(card.Tags, ", "))
	}
	contextLines = append(contextLines, "Author prompt: "+trimTicketText(prompt, ticketAuthorPromptMax))
	user = trimTicketPrompt(strings.Join(contextLines, "\n"), ticketUserPromptMaxTotal)
	return system, user
}

func trimTicketText(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "…"
}

func trimTicketRefFiles(files []string) []string {
	if len(files) == 0 {
		return nil
	}
	out := make([]string, 0, len(files))
	for _, file := range files {
		if len(out) >= ticketRefFilesMax {
			break
		}
		trimmed := trimTicketText(file, ticketRefFileMax)
		if trimmed == "" {
			continue
		}
		out = append(out, trimmed)
	}
	return out
}

func trimTicketPrompt(text string, max int) string {
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
		return trimTicketText(text, max)
	}
	return strings.TrimSpace(text[:head]) + "\n…context trimmed…\n" + strings.TrimSpace(text[len(text)-tail:])
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
