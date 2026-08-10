package labyrinth

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
)

func entityKinds(entities map[EntityID]Entity) map[EntityKind]struct{} {
	kinds := make(map[EntityKind]struct{})
	for _, entity := range entities {
		kinds[entity.Kind] = struct{}{}
	}
	return kinds
}

func (environment *Environment) failure(
	code cognition.ActionFailureCode,
	action cognition.RegisteredAction,
	expected cognition.WorldRevision,
	message string,
	cause error,
) error {
	failure, err := cognition.NewActionFailure(code, action, expected, message, nil)
	if err != nil {
		return fmt.Errorf("%w: typed failure authority is invalid: %v", cause, err)
	}
	return errors.Join(failure, cause)
}

func (environment *Environment) bindFailure(
	code cognition.ActionFailureCode,
	action cognition.RegisteredAction,
	expected cognition.WorldRevision,
	requestSHA256 string,
	message string,
	cause error,
) error {
	failure, err := cognition.NewActionFailure(code, action, expected, message, nil)
	if err != nil {
		return fmt.Errorf("%w: typed failure authority is invalid: %v", cause, err)
	}
	stored := failure.Clone()
	environment.receipts[action.ID] = actionReceipt{
		requestSHA256: requestSHA256,
		expected:      expected,
		failure:       &stored,
		cause:         cause,
	}
	return errors.Join(failure, cause)
}

func (environment *Environment) bindSurfaceFailure(
	action cognition.RegisteredAction,
	expected cognition.WorldRevision,
	requestSHA256 string,
	surfaceErr error,
) error {
	code := cognition.ActionFailureInvalidAction
	message := "The registered surface operation failed validation."
	if errors.Is(surfaceErr, ErrSurfacePrecondition) {
		code = cognition.ActionFailurePreconditionFailed
		message = "The registered surface operation precondition is not satisfied."
	} else if errors.Is(surfaceErr, ErrSurfaceLimit) {
		code = cognition.ActionFailureBudget
		message = "The bounded surface cannot represent the registered operation."
	}
	return environment.bindFailure(
		code, action, expected, requestSHA256, message,
		errors.Join(ErrSurfaceOperation, surfaceErr),
	)
}

func (receipt actionReceipt) result() (cognition.Transition, error) {
	if receipt.failure != nil {
		return cognition.Transition{}, errors.Join(receipt.failure.Clone(), receipt.cause)
	}
	if receipt.transition == nil {
		return cognition.Transition{}, fmt.Errorf("invalid symbolic action receipt")
	}
	return receipt.transition.Clone(), nil
}

func (environment *Environment) MarshalJSON() ([]byte, error) {
	environment.mu.Lock()
	defer environment.mu.Unlock()
	projection := struct {
		Scenario cognition.ScenarioRef    `json:"scenario"`
		Episode  cognition.EpisodeRef     `json:"episode"`
		Started  bool                     `json:"started"`
		Current  *cognition.WorldRevision `json:"current,omitempty"`
	}{Scenario: environment.scenario.ref, Episode: environment.episode, Started: environment.started}
	if environment.started {
		current := environment.current
		projection.Current = &current
	}
	return json.Marshal(projection)
}
