package specialistworkflow

import (
	"context"
	"fmt"
)

func RunAttempt[
	S BoundedCloneable[S],
	C BoundedCloneable[C],
	O BoundedCloneable[O],
	F BoundedCloneable[F],
](
	ctx context.Context,
	budget *AttemptBudget,
	state S,
	contract Contract[S, C, O, F],
) (Receipt[O, F], error) {
	receipt := Receipt[O, F]{registration: contract.registration}
	if ctx == nil {
		return receipt, ErrNilContext
	}
	if err := ctx.Err(); err != nil {
		return receipt, err
	}
	if err := contract.validate(); err != nil {
		return receipt, err
	}
	reservation, err := budget.reserve()
	if err != nil {
		return receipt, err
	}
	defer reservation.release()
	if err := ctx.Err(); err != nil {
		return receipt, err
	}

	ownedState, err := cloneBounded("authoritative state", state)
	if err != nil {
		return receipt, err
	}
	config, err := contract.derive(ownedState)
	if err != nil {
		return receipt, fmt.Errorf(
			"derive configuration for workflow %q: %w", contract.registration.workflow, err,
		)
	}
	if err := ctx.Err(); err != nil {
		return receipt, err
	}
	ownedConfig, err := cloneBounded("derived configuration", config)
	if err != nil {
		return receipt, err
	}
	validationConfig, err := cloneBounded("configuration validation input", ownedConfig)
	if err != nil {
		return receipt, err
	}
	if err := contract.validateConfig(validationConfig); err != nil {
		return receipt, fmt.Errorf(
			"validate configuration for workflow %q: %w", contract.registration.workflow, err,
		)
	}
	if err := ctx.Err(); err != nil {
		return receipt, err
	}

	attempt, err := reservation.commit()
	if err != nil {
		return receipt, err
	}
	receipt.attempt = attempt
	if err := ctx.Err(); err != nil {
		return receipt, err
	}
	executionConfig, err := cloneBounded("execution configuration", ownedConfig)
	if err != nil {
		return receipt, err
	}
	observation, err := contract.execute(ctx, executionConfig)
	contextErr := ctx.Err()
	receipt.executed = true
	if contextErr != nil {
		return receipt, contextErr
	}
	if err != nil {
		return receipt, fmt.Errorf(
			"execute workflow %q attempt %d: %w", contract.registration.workflow, attempt, err,
		)
	}
	ownedObservation, err := cloneBounded("execution observation", observation)
	if err != nil {
		return receipt, err
	}
	receipt.observation = ownedObservation
	receipt.hasObservation = true
	if err := ctx.Err(); err != nil {
		return receipt, err
	}

	verificationConfig, err := cloneBounded("verification configuration", ownedConfig)
	if err != nil {
		return receipt, err
	}
	verificationObservation, err := cloneBounded("verification observation", ownedObservation)
	if err != nil {
		return receipt, err
	}
	verified, err := contract.verify(ctx, verificationConfig, verificationObservation)
	if err != nil {
		return receipt, fmt.Errorf(
			"verify workflow %q attempt %d: %w", contract.registration.workflow, attempt, err,
		)
	}
	if err := ctx.Err(); err != nil {
		return receipt, err
	}
	if verified {
		receipt.verified = true
		return receipt, nil
	}

	reductionConfig, err := cloneBounded("failure reduction configuration", ownedConfig)
	if err != nil {
		return receipt, err
	}
	reductionObservation, err := cloneBounded("failure reduction observation", ownedObservation)
	if err != nil {
		return receipt, err
	}
	failure, err := contract.reduceFailure(ctx, reductionConfig, reductionObservation)
	if err != nil {
		return receipt, fmt.Errorf(
			"reduce failure for workflow %q attempt %d: %w",
			contract.registration.workflow,
			attempt,
			err,
		)
	}
	if err := ctx.Err(); err != nil {
		return receipt, err
	}
	ownedFailure, err := cloneBounded("reduced failure", failure)
	if err != nil {
		return receipt, err
	}
	if err := ctx.Err(); err != nil {
		return receipt, err
	}
	receipt.failure = ownedFailure
	receipt.hasFailure = true
	return receipt, nil
}

func cloneBounded[T BoundedCloneable[T]](name string, value T) (T, error) {
	if err := value.ValidateBounds(); err != nil {
		var zero T
		return zero, fmt.Errorf("%w: %s: %w", ErrInvalidBoundedValue, name, err)
	}
	cloned := value.Clone()
	if err := cloned.ValidateBounds(); err != nil {
		var zero T
		return zero, fmt.Errorf("%w: cloned %s: %w", ErrInvalidBoundedValue, name, err)
	}
	return cloned, nil
}
