package workingset

import "sort"

func New(owner Owner, budget Budget) (*Set, error) {
	id, err := NewSetID(owner)
	if err != nil {
		return nil, err
	}
	if err := validateBudget(budget); err != nil {
		return nil, err
	}
	return &Set{
		id: id, owner: owner, scope: rootScope(owner), budget: budget, status: StatusActive,
		items: make(map[ItemID]Item), refs: make(map[string]ItemID), closedScopes: make(map[string]Scope),
		commandEvents: make(map[CommandID]Event),
	}, nil
}

func (set *Set) ID() SetID       { return set.id }
func (set *Set) Owner() Owner    { return set.owner }
func (set *Set) Scope() Scope    { return set.scope }
func (set *Set) Budget() Budget  { return set.budget }
func (set *Set) Status() Status  { return set.status }
func (set *Set) Version() uint64 { return set.version }

func (set *Set) Item(id ItemID) (Item, bool) {
	item, exists := set.items[id]
	return cloneItem(item), exists
}

func (set *Set) Items() []Item {
	ids := set.sortedItemIDs()
	items := make([]Item, 0, len(ids))
	for _, id := range ids {
		items = append(items, cloneItem(set.items[id]))
	}
	return items
}

func (set *Set) ResidentItems() []Item {
	items := make([]Item, 0)
	for _, id := range set.sortedItemIDs() {
		item := set.items[id]
		if item.State == ItemResident {
			items = append(items, cloneItem(item))
		}
	}
	return items
}

func (set *Set) Usage() Usage {
	var usage Usage
	for _, item := range set.items {
		if item.State != ItemResident {
			continue
		}
		usage.ResidentItems++
		usage.ResidentBytes += item.ByteCost
		if item.Retention == RetentionPinned {
			usage.PinnedItems++
			usage.PinnedBytes += item.ByteCost
		}
	}
	return usage
}

func (set *Set) Snapshot() Snapshot {
	closed := make([]Scope, 0, len(set.closedScopes))
	for _, scope := range set.closedScopes {
		closed = append(closed, scope)
	}
	sortScopes(closed)
	return Snapshot{
		Schema: WorkingSetSchemaV1, ID: set.id, Owner: set.owner, Scope: set.scope,
		Budget: set.budget, Status: set.status, Version: set.version, Clock: set.clock,
		Items: set.Items(), ClosedScopes: closed,
		ClosedTick: set.closedTick, CloseReason: set.closeReason,
	}
}

func (set *Set) ScopeClosed(scope Scope) bool {
	_, closed := set.closedScopes[scopeKey(scope)]
	return closed
}

func (set *Set) sortedItemIDs() []ItemID {
	ids := make([]ItemID, 0, len(set.items))
	for id := range set.items {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func cloneItem(item Item) Item {
	item.Memberships = append([]Membership(nil), item.Memberships...)
	return item
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	clone := snapshot
	clone.Items = make([]Item, len(snapshot.Items))
	for index, item := range snapshot.Items {
		clone.Items[index] = cloneItem(item)
	}
	clone.ClosedScopes = make([]Scope, len(snapshot.ClosedScopes))
	copy(clone.ClosedScopes, snapshot.ClosedScopes)
	return clone
}

func sortScopes(scopes []Scope) {
	sort.Slice(scopes, func(i, j int) bool {
		if scopes[i].Kind != scopes[j].Kind {
			return scopes[i].Kind < scopes[j].Kind
		}
		return scopes[i].ID < scopes[j].ID
	})
}
