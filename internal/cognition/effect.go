package cognition

import "fmt"

func NewEffect(
	actionID ActionID,
	revision WorldRevision,
	kind EffectKind,
	content string,
) (Effect, error) {
	effect := Effect{
		ActionID: actionID, Revision: revision, Kind: kind,
		Content: content, ContentSHA256: contentSHA256(content),
	}
	if err := effect.Validate(); err != nil {
		return Effect{}, err
	}
	return effect, nil
}

func (effect Effect) Validate() error {
	if err := validateIdentity(string(effect.ActionID), "effect action ID"); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidEffect, err)
	}
	if err := effect.Revision.Validate(); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEffect, err)
	}
	switch effect.Kind {
	case EffectStateChanged, EffectObservationProduced, EffectNoChange:
	default:
		return fmt.Errorf("%w: kind %q is not registered", ErrInvalidEffect, effect.Kind)
	}
	if err := validateExactText(effect.Content, "effect content", MaxEffectBytes); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEffect, err)
	}
	if !validSHA256(effect.ContentSHA256) || contentSHA256(effect.Content) != effect.ContentSHA256 {
		return fmt.Errorf("%w: content hash does not bind the exact public effect", ErrInvalidEffect)
	}
	return nil
}
