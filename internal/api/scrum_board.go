package api

import (
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
)

func normalizeScrumColumn(column string) string {
	typed, err := queue.ParseScrumCardColumn(column)
	if err != nil {
		return ""
	}
	return string(typed)
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

func cardsByColumn(board ScrumBoard) (map[string][]ScrumCard, error) {
	out := make(map[string][]ScrumCard, len(board.Columns))
	for _, col := range board.Columns {
		canonical := normalizeScrumColumn(col)
		if canonical == "" || canonical != col {
			return nil, fmt.Errorf("Scrum board contains noncanonical column %q", col)
		}
		out[canonical] = []ScrumCard{}
	}
	for _, card := range board.Cards {
		col := normalizeScrumColumn(card.Column)
		if col == "" || col != card.Column {
			return nil, fmt.Errorf("Scrum card %q contains noncanonical column %q", card.ID, card.Column)
		}
		if _, exists := out[col]; !exists {
			return nil, fmt.Errorf("Scrum card %q column %q is absent from board inventory", card.ID, col)
		}
		out[col] = append(out[col], card)
	}
	for col := range out {
		sortCardsForColumn(col, out[col])
	}
	return out, nil
}

func scrumColumnCounts(cardsByCol map[string][]ScrumCard) map[string]int {
	counts := make(map[string]int, len(cardsByCol))
	for column, cards := range cardsByCol {
		counts[column] = len(cards)
	}
	return counts
}
