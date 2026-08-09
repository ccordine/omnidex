package workingset

import (
	"fmt"
	"sort"
)

func (set *Set) planEvictions(request AcquireRequest) ([]ItemID, error) {
	usage := set.Usage()
	if request.Retention == RetentionPinned {
		if usage.PinnedItems+1 > set.budget.MaxPinnedItems ||
			usage.PinnedBytes+request.ByteCost > set.budget.MaxPinnedBytes {
			return nil, fmt.Errorf("%w: acquiring %s would exceed %d items or %d bytes",
				ErrPinnedBudgetExceeded, request.ID, set.budget.MaxPinnedItems, set.budget.MaxPinnedBytes)
		}
	}
	usage.ResidentItems++
	usage.ResidentBytes += request.ByteCost
	if fitsResidentBudget(usage, set.budget) {
		return nil, nil
	}

	candidates := make([]Item, 0)
	for _, item := range set.items {
		if item.State == ItemResident && item.Retention == request.Retention && item.Retention != RetentionPinned {
			candidates = append(candidates, item)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].LastUsedTick != candidates[j].LastUsedTick {
			return candidates[i].LastUsedTick < candidates[j].LastUsedTick
		}
		if candidates[i].CreatedTick != candidates[j].CreatedTick {
			return candidates[i].CreatedTick < candidates[j].CreatedTick
		}
		return candidates[i].ID < candidates[j].ID
	})

	victims := make([]ItemID, 0)
	for _, candidate := range candidates {
		victims = append(victims, candidate.ID)
		usage.ResidentItems--
		usage.ResidentBytes -= candidate.ByteCost
		if fitsResidentBudget(usage, set.budget) {
			return victims, nil
		}
	}
	return nil, fmt.Errorf("%w: %s retention cannot free enough same-class capacity", ErrBudgetExceeded, request.Retention)
}

func (set *Set) validatePinPromotion(item Item) error {
	if item.Retention == RetentionPinned {
		return nil
	}
	usage := set.Usage()
	if usage.PinnedItems+1 > set.budget.MaxPinnedItems ||
		usage.PinnedBytes+item.ByteCost > set.budget.MaxPinnedBytes {
		return fmt.Errorf("%w: retaining %s as pinned would exceed %d items or %d bytes",
			ErrPinnedBudgetExceeded, item.ID, set.budget.MaxPinnedItems, set.budget.MaxPinnedBytes)
	}
	return nil
}

func fitsResidentBudget(usage Usage, budget Budget) bool {
	return usage.ResidentItems <= budget.MaxItems && usage.ResidentBytes <= budget.MaxBytes
}
