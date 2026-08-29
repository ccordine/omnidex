package api

import "sort"

func sortCardsForColumn(column string, cards []ScrumCard) {
	switch column {
	case "assigned":
		sort.SliceStable(cards, func(i, j int) bool {
			aQueued := cards[i].PlayState == scrumPlayQueued
			bQueued := cards[j].PlayState == scrumPlayQueued
			if aQueued != bQueued {
				return !aQueued
			}
			if aQueued && bQueued {
				if cards[i].QueueOrder != cards[j].QueueOrder {
					return cards[i].QueueOrder < cards[j].QueueOrder
				}
				return cards[i].BoardOrder < cards[j].BoardOrder
			}
			if cards[i].BoardOrder != cards[j].BoardOrder {
				return cards[i].BoardOrder < cards[j].BoardOrder
			}
			return cards[i].UpdatedAt > cards[j].UpdatedAt
		})
	case "in_progress":
		sort.SliceStable(cards, func(i, j int) bool {
			if cards[i].PlayState == scrumPlayRunning {
				return true
			}
			if cards[j].PlayState == scrumPlayRunning {
				return false
			}
			if cards[i].BoardOrder != cards[j].BoardOrder {
				return cards[i].BoardOrder < cards[j].BoardOrder
			}
			return cards[i].UpdatedAt > cards[j].UpdatedAt
		})
	default:
		sort.SliceStable(cards, func(i, j int) bool {
			if cards[i].BoardOrder != cards[j].BoardOrder {
				return cards[i].BoardOrder < cards[j].BoardOrder
			}
			return cards[i].UpdatedAt > cards[j].UpdatedAt
		})
	}
}
