package workingset

import (
	"fmt"
	"sort"
)

func (set *Set) Retain(id ItemID, scope Scope, retention Retention) (Item, error) {
	if err := set.ensureActive(); err != nil {
		return Item{}, err
	}
	if err := validateMembership(scope, retention); err != nil {
		return Item{}, err
	}
	if err := set.validateScopeOwnership(scope); err != nil {
		return Item{}, err
	}
	if set.ScopeClosed(scope) {
		return Item{}, fmt.Errorf("%w: %s %s", ErrScopeClosed, scope.Kind, scope.ID)
	}
	item, err := set.requireResident(id)
	if err != nil {
		return Item{}, err
	}
	item = cloneItem(item)
	index := membershipIndex(item.Memberships, scope)
	if index >= 0 {
		current := item.Memberships[index].Retention
		if current == retention {
			return Item{}, fmt.Errorf("%w: item %s already belongs to %s %s", ErrMembershipExists, id, scope.Kind, scope.ID)
		}
		if retention != RetentionPinned {
			return Item{}, fmt.Errorf("%w: existing membership can only be promoted to pinned", ErrInvalidRetention)
		}
		if err := set.validatePinPromotion(item); err != nil {
			return Item{}, err
		}
		item.Memberships[index].Retention = RetentionPinned
	} else {
		if set.membershipCount() >= MaxMemberships {
			return Item{}, fmt.Errorf("%w: membership limit is %d", ErrCapacityExceeded, MaxMemberships)
		}
		if retention == RetentionPinned {
			if err := set.validatePinPromotion(item); err != nil {
				return Item{}, err
			}
		}
		item.Memberships = append(item.Memberships, Membership{Scope: scope, Retention: retention})
	}
	item.Retention = effectiveRetention(item.Memberships)
	sortMemberships(item.Memberships)
	tick, err := set.nextMutationTick()
	if err != nil {
		return Item{}, err
	}
	set.items[id] = item
	set.commitMutation(tick)
	return cloneItem(item), nil
}

func (set *Set) Release(id ItemID, scope Scope, reason string) (Item, error) {
	if err := set.ensureActive(); err != nil {
		return Item{}, err
	}
	if err := set.validateScopeOwnership(scope); err != nil {
		return Item{}, err
	}
	if err := requireExact(reason, "release reason", ErrInvalidItem); err != nil {
		return Item{}, err
	}
	item, err := set.requireResident(id)
	if err != nil {
		return Item{}, err
	}
	index := membershipIndex(item.Memberships, scope)
	if index < 0 {
		return Item{}, fmt.Errorf("%w: item %s does not belong to %s %s", ErrMembershipNotFound, id, scope.Kind, scope.ID)
	}
	tick, err := set.nextMutationTick()
	if err != nil {
		return Item{}, err
	}
	item.Memberships = append(item.Memberships[:index], item.Memberships[index+1:]...)
	if len(item.Memberships) == 0 {
		set.releaseItem(&item, tick, reason)
	} else {
		item.Retention = effectiveRetention(item.Memberships)
	}
	set.items[id] = item
	set.commitMutation(tick)
	return cloneItem(item), nil
}

