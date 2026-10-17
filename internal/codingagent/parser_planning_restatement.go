package codingagent

import (
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

// A response can describe an edit and then quote the resulting wording. Code
// accounts for that exact restatement without asking for the same edit again.
// Only text changed by an earlier applied item can supply this evidence, and
// the complete text must still be present in that item's current active state.
func representedPlanningStatement(source string, current EngineeringLedger, applied []planningAppliedText) model.LedgerItemID {
	lines := strings.Split(strings.TrimSpace(source), "\n")
	for index, line := range lines {
		lines[index] = strings.TrimPrefix(line, "> ")
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	if text == "" {
		return ""
	}
	for _, change := range applied {
		retained := retainedPlanningItem(current, change.ItemID)
		if retained == nil || retained.Base().Disposition != model.LedgerDispositionActive {
			continue
		}
		if contains(planningItemStatements(retained), change.Text) && planningTextRepresentsStatement(change.Text, text) {
			return change.ItemID
		}
	}
	return ""
}

type planningAppliedText struct {
	ItemID model.LedgerItemID
	Text   string
}

func changedPlanningTexts(before EngineeringLedger, operation model.LedgerMutationOperation) []planningAppliedText {
	if operation.Item == nil || operation.Item.Base() == nil {
		return nil
	}
	id := operation.Item.Base().ID
	previous := []string{}
	if retained := retainedPlanningItem(before, id); retained != nil {
		previous = planningItemStatements(retained)
	}
	changes := []planningAppliedText{}
	for _, text := range planningItemStatements(operation.Item) {
		if strings.TrimSpace(text) != "" && !contains(previous, text) {
			changes = append(changes, planningAppliedText{ItemID: id, Text: text})
		}
	}
	return changes
}

// A scan may keep adjacent sentences together or classify them separately.
// Match only complete code-selected sentence spans, never arbitrary substrings.
func planningTextRepresentsStatement(value, text string) bool {
	if strings.TrimSpace(value) == text {
		return true
	}
	starts, ends := map[int]bool{}, map[int]bool{}
	offset := 0
	for _, fragment := range parserSentenceFragments(value) {
		trimmed := strings.TrimSpace(fragment)
		if trimmed != "" {
			start := offset + strings.Index(fragment, trimmed)
			starts[start], ends[start+len(trimmed)] = true, true
		}
		offset += len(fragment)
	}
	for offset = 0; offset < len(value); {
		index := strings.Index(value[offset:], text)
		if index < 0 {
			return false
		}
		start := offset + index
		if starts[start] && ends[start+len(text)] {
			return true
		}
		offset = start + 1
	}
	return false
}

func planningItemStatements(item *model.LedgerMutationItem) []string {
	switch {
	case item.Objective != nil:
		return []string{item.Objective.Outcome}
	case item.Requirement != nil:
		return []string{item.Requirement.Outcome}
	case item.Constraint != nil:
		return []string{item.Constraint.Rule}
	case item.Group != nil:
		return []string{item.Group.Title}
	case item.Task != nil:
		return []string{item.Task.Goal, item.Task.DeliveryObligation}
	case item.AcceptanceCriterion != nil:
		return []string{item.AcceptanceCriterion.Outcome}
	case item.Check != nil:
		return []string{item.Check.Observation}
	default:
		return nil
	}
}
