package api

import "strings"

func scrumCardTagsLLMPending(card ScrumCard) bool {
	return strings.TrimSpace(card.TagsJobID) != ""
}

func scrumCardTicketLLMPending(card ScrumCard) bool {
	return strings.TrimSpace(card.TicketJobID) != ""
}

func scrumCardAnyLLMPending(card ScrumCard) bool {
	return scrumCardTagsLLMPending(card) || scrumCardTicketLLMPending(card)
}
