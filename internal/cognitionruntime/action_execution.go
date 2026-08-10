package cognitionruntime

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/cognition"
)

func (runtime *Runtime) execute(
	ctx context.Context,
	binding Binding,
	record ActionRecord,
	dispatch bool,
	policyCalled bool,
	recoveredDecision bool,
	recoveredAction bool,
) (StepResult, error) {
	partial := StepResult{
		Binding: binding, Revision: record.ExpectedRevision, ActionID: record.Action.ID,
		RecoveredDecision: recoveredDecision, RecoveredAction: recoveredAction,
		PolicyCalled: policyCalled,
	}
	if dispatch {
		before := record
		updated, err := runtime.actions.MarkDispatched(ctx, ActionMutation{
			Binding: binding, ActionID: record.Action.ID, ExpectedRevision: record.ExpectedRevision,
		})
		if err != nil {
			return partial, fmt.Errorf("dispatch cognition action: %w", err)
		}
		if err := validateActionMutation(
			ActionMutation{Binding: binding, ActionID: before.Action.ID, ExpectedRevision: before.ExpectedRevision},
			updated, ActionDispatched,
		); err != nil {
			return partial, err
		}
		if err := requireSameActionIdentity(before, updated); err != nil {
			return partial, err
		}
		record = updated
	}
	action, err := authorizeAction(record, binding)
	if err != nil {
		return partial, err
	}
	partial.EnvironmentActions = 1
	transition, applyErr := runtime.environment.Apply(
		ctx, binding.Episode, record.ExpectedRevision, action.Clone(),
	)
	if applyErr != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return partial, contextErr
		}
		failure, typed := typedActionFailure(applyErr)
		if !typed {
			return partial, fmt.Errorf("%w: action %q: %v", ErrEnvironment, action.ID, applyErr)
		}
		if err := failure.Validate(action, record.ExpectedRevision); err != nil {
			return partial, fmt.Errorf("%w: invalid typed failure: %v", ErrEnvironment, err)
		}
		failureCopy := failure.Clone()
		partial.Failure = &failureCopy
		failed, err := runtime.actions.RecordFailure(ctx, FailureMutation{
			ActionMutation: ActionMutation{
				Binding: binding, ActionID: action.ID, ExpectedRevision: record.ExpectedRevision,
			},
			Failure: failure.Clone(),
		})
		if err != nil {
			return partial, fmt.Errorf("persist cognition action failure: %w", err)
		}
		if err := validateActionMutation(
			ActionMutation{Binding: binding, ActionID: action.ID, ExpectedRevision: record.ExpectedRevision},
			failed, ActionFailed,
		); err != nil {
			return partial, err
		}
		if err := requireSameActionIdentity(record, failed); err != nil {
			return partial, err
		}
		if failed.Failure == nil || !reflect.DeepEqual(*failed.Failure, failure) {
			return partial, fmt.Errorf("%w: persisted failure differs from the environment failure", ErrInvalidJournalState)
		}
		copy := failure.Clone()
		return StepResult{
			State: StepActionFailed, Binding: binding, Revision: record.ExpectedRevision,
			ActionID: action.ID, Failure: &copy, RecoveredDecision: recoveredDecision,
			RecoveredAction: recoveredAction, PolicyCalled: policyCalled,
			EnvironmentActions: 1,
		}, nil
	}
	if err := transition.ValidateApply(binding.Episode, record.ExpectedRevision, action); err != nil {
		return partial, fmt.Errorf("%w: invalid transition: %v", ErrEnvironment, err)
	}
	transitionCopy := transition.Clone()
	partial.Transition = &transitionCopy
	succeeded, err := runtime.actions.RecordTransition(ctx, TransitionMutation{
		ActionMutation: ActionMutation{
			Binding: binding, ActionID: action.ID, ExpectedRevision: record.ExpectedRevision,
		},
		Transition: transition.Clone(),
	})
	if err != nil {
		return partial, fmt.Errorf("persist cognition transition: %w", err)
	}
	if err := validateActionMutation(
		ActionMutation{Binding: binding, ActionID: action.ID, ExpectedRevision: record.ExpectedRevision},
		succeeded, ActionSucceeded,
	); err != nil {
		return partial, err
	}
	if err := requireSameActionIdentity(record, succeeded); err != nil {
		return partial, err
	}
	if succeeded.ResultRevision == nil || *succeeded.ResultRevision != transition.Current {
		return partial, fmt.Errorf("%w: persisted result revision differs from the environment transition", ErrInvalidJournalState)
	}
	copy := transition.Clone()
	return StepResult{
		State: StepActionSucceeded, Binding: binding, Revision: transition.Current,
		ActionID: action.ID, Transition: &copy, RecoveredDecision: recoveredDecision,
		RecoveredAction: recoveredAction, PolicyCalled: policyCalled,
		EnvironmentActions: 1,
	}, nil
}

func requireSameActionIdentity(before, after ActionRecord) error {
	left, right := before, after
	left.Status, right.Status = "", ""
	left.Failure, right.Failure = nil, nil
	left.ResultRevision, right.ResultRevision = nil, nil
	if !reflect.DeepEqual(left, right) {
		return fmt.Errorf("%w: action content changed during lifecycle transition", ErrInvalidJournalState)
	}
	return nil
}

func typedActionFailure(err error) (cognition.ActionFailure, bool) {
	var value cognition.ActionFailure
	if errors.As(err, &value) {
		return value.Clone(), true
	}
	var pointer *cognition.ActionFailure
	if errors.As(err, &pointer) && pointer != nil {
		return pointer.Clone(), true
	}
	return cognition.ActionFailure{}, false
}
