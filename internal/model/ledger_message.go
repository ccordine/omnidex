package model

// LedgerPlanningMessage is a conversational edit of one item, retained separately
// from that item's executable instructions.
type LedgerPlanningMessage struct {
	ItemID LedgerItemID `json:"item_id"`
	Role   string       `json:"role"`
	Text   string       `json:"text"`
}
