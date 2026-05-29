package scrum

import (
	"encoding/json"
	"strings"
)

const AgentStatusFooter = `When you finish (or must stop), include exactly one status line in your final output:
SCRUM_STATUS: success|failed|blocked|in_progress

- success: work is complete and ready for human review
- failed: could not complete; explain what blocked you
- blocked: waiting on an external dependency or decision
- in_progress: meaningful partial progress; more work remains`

type ChecklistItem struct {
	ID   string `json:"id"`
	Text string `json:"text"`
	Done bool   `json:"done"`
}

type CardContext struct {
	ID           string
	Title        string
	Description  string
	JiraTicket   string
	Checklist    []ChecklistItem
	TestCriteria []ChecklistItem
	Tags         []string
	RefFiles     []string
	RecipeID     string
	RecipeJSON   string
}

func FormatChecklist(items []ChecklistItem) string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		state := "[ ]"
		if item.Done {
			state = "[x]"
		}
		lines = append(lines, state+" "+item.Text)
	}
	return strings.Join(lines, "\n")
}

func AppendCardContextLines(lines []string, card CardContext) []string {
	if strings.TrimSpace(card.Description) != "" {
		lines = append(lines, "Description:", card.Description)
	}
	if strings.TrimSpace(card.JiraTicket) != "" {
		lines = append(lines, "Jira ticket draft:", card.JiraTicket)
	}
	if checklist := FormatChecklist(card.Checklist); checklist != "" {
		lines = append(lines, "Checklist:", checklist)
	}
	if tests := FormatChecklist(card.TestCriteria); tests != "" {
		lines = append(lines, "Test criteria (must pass before done):", tests)
	}
	if len(card.Tags) > 0 {
		lines = append(lines, "Tags: "+strings.Join(card.Tags, ", "))
	}
	if len(card.RefFiles) > 0 {
		lines = append(lines, "Reference files:", strings.Join(card.RefFiles, "\n"))
	}
	if strings.TrimSpace(card.RecipeID) != "" {
		lines = append(lines, "Recipe ID: "+strings.TrimSpace(card.RecipeID))
	}
	if strings.TrimSpace(card.RecipeJSON) != "" {
		lines = append(lines, "Recipe JSON:", card.RecipeJSON)
	}
	return lines
}

func ContextLinesFromMetadata(raw json.RawMessage) []string {
	lines := []string{}
	if title := metadataString(raw, "scrum_card_title"); title != "" {
		lines = append(lines, "Scrum card: "+title)
	}
	if cardID := metadataString(raw, "scrum_card_id"); cardID != "" {
		lines = append(lines, "Card ID: "+cardID)
	}
	if dir := metadataString(raw, "project_directory"); dir != "" {
		lines = append(lines, "Project directory: "+dir)
	}
	if desc := metadataString(raw, "scrum_card_description"); desc != "" {
		lines = append(lines, "Description:", desc)
	}
	if ticket := metadataString(raw, "scrum_jira_ticket"); ticket != "" {
		lines = append(lines, "Jira ticket draft:", ticket)
	}
	if checklist := metadataString(raw, "scrum_checklist"); checklist != "" {
		lines = append(lines, "Checklist:", checklist)
	}
	if tests := metadataString(raw, "scrum_test_criteria"); tests != "" {
		lines = append(lines, "Test criteria (must pass before done):", tests)
	}
	if tags := metadataStringSlice(raw, "scrum_card_tags"); len(tags) > 0 {
		lines = append(lines, "Tags: "+strings.Join(tags, ", "))
	}
	if refs := metadataStringSlice(raw, "ref_files"); len(refs) > 0 {
		lines = append(lines, "Reference files:", strings.Join(refs, "\n"))
	}
	if recipeID := metadataString(raw, "recipe_id"); recipeID != "" {
		lines = append(lines, "Recipe ID: "+recipeID)
	}
	return lines
}

func metadataString(raw json.RawMessage, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		out, _ := json.Marshal(typed)
		return strings.TrimSpace(strings.Trim(string(out), `"`))
	}
}

func metadataStringSlice(raw json.RawMessage, key string) []string {
	if len(raw) == 0 {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, strings.TrimSpace(text))
			}
		}
		return out
	case []string:
		return typed
	default:
		return nil
	}
}

func IsScrumJob(raw json.RawMessage) bool {
	return metadataString(raw, "source") == "omni-scrum"
}

func IsStrictScrumExternal(raw json.RawMessage) bool {
	if !IsScrumJob(raw) {
		return false
	}
	if metadataString(raw, "agent_strict") == "true" {
		return true
	}
	agent := metadataString(raw, "execution_agent")
	return agent == "cursor" || agent == "codex"
}
