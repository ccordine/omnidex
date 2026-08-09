package workingset

import (
	"fmt"
	"math"
)

func (set *Set) ensureActive() error {
	if set == nil || set.status != StatusActive {
		return ErrSetClosed
	}
	return nil
}

func (set *Set) nextMutationTick() (uint64, error) {
	if set.clock != set.version {
		return 0, fmt.Errorf("%w: version and clock diverged", ErrInvalidSet)
	}
	if set.clock >= uint64(math.MaxInt64) {
		return 0, ErrClockOverflow
	}
	return set.clock + 1, nil
}

func (set *Set) commitMutation(tick uint64) {
	set.clock = tick
	set.version = tick
}

func (set *Set) membershipCount() int {
	count := 0
	for _, item := range set.items {
		count += len(item.Memberships)
	}
	return count
}
