package specialistworkflow

import (
	"context"
	"fmt"
)

// BoundedCloneable is the explicit ownership boundary for every value that
// crosses the specialist kernel. ValidateBounds must inspect counts and byte
// sizes without copying the value. The kernel calls it before every Clone.
type BoundedCloneable[T interface{}] interface {
	ValidateBounds() error
	Clone() T
}

type DeriveConfig[S BoundedCloneable[S], C BoundedCloneable[C]] func(S) (C, error)

type ValidateConfig[C BoundedCloneable[C]] func(C) error

type Execute[C BoundedCloneable[C], O BoundedCloneable[O]] func(context.Context, C) (O, error)

type Verify[C BoundedCloneable[C], O BoundedCloneable[O]] func(context.Context, C, O) (bool, error)

type ReduceFailure[C BoundedCloneable[C], O BoundedCloneable[O], F BoundedCloneable[F]] func(
	context.Context,
	C,
	O,
) (F, error)

type Contract[
	S BoundedCloneable[S],
	C BoundedCloneable[C],
	O BoundedCloneable[O],
	F BoundedCloneable[F],
] struct {
	registration   Registration
	derive         DeriveConfig[S, C]
	validateConfig ValidateConfig[C]
	execute        Execute[C, O]
	verify         Verify[C, O]
	reduceFailure  ReduceFailure[C, O, F]
}

func NewContract[
	S BoundedCloneable[S],
	C BoundedCloneable[C],
	O BoundedCloneable[O],
	F BoundedCloneable[F],
](
	registration Registration,
	derive DeriveConfig[S, C],
	validateConfig ValidateConfig[C],
	execute Execute[C, O],
	verify Verify[C, O],
	reduceFailure ReduceFailure[C, O, F],
) (Contract[S, C, O, F], error) {
	contract := Contract[S, C, O, F]{
		registration:   registration,
		derive:         derive,
		validateConfig: validateConfig,
		execute:        execute,
		verify:         verify,
		reduceFailure:  reduceFailure,
	}
	if err := contract.validate(); err != nil {
		return Contract[S, C, O, F]{}, err
	}
	return contract, nil
}

func (contract Contract[S, C, O, F]) Registration() Registration {
	return contract.registration
}

func (contract Contract[S, C, O, F]) validate() error {
	if err := contract.registration.validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidContract, err)
	}
	if contract.derive == nil {
		return fmt.Errorf("%w: derive configuration function is nil", ErrInvalidContract)
	}
	if contract.validateConfig == nil {
		return fmt.Errorf("%w: validate configuration function is nil", ErrInvalidContract)
	}
	if contract.execute == nil {
		return fmt.Errorf("%w: execute function is nil", ErrInvalidContract)
	}
	if contract.verify == nil {
		return fmt.Errorf("%w: verify function is nil", ErrInvalidContract)
	}
	if contract.reduceFailure == nil {
		return fmt.Errorf("%w: reduce failure function is nil", ErrInvalidContract)
	}
	return nil
}
