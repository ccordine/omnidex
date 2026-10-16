package model

// Base exposes the common metadata of the selected typed item. Domain
// validation still requires exactly one variant before applying a mutation.
func (item LedgerMutationItem) Base() *LedgerItem {
	switch {
	case item.Objective != nil:
		return &item.Objective.LedgerItem
	case item.Requirement != nil:
		return &item.Requirement.LedgerItem
	case item.Constraint != nil:
		return &item.Constraint.LedgerItem
	case item.Group != nil:
		return &item.Group.LedgerItem
	case item.Task != nil:
		return &item.Task.LedgerItem
	case item.AcceptanceCriterion != nil:
		return &item.AcceptanceCriterion.LedgerItem
	case item.Check != nil:
		return &item.Check.LedgerItem
	case item.Dependency != nil:
		return &item.Dependency.LedgerItem
	default:
		return nil
	}
}
