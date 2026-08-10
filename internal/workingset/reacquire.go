package workingset

import "fmt"

func (set *Set) Reacquire(request ReacquireRequest) (ReacquireResult, error) {
	if err := set.ensureActive(); err != nil {
		return ReacquireResult{}, err
	}
	if err := validateReacquire(request); err != nil {
		return ReacquireResult{}, err
	}
	if err := set.validateScopeOwnership(request.Scope); err != nil {
		return ReacquireResult{}, err
	}
	if set.ScopeClosed(request.Scope) {
		return ReacquireResult{}, fmt.Errorf("%w: %s %s", ErrScopeClosed, request.Scope.Kind, request.Scope.ID)
	}
	item, exists := set.items[request.ItemID]
	if !exists {
		return ReacquireResult{}, fmt.Errorf("%w: %s", ErrItemNotFound, request.ItemID)
	}
	switch item.State {
	case ItemInvalidated:
		return ReacquireResult{}, fmt.Errorf("%w: %s", ErrItemInvalidated, request.ItemID)
	case ItemResident:
		return ReacquireResult{}, fmt.Errorf("%w: %s is resident", ErrItemNotReleased, request.ItemID)
	case ItemReleased:
	default:
		return ReacquireResult{}, fmt.Errorf("%w: item %s has state %q", ErrInvalidSet, request.ItemID, item.State)
	}
	if item.Ref != request.Ref || item.ReacquisitionCount != request.ExpectedReacquisitionCount {
		return ReacquireResult{}, fmt.Errorf(
			"%w: item %s does not match the exact immutable reference and count",
			ErrReacquisitionConflict, request.ItemID,
		)
	}
	if set.membershipCount() >= MaxMemberships {
		return ReacquireResult{}, fmt.Errorf("%w: membership limit is %d", ErrCapacityExceeded, MaxMemberships)
	}
	victims, err := set.planEvictions(admissionRequest{
		ID: item.ID, Retention: request.Retention, ByteCost: item.ByteCost,
	})
	if err != nil {
		return ReacquireResult{}, err
	}
	tick, err := set.nextMutationTick()
	if err != nil {
		return ReacquireResult{}, err
	}
	evicted := make([]Item, 0, len(victims))
	for _, id := range victims {
		victim := set.items[id]
		set.releaseItem(&victim, tick, budgetEvictionReason)
		set.items[id] = victim
		evicted = append(evicted, cloneItem(victim))
	}
	item.State = ItemResident
	item.Retention = request.Retention
	item.Memberships = []Membership{{Scope: request.Scope, Retention: request.Retention}}
	item.LastUsedTick = tick
	item.ReleasedTick = 0
	item.DispositionReason = ""
	item.ReacquisitionCount++
	set.items[item.ID] = item
	set.commitMutation(tick)
	return ReacquireResult{Item: cloneItem(item), Evicted: evicted}, nil
}
