package workingset

import (
	"fmt"
	"math"
)

func ValidateSnapshot(snapshot Snapshot) error {
	_, err := restoreSnapshot(snapshot)
	return err
}

func Restore(snapshot Snapshot) (*Set, error) {
	return restoreSnapshot(cloneSnapshot(snapshot))
}

func restoreSnapshot(snapshot Snapshot) (*Set, error) {
	if len(snapshot.Items) > MaxHistoricalItems {
		return nil, capacityError("historical items", MaxHistoricalItems)
	}
	if len(snapshot.ClosedScopes) > MaxClosedScopes {
		return nil, capacityError("closed scopes", MaxClosedScopes)
	}
	membershipCount := 0
	for _, item := range snapshot.Items {
		if len(item.Memberships) > MaxMemberships-membershipCount {
			return nil, capacityError("memberships", MaxMemberships)
		}
		membershipCount += len(item.Memberships)
	}
	if snapshot.Schema != WorkingSetSchemaV1 {
		return nil, fmt.Errorf("%w: unsupported snapshot schema %q", ErrInvalidSet, snapshot.Schema)
	}
	if err := validateOwner(snapshot.Owner); err != nil {
		return nil, err
	}
	if err := validateSetIdentity(snapshot.ID, snapshot.Owner); err != nil {
		return nil, err
	}
	if snapshot.Scope != rootScope(snapshot.Owner) {
		return nil, fmt.Errorf("%w: root scope does not match immutable owner", ErrInvalidSet)
	}
	if err := validateBudget(snapshot.Budget); err != nil {
		return nil, err
	}
	if snapshot.Version != snapshot.Clock || snapshot.Clock > uint64(math.MaxInt64) {
		return nil, fmt.Errorf("%w: version and clock must be one PostgreSQL BIGINT counter", ErrInvalidSet)
	}
	if snapshot.Items == nil || snapshot.ClosedScopes == nil {
		return nil, fmt.Errorf("%w: snapshot collections must be canonical arrays", ErrInvalidSet)
	}
	set := &Set{
		id: snapshot.ID, owner: snapshot.Owner, scope: snapshot.Scope, budget: snapshot.Budget,
		status: snapshot.Status, version: snapshot.Version, clock: snapshot.Clock,
		items: make(map[ItemID]Item, len(snapshot.Items)), refs: make(map[string]ItemID, len(snapshot.Items)),
		closedScopes:  make(map[string]Scope, len(snapshot.ClosedScopes)),
		commandEvents: make(map[CommandID]Event),
		closedTick:    snapshot.ClosedTick, closeReason: snapshot.CloseReason,
	}
	if err := restoreClosedScopes(set, snapshot.ClosedScopes); err != nil {
		return nil, err
	}
	if err := validateSnapshotStatus(set); err != nil {
		return nil, err
	}
	createdTicks := make(map[uint64]ItemID, len(snapshot.Items))
	for index, item := range snapshot.Items {
		if index > 0 && snapshot.Items[index-1].ID >= item.ID {
			return nil, fmt.Errorf("%w: snapshot items must be uniquely sorted by ID", ErrInvalidSet)
		}
		if err := validateSnapshotItem(set, item); err != nil {
			return nil, fmt.Errorf("item %q: %w", item.ID, err)
		}
		if existing, exists := createdTicks[item.CreatedTick]; exists {
			return nil, fmt.Errorf(
				"%w: items %s and %s share creation tick %d",
				ErrInvalidSet, existing, item.ID, item.CreatedTick,
			)
		}
		createdTicks[item.CreatedTick] = item.ID
		identity := referenceKey(item.Ref)
		if existing, exists := set.refs[identity]; exists {
			if set.items[existing].Ref.Hash != item.Ref.Hash {
				return nil, fmt.Errorf(
					"%w: items %s and %s bind one reference identity to different hashes",
					ErrReferenceConflict, existing, item.ID,
				)
			}
			return nil, fmt.Errorf("%w: items %s and %s repeat one reference", ErrDuplicateReference, existing, item.ID)
		}
		set.items[item.ID] = cloneItem(item)
		set.refs[identity] = item.ID
	}
	if snapshot.Clock < uint64(len(snapshot.Items)+len(snapshot.ClosedScopes)) {
		return nil, fmt.Errorf("%w: clock cannot precede retained lifecycle records", ErrInvalidSet)
	}
	if err := validateRestoredUsage(set); err != nil {
		return nil, err
	}
	emptyState := len(set.items) == 0 && len(set.closedScopes) == 0
	if (set.version == 0) != emptyState {
		return nil, fmt.Errorf("%w: zero version and empty lifecycle state must agree", ErrInvalidSet)
	}
	return set, nil
}

func restoreClosedScopes(set *Set, scopes []Scope) error {
	for index, scope := range scopes {
		if err := set.validateScopeOwnership(scope); err != nil {
			return err
		}
		if index > 0 && !scopeLess(scopes[index-1], scope) {
			return fmt.Errorf("%w: closed scopes must be uniquely sorted", ErrInvalidSet)
		}
		set.closedScopes[scopeKey(scope)] = scope
	}
	return nil
}

func validateSnapshotStatus(set *Set) error {
	rootClosed := set.ScopeClosed(set.scope)
	switch set.status {
	case StatusActive:
		if rootClosed || set.closedTick != 0 || set.closeReason != "" {
			return fmt.Errorf("%w: active set contains terminal root state", ErrInvalidSet)
		}
	case StatusClosed:
		if !rootClosed || set.closedTick == 0 || set.closedTick != set.clock {
			return fmt.Errorf("%w: closed set requires its root close at the final tick", ErrInvalidSet)
		}
		if err := requireExact(set.closeReason, "root close reason", ErrInvalidSet); err != nil {
			return err
		}
	default:
		return fmt.Errorf("%w: status %q is not registered", ErrInvalidSet, set.status)
	}
	return nil
}

func validateRestoredUsage(set *Set) error {
	usage := set.Usage()
	if !fitsResidentBudget(usage, set.budget) || usage.PinnedItems > set.budget.MaxPinnedItems ||
		usage.PinnedBytes > set.budget.MaxPinnedBytes {
		return fmt.Errorf("%w: restored resident usage exceeds its budget", ErrInvalidBudget)
	}
	if set.status == StatusClosed && usage.ResidentItems != 0 {
		return fmt.Errorf("%w: closed set retains resident items", ErrInvalidSet)
	}
	return nil
}

func capacityError(subject string, limit int) error {
	return fmt.Errorf("%w: %s exceed the %d-record limit", ErrCapacityExceeded, subject, limit)
}
