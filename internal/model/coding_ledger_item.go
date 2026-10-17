package model

func (item LedgerMutationItem) Base() *LedgerItem {
	if item.Task != nil {
		return &item.Task.LedgerItem
	}
	return nil
}
