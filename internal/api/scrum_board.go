package api

import (
	"net/http"
	"strings"
)

func normalizeScrumColumn(column string) string {
	column = strings.ToLower(strings.TrimSpace(column))
	column = strings.ReplaceAll(column, " ", "_")
	column = strings.ReplaceAll(column, "-", "_")
	for _, allowed := range scrumColumns {
		if column == allowed {
			return allowed
		}
	}
	return ""
}

func nextPlayColumn(current string) string {
	switch normalizeScrumColumn(current) {
	case "ready":
		return "assigned"
	case "assigned", "in_progress":
		return "in_progress"
	case "review":
		return "review"
	default:
		return ""
	}
}

func buildScrumPlayInstruction(board ScrumBoard, card ScrumCard) string {
	lines := []string{"Scrum task execution for card: " + card.Title}
	if strings.TrimSpace(board.ProjectDirectory) != "" {
		lines = append(lines, "Project directory: "+board.ProjectDirectory)
	}
	lines = appendScrumCardContextLines(lines, card)
	lines = append(lines, "Execute with the server-resolved agent configuration. Omnidex owns completion from typed job and verification state.")
	return strings.Join(lines, "\n\n")
}

func cardsByColumn(board ScrumBoard) map[string][]ScrumCard {
	out := make(map[string][]ScrumCard, len(board.Columns))
	for _, col := range board.Columns {
		out[col] = []ScrumCard{}
	}
	for _, card := range board.Cards {
		col := normalizeScrumColumn(card.Column)
		if col == "" {
			col = "backlog"
		}
		out[col] = append(out[col], card)
	}
	for col := range out {
		sortCardsForColumn(col, out[col])
	}
	return out
}

func scrumViewportColumn(r *http.Request, columns []string) string {
	raw := ""
	if r != nil && r.URL != nil {
		raw = r.URL.Query().Get("column")
	}
	column := normalizeScrumColumn(raw)
	if column == "" {
		column = "assigned"
	}
	for _, candidate := range columns {
		if normalizeScrumColumn(candidate) == column {
			return column
		}
	}
	if len(columns) > 0 {
		return normalizeScrumColumn(columns[0])
	}
	return ""
}

func scrumCardBoardSummary(card ScrumCard) ScrumCard {
	checklistDone := completedScrumItems(card.Checklist)
	testDone := completedScrumItems(card.TestCriteria)
	return ScrumCard{
		ID: card.ID, Title: card.Title, Description: card.Description, Column: card.Column,
		Summary: true, ChecklistDone: checklistDone, ChecklistTotal: len(card.Checklist),
		RefFileCount: len(card.RefFiles), ChatCount: len(card.Chat),
		PlanningChatCount: len(card.PlanningChat), TestCriteriaDone: testDone,
		TestCriteriaTotal: len(card.TestCriteria), HasCardTicket: strings.TrimSpace(card.CardTicket) != "",
		Tags: append([]string(nil), card.Tags...), FlowMetrics: card.FlowMetrics,
		JobID:     card.JobID,
		PlayState: card.PlayState, QueueOrder: card.QueueOrder, BoardOrder: card.BoardOrder,
		CreatedAt: card.CreatedAt, UpdatedAt: card.UpdatedAt,
		Checklist: []ScrumChecklistItem{}, RefFiles: []string{}, Chat: []ScrumChatMessage{},
		PlanningChat: []ScrumChatMessage{}, TestCriteria: []ScrumChecklistItem{},
	}
}

func completedScrumItems(items []ScrumChecklistItem) int {
	count := 0
	for _, item := range items {
		if item.Done {
			count++
		}
	}
	return count
}

func scrumColumnCounts(cardsByCol map[string][]ScrumCard) map[string]int {
	counts := make(map[string]int, len(cardsByCol))
	for column, cards := range cardsByCol {
		counts[column] = len(cards)
	}
	return counts
}
