package workingset

import "fmt"

const budgetEvictionReason = "Resident budget evicted the least-recently-used item in the same retention class."

func (set *Set) Acquire(request AcquireRequest) (AcquireResult, error) {
	if err := set.ensureActive(); err != nil {
		return AcquireResult{}, err
	}
	if err := validateAcquire(request); err != nil {
		return AcquireResult{}, err
	}
	if err := set.validateScopeOwnership(request.Scope); err != nil {
		return AcquireResult{}, err
	}
	if set.ScopeClosed(request.Scope) {
		return AcquireResult{}, fmt.Errorf("%w: %s %s", ErrScopeClosed, request.Scope.Kind, request.Scope.ID)
	}
	if _, exists := set.items[request.ID]; exists {
		return AcquireResult{}, fmt.Errorf("%w: item ID %s was already used", ErrDuplicateItem, request.ID)
	}
	if len(set.items) >= MaxHistoricalItems {
		return AcquireResult{}, fmt.Errorf("%w: historical item limit is %d", ErrCapacityExceeded, MaxHistoricalItems)
	}
	if set.membershipCount() >= MaxMemberships {
		return AcquireResult{}, fmt.Errorf("%w: membership limit is %d", ErrCapacityExceeded, MaxMemberships)
	}
	refKey := referenceKey(request.Ref)
	if existing, exists := set.refs[refKey]; exists {
		if set.items[existing].Ref.Hash != request.Ref.Hash {
			return AcquireResult{}, fmt.Errorf("%w: reference identity is resident as %s with a different hash", ErrReferenceConflict, existing)
		}
		return AcquireResult{}, fmt.Errorf("%w: reference is already resident as %s", ErrDuplicateReference, existing)
	}
	victims, err := set.planEvictions(admissionRequest{
		ID: request.ID, Retention: request.Retention, ByteCost: request.ByteCost,
	})
	if err != nil {
		return AcquireResult{}, err
	}

	tick, err := set.nextMutationTick()
	if err != nil {
		return AcquireResult{}, err
	}
	evicted := make([]Item, 0, len(victims))
	for _, id := range victims {
		item := set.items[id]
		item.State = ItemReleased
		item.Memberships = nil
		item.ReleasedTick = tick
		item.DispositionReason = budgetEvictionReason
		set.items[id] = item
		evicted = append(evicted, cloneItem(item))
	}
	item := Item{
		ID: request.ID, Ref: request.Ref, Role: request.Role, Retention: request.Retention,
		Priority: request.Priority, State: ItemResident, ByteCost: request.ByteCost,
		Acquisition: request.Acquisition,
		Memberships: []Membership{{Scope: request.Scope, Retention: request.Retention}},
		CreatedTick: tick, LastUsedTick: tick,
	}
	set.items[item.ID] = item
	set.refs[refKey] = item.ID
	set.commitMutation(tick)
	return AcquireResult{Item: cloneItem(item), Evicted: evicted}, nil
}
