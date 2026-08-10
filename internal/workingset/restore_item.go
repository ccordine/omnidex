package workingset

import (
	"fmt"
	"math"
)

func validateSnapshotItem(set *Set, item Item) error {
	if err := requireExactIdentity(string(item.ID), "item ID", ErrInvalidItem); err != nil {
		return err
	}
	if err := validateReference(item.Ref); err != nil {
		return err
	}
	if err := validateRole(item.Role); err != nil {
		return err
	}
	if retentionRank(item.Retention) == 0 {
		return fmt.Errorf("%w: retention %q is not registered", ErrInvalidRetention, item.Retention)
	}
	if item.Priority < 1 || item.Priority > 100 || item.ByteCost <= 0 || item.ByteCost > MaxResidentBytes {
		return fmt.Errorf("%w: priority or byte cost is outside its bounded domain", ErrInvalidItem)
	}
	if err := validateAcquisition(item.Acquisition); err != nil {
		return err
	}
	if item.CreatedTick == 0 || item.CreatedTick > item.LastUsedTick || item.LastUsedTick > set.clock ||
		item.UseCount > uint64(math.MaxInt64) || item.ReacquisitionCount > uint64(math.MaxInt64) {
		return fmt.Errorf("%w: creation, usage, or clock values are inconsistent", ErrInvalidItem)
	}
	if item.ReacquisitionCount > (item.LastUsedTick-item.CreatedTick)/2 {
		return fmt.Errorf("%w: reacquisition count exceeds the recorded lifecycle", ErrInvalidItem)
	}
	switch item.State {
	case ItemResident:
		return validateResidentItem(set, item)
	case ItemReleased, ItemInvalidated:
		if len(item.Memberships) != 0 || item.ReleasedTick <= item.LastUsedTick || item.ReleasedTick > set.clock {
			return fmt.Errorf("%w: terminal item lifecycle is inconsistent", ErrInvalidItem)
		}
		return requireExact(item.DispositionReason, "item disposition reason", ErrInvalidItem)
	default:
		return fmt.Errorf("%w: state %q is not registered", ErrInvalidItem, item.State)
	}
}

func validateResidentItem(set *Set, item Item) error {
	if item.ReleasedTick != 0 || item.DispositionReason != "" || len(item.Memberships) == 0 {
		return fmt.Errorf("%w: resident item lifecycle is inconsistent", ErrInvalidItem)
	}
	seenScopes := make(map[string]struct{}, len(item.Memberships))
	for index, membership := range item.Memberships {
		if err := validateMembership(membership.Scope, membership.Retention); err != nil {
			return err
		}
		if err := set.validateScopeOwnership(membership.Scope); err != nil {
			return err
		}
		if index > 0 && !membershipLess(item.Memberships[index-1], membership) {
			return fmt.Errorf("%w: memberships must be uniquely sorted", ErrInvalidItem)
		}
		key := scopeKey(membership.Scope)
		if _, exists := seenScopes[key]; exists {
			return fmt.Errorf("%w: item repeats one scope membership", ErrInvalidItem)
		}
		seenScopes[key] = struct{}{}
		if set.ScopeClosed(membership.Scope) && membership.Retention != RetentionPinned {
			return fmt.Errorf("%w: resident non-pinned membership uses a closed scope", ErrInvalidItem)
		}
	}
	if effectiveRetention(item.Memberships) != item.Retention {
		return fmt.Errorf("%w: effective retention does not match memberships", ErrInvalidRetention)
	}
	return nil
}

func membershipLess(left, right Membership) bool {
	leftRank, rightRank := retentionRank(left.Retention), retentionRank(right.Retention)
	if leftRank != rightRank {
		return leftRank < rightRank
	}
	if left.Scope.Kind != right.Scope.Kind {
		return left.Scope.Kind < right.Scope.Kind
	}
	return left.Scope.ID < right.Scope.ID
}

func scopeLess(left, right Scope) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return left.ID < right.ID
}
