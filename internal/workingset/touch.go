package workingset

import (
	"fmt"
	"math"
	"sort"
)

func (set *Set) Touch(id ItemID) (Item, error) {
	items, err := set.TouchMany([]ItemID{id})
	if err != nil {
		return Item{}, err
	}
	return items[0], nil
}

func (set *Set) TouchMany(ids []ItemID) ([]Item, error) {
	if err := set.ensureActive(); err != nil {
		return nil, err
	}
	if len(ids) == 0 || len(ids) > MaxTouchBatchItems {
		return nil, fmt.Errorf("%w: touch batch must contain 1 to %d items", ErrInvalidItem, MaxTouchBatchItems)
	}
	ordered := append([]ItemID(nil), ids...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	items := make([]Item, len(ordered))
	for index, id := range ordered {
		if err := requireExactIdentity(string(id), "touched item ID", ErrInvalidItem); err != nil {
			return nil, err
		}
		if index > 0 && id == ordered[index-1] {
			return nil, fmt.Errorf("%w: touch batch repeats item %s", ErrDuplicateItem, id)
		}
		item, err := set.requireResident(id)
		if err != nil {
			return nil, err
		}
		if item.UseCount >= uint64(math.MaxInt64) {
			return nil, ErrClockOverflow
		}
		items[index] = item
	}
	tick, err := set.nextMutationTick()
	if err != nil {
		return nil, err
	}
	for index := range items {
		items[index].UseCount++
		items[index].LastUsedTick = tick
		set.items[items[index].ID] = items[index]
		items[index] = cloneItem(items[index])
	}
	set.commitMutation(tick)
	return items, nil
}
