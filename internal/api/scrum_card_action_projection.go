package api

import (
	"fmt"
	"strings"
)

// scrumCardActionProjection removes growing operational histories from the
// authoritative card returned by a completed server mutation. Channel POST
// replay uses its separate immutable projection because it must return a
// bounded tail of the operation result.
func scrumCardActionProjection(card ScrumCard) (ScrumCard, error) {
	if strings.TrimSpace(card.ID) == "" {
		return ScrumCard{}, fmt.Errorf("Scrum card action response requires an authoritative card ID")
	}
	if card.Summary {
		return ScrumCard{}, fmt.Errorf("Scrum card action response requires a mutation result, not a board summary")
	}
	card.Chat = []ScrumChatMessage{}
	card.PendingChannelMessages = nil
	return card, nil
}