func (set *Set) CloseScope(scope Scope, reason string) (ScopeCloseResult, error) {
	if err := set.ensureActive(); err != nil {
		return ScopeCloseResult{}, err
	}
	if err := set.validateScopeOwnership(scope); err != nil {
		return ScopeCloseResult{}, err
	}
	if err := requireExact(reason, "scope-close reason", ErrInvalidScope); err != nil {
		return ScopeCloseResult{}, err
	}
	key := scopeKey(scope)
	if _, exists := set.closedScopes[key]; exists {
		return ScopeCloseResult{}, fmt.Errorf("%w: %s %s", ErrScopeClosed, scope.Kind, scope.ID)
	}
	if len(set.closedScopes) >= MaxClosedScopes {
		return ScopeCloseResult{}, fmt.Errorf("%w: closed-scope limit is %d", ErrCapacityExceeded, MaxClosedScopes)
	}
	tick, err := set.nextMutationTick()
	if err != nil {
		return ScopeCloseResult{}, err
	}
	rootClose := scope == set.scope
	result := ScopeCloseResult{}
	for _, id := range set.sortedItemIDs() {
		item := set.items[id]
		if item.State != ItemResident {
			continue
		}
		index := membershipIndex(item.Memberships, scope)
		if rootClose {
			item.Memberships = nil
		} else {
			if index < 0 || item.Memberships[index].Retention == RetentionPinned {
				continue
			}
			item.Memberships = append(item.Memberships[:index], item.Memberships[index+1:]...)
		}
		if len(item.Memberships) == 0 {
			set.releaseItem(&item, tick, reason)
			result.Released = append(result.Released, cloneItem(item))
		} else {
			item.Retention = effectiveRetention(item.Memberships)
			result.Updated = append(result.Updated, cloneItem(item))
		}
		set.items[id] = item
	}
	set.closedScopes[key] = scope
	if rootClose {
		set.status = StatusClosed
		set.closedTick = tick
		set.closeReason = reason
	}
	set.commitMutation(tick)
	return result, nil
}

func (set *Set) InvalidateStale(id ItemID, currentVersion, currentHash, reason string) (Item, bool, error) {
	if err := set.ensureActive(); err != nil {
		return Item{}, false, err
	}
	item, err := set.requireResident(id)
	if err != nil {
		return Item{}, false, err
	}
	current := item.Ref
	current.Version = currentVersion
	current.Hash = currentHash
	if err := validateReference(current); err != nil {
		return Item{}, false, err
	}
	if err := requireExact(reason, "invalidation reason", ErrInvalidItem); err != nil {
		return Item{}, false, err
	}
	if item.Ref.Version == currentVersion && item.Ref.Hash == currentHash {
		return cloneItem(item), false, nil
	}
	tick, err := set.nextMutationTick()
	if err != nil {
		return Item{}, false, err
	}
	item.State = ItemInvalidated
	item.Memberships = nil
	item.ReleasedTick = tick
	item.DispositionReason = reason
	set.items[id] = item
	set.commitMutation(tick)
	return cloneItem(item), true, nil
}

func (set *Set) requireResident(id ItemID) (Item, error) {
	item, exists := set.items[id]
	if !exists {
		return Item{}, fmt.Errorf("%w: %s", ErrItemNotFound, id)
	}
	if item.State != ItemResident {
		return Item{}, fmt.Errorf("%w: %s is %s", ErrItemNotResident, id, item.State)
	}
	return item, nil
}

func (set *Set) releaseItem(item *Item, tick uint64, reason string) {
	item.State = ItemReleased
	item.Memberships = nil
	item.ReleasedTick = tick
	item.DispositionReason = reason
}

func membershipIndex(memberships []Membership, scope Scope) int {
	for index, membership := range memberships {
		if membership.Scope == scope {
			return index
		}
	}
	return -1
}

func effectiveRetention(memberships []Membership) Retention {
	result := memberships[0].Retention
	for _, membership := range memberships[1:] {
		if retentionRank(membership.Retention) > retentionRank(result) {
			result = membership.Retention
		}
	}
	return result
}

func retentionRank(retention Retention) int {
	switch retention {
	case RetentionCall:
		return 1
	case RetentionStep:
		return 2
	case RetentionPhase:
		return 3
	case RetentionTask:
		return 4
	case RetentionObjective:
		return 5
	case RetentionJob:
		return 6
	case RetentionPinned:
		return 7
	default:
		return 0
	}
}

func sortMemberships(memberships []Membership) {
	sort.Slice(memberships, func(i, j int) bool {
		if memberships[i].Retention != memberships[j].Retention {
			return retentionRank(memberships[i].Retention) < retentionRank(memberships[j].Retention)
		}
		if memberships[i].Scope.Kind != memberships[j].Scope.Kind {
			return memberships[i].Scope.Kind < memberships[j].Scope.Kind
		}
		return memberships[i].Scope.ID < memberships[j].Scope.ID
	})
}
