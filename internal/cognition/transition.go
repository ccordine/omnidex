package cognition

import (
	"fmt"
	"math"
)

func (transition Transition) ValidateStart() error {
	if transition.Previous != nil {
		return fmt.Errorf("%w: start transition cannot have a previous revision", ErrInvalidTransition)
	}
	if transition.ActionID != "" {
		return fmt.Errorf("%w: start transition cannot claim an action identity", ErrInvalidTransition)
	}
	if transition.Current.Number != 1 {
		return fmt.Errorf("%w: start transition must establish revision one", ErrInvalidTransition)
	}
	if transition.Cost != 0 {
		return fmt.Errorf("%w: start transition cost must be zero", ErrInvalidTransition)
	}
	if err := transition.validateCommon(); err != nil {
		return err
	}
	for index, observation := range transition.Observations {
		if observation.ActionID != "" {
			return fmt.Errorf("%w: start observation %d cannot claim an action identity", ErrInvalidTransition, index)
		}
	}
	if len(transition.Effects) != 0 {
		return fmt.Errorf("%w: start transition cannot report action effects", ErrInvalidTransition)
	}
	return nil
}

func (transition Transition) ValidateApply(
	episode EpisodeRef,
	expected WorldRevision,
	action RegisteredAction,
) error {
	if err := episode.Validate(); err != nil {
		return fmt.Errorf("%w: episode: %v", ErrInvalidTransition, err)
	}
	if err := expected.Validate(); err != nil {
		return fmt.Errorf("%w: expected revision: %v", ErrInvalidTransition, err)
	}
	if err := action.validateBase(); err != nil {
		return fmt.Errorf("%w: action: %v", ErrInvalidTransition, err)
	}
	if transition.Previous == nil {
		return fmt.Errorf("%w: apply transition requires a previous revision", ErrInvalidTransition)
	}
	if *transition.Previous != expected {
		return fmt.Errorf("%w: previous revision does not equal the expected revision", ErrInvalidTransition)
	}
	if expected.EpisodeID != episode.ID {
		return fmt.Errorf("%w: expected revision belongs to another episode", ErrInvalidTransition)
	}
	if transition.ActionID != action.ID {
		return fmt.Errorf("%w: transition action identity does not match the applied action", ErrInvalidTransition)
	}
	if expected.Number == math.MaxUint64 || transition.Current.Number != expected.Number+1 {
		return fmt.Errorf("%w: current revision must immediately follow the previous revision", ErrInvalidTransition)
	}
	if transition.Current.EpisodeID != episode.ID {
		return fmt.Errorf("%w: current revision belongs to another episode", ErrInvalidTransition)
	}
	for index, ref := range action.EvidenceRefs {
		if ref.Revision.EpisodeID != episode.ID {
			return fmt.Errorf("%w: action evidence %d belongs to another episode", ErrInvalidTransition, index)
		}
	}
	if err := transition.validateCommon(); err != nil {
		return err
	}
	for index, observation := range transition.Observations {
		if observation.ActionID != action.ID {
			return fmt.Errorf("%w: observation %d does not name its producing action", ErrInvalidTransition, index)
		}
	}
	for index, effect := range transition.Effects {
		if effect.ActionID != action.ID {
			return fmt.Errorf("%w: effect %d does not name its producing action", ErrInvalidTransition, index)
		}
	}
	return nil
}

func (transition Transition) validateCommon() error {
	if err := transition.Current.Validate(); err != nil {
		return fmt.Errorf("%w: current revision: %v", ErrInvalidTransition, err)
	}
	if transition.Cost < 0 || transition.Cost > MaxTransitionCost {
		return fmt.Errorf("%w: cost must be between zero and %d", ErrInvalidTransition, MaxTransitionCost)
	}
	if transition.PublicOutcome != "" {
		if err := validateExactText(transition.PublicOutcome, "public outcome", MaxPublicOutcomeBytes); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidTransition, err)
		}
	} else if transition.Terminal {
		return fmt.Errorf("%w: terminal transition requires a public outcome", ErrInvalidTransition)
	}
	if len(transition.Observations) > MaxTransitionObservations {
		return fmt.Errorf("%w: observation count exceeds %d", ErrInvalidTransition, MaxTransitionObservations)
	}
	if len(transition.Effects) > MaxTransitionEffects {
		return fmt.Errorf("%w: effect count exceeds %d", ErrInvalidTransition, MaxTransitionEffects)
	}
	seen := make(map[ObservationID]struct{}, len(transition.Observations))
	for index, observation := range transition.Observations {
		if err := observation.Validate(); err != nil {
			return fmt.Errorf("%w: observation %d: %v", ErrInvalidTransition, index, err)
		}
		if observation.Revision != transition.Current {
			return fmt.Errorf("%w: observation %d is not bound to the current revision", ErrInvalidTransition, index)
		}
		if _, duplicate := seen[observation.ID]; duplicate {
			return fmt.Errorf("%w: observation ID %q is duplicated", ErrInvalidTransition, observation.ID)
		}
		seen[observation.ID] = struct{}{}
	}
	seenEffects := make(map[string]struct{}, len(transition.Effects))
	for index, effect := range transition.Effects {
		if err := effect.Validate(); err != nil {
			return fmt.Errorf("%w: effect %d: %v", ErrInvalidTransition, index, err)
		}
		if effect.Revision != transition.Current {
			return fmt.Errorf("%w: effect %d is not bound to the current revision", ErrInvalidTransition, index)
		}
		key := string(effect.Kind) + "\x00" + effect.ContentSHA256
		if _, duplicate := seenEffects[key]; duplicate {
			return fmt.Errorf("%w: effect %d is duplicated", ErrInvalidTransition, index)
		}
		seenEffects[key] = struct{}{}
	}
	return nil
}

func (transition Transition) Clone() Transition {
	if transition.Previous != nil {
		previous := *transition.Previous
		transition.Previous = &previous
	}
	if transition.Observations != nil {
		observations := make([]Observation, len(transition.Observations))
		copy(observations, transition.Observations)
		transition.Observations = observations
	}
	if transition.Effects != nil {
		effects := make([]Effect, len(transition.Effects))
		copy(effects, transition.Effects)
		transition.Effects = effects
	}
	return transition
}
