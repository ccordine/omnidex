package specialistworkflow

import (
	"fmt"
	"sync"
)

const maxAttemptLimit = 32

type AttemptNumber uint16

type AttemptBudget struct {
	lock     sync.Mutex
	maximum  uint16
	used     uint16
	reserved uint16
}

func NewAttemptBudget(maximum uint16) (*AttemptBudget, error) {
	if maximum == 0 || maximum > maxAttemptLimit {
		return nil, fmt.Errorf(
			"%w: maximum must be between 1 and %d", ErrInvalidAttemptBudget, maxAttemptLimit,
		)
	}
	return &AttemptBudget{maximum: maximum}, nil
}

func (budget *AttemptBudget) Claim() (AttemptNumber, error) {
	reservation, err := budget.reserve()
	if err != nil {
		return 0, err
	}
	return reservation.commit()
}

func (budget *AttemptBudget) Used() uint16 {
	if budget == nil {
		return 0
	}
	budget.lock.Lock()
	defer budget.lock.Unlock()
	return budget.used
}

func (budget *AttemptBudget) Maximum() uint16 {
	if budget == nil {
		return 0
	}
	budget.lock.Lock()
	defer budget.lock.Unlock()
	return budget.maximum
}

type attemptReservation struct {
	budget *AttemptBudget
	active bool
}

func (budget *AttemptBudget) reserve() (*attemptReservation, error) {
	if budget == nil {
		return nil, fmt.Errorf("%w: budget is nil", ErrInvalidAttemptBudget)
	}
	budget.lock.Lock()
	defer budget.lock.Unlock()
	if budget.maximum == 0 || budget.maximum > maxAttemptLimit {
		return nil, fmt.Errorf(
			"%w: maximum must be between 1 and %d", ErrInvalidAttemptBudget, maxAttemptLimit,
		)
	}
	if budget.used+budget.reserved >= budget.maximum {
		return nil, fmt.Errorf(
			"%w: used=%d reserved=%d maximum=%d",
			ErrAttemptBudgetExhausted,
			budget.used,
			budget.reserved,
			budget.maximum,
		)
	}
	budget.reserved++
	return &attemptReservation{budget: budget, active: true}, nil
}

func (reservation *attemptReservation) commit() (AttemptNumber, error) {
	if reservation == nil || reservation.budget == nil {
		return 0, fmt.Errorf("%w: reservation is nil", ErrInvalidAttemptBudget)
	}
	budget := reservation.budget
	budget.lock.Lock()
	defer budget.lock.Unlock()
	if !reservation.active || budget.reserved == 0 {
		return 0, fmt.Errorf("%w: reservation is inactive", ErrInvalidAttemptBudget)
	}
	budget.reserved--
	budget.used++
	reservation.active = false
	return AttemptNumber(budget.used), nil
}

func (reservation *attemptReservation) release() {
	if reservation == nil || reservation.budget == nil {
		return
	}
	budget := reservation.budget
	budget.lock.Lock()
	defer budget.lock.Unlock()
	if !reservation.active {
		return
	}
	if budget.reserved == 0 {
		panic("specialist workflow attempt reservation underflow")
	}
	budget.reserved--
	reservation.active = false
}

type Receipt[O BoundedCloneable[O], F BoundedCloneable[F]] struct {
	registration   Registration
	attempt        AttemptNumber
	executed       bool
	verified       bool
	observation    O
	hasObservation bool
	failure        F
	hasFailure     bool
}

func (receipt Receipt[O, F]) Registration() Registration {
	return receipt.registration
}

func (receipt Receipt[O, F]) Attempt() AttemptNumber {
	return receipt.attempt
}

func (receipt Receipt[O, F]) Executed() bool {
	return receipt.executed
}

func (receipt Receipt[O, F]) Verified() bool {
	return receipt.verified
}

func (receipt Receipt[O, F]) Observation() (O, bool, error) {
	if !receipt.hasObservation {
		var zero O
		return zero, false, nil
	}
	cloned, err := cloneBounded("receipt observation", receipt.observation)
	return cloned, true, err
}

func (receipt Receipt[O, F]) Failure() (F, bool, error) {
	if !receipt.hasFailure {
		var zero F
		return zero, false, nil
	}
	cloned, err := cloneBounded("receipt failure", receipt.failure)
	return cloned, true, err
}
