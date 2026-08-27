package roleplay

import "fmt"

func validateOngoingActionResolutionInput(
	completionOperationID string,
	sourceKind OngoingActionSourceKind,
	sourcePosition int,
	previousOngoingAction, ongoingAction *string,
) error {
	if err := validateCompletionOperationID(completionOperationID); err != nil {
		return err
	}
	if err := validateOngoingActionSource(sourceKind, sourcePosition); err != nil {
		return err
	}
	for _, value := range []*string{previousOngoingAction, ongoingAction} {
		if value != nil {
			if err := ValidateOngoingActionText(*value); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateOngoingActionSource(sourceKind OngoingActionSourceKind, sourcePosition int) error {
	switch sourceKind {
	case OngoingActionSourceResponse:
		if sourcePosition < 0 || sourcePosition >= MaxSceneParticipants {
			return fmt.Errorf("roleplay ongoing-action response requires a bounded source position")
		}
	case OngoingActionSourceUserAction:
		if sourcePosition != UserActionOngoingActionSourcePosition {
			return fmt.Errorf("roleplay user ongoing-action requires the exact actor source position")
		}
	default:
		return fmt.Errorf("roleplay ongoing-action source kind %q is unsupported", sourceKind)
	}
	return nil
}

func scanOngoingActionState(row rowScanner) (OngoingActionState, error) {
	var state OngoingActionState
	if err := row.Scan(
		&state.ID, &state.Ordinal, &state.WorldID, &state.CharacterID,
		&state.SourceCompletionOperation, &state.SourceKind, &state.SourcePosition,
		&state.SourceMessageID, &state.OngoingAction, &state.Authority,
		&state.CreatedAt,
	); err != nil {
		return OngoingActionState{}, err
	}
	if err := validateOngoingActionState(state); err != nil {
		return OngoingActionState{}, err
	}
	return state, nil
}

func validateOngoingActionState(state OngoingActionState) error {
	if validateIdentity(state.ID, ongoingActionStateIdentity) != nil ||
		state.Ordinal < 1 ||
		validateCompletionOperationID(state.SourceCompletionOperation) != nil ||
		validateOngoingActionSource(state.SourceKind, state.SourcePosition) != nil ||
		validateIdentity(state.WorldID, worldIdentity) != nil ||
		validateIdentity(state.CharacterID, characterIdentity) != nil ||
		state.SourceMessageID < 1 ||
		state.Authority != AuthoritySimulationState || state.CreatedAt.IsZero() {
		return fmt.Errorf("roleplay ongoing-action state has invalid exact authority")
	}
	if state.OngoingAction != nil {
		return ValidateOngoingActionText(*state.OngoingAction)
	}
	return nil
}

func scanOngoingActionResolution(row rowScanner) (OngoingActionResolution, error) {
	var resolution OngoingActionResolution
	if err := row.Scan(
		&resolution.CompletionOperation, &resolution.SourceKind, &resolution.SourcePosition,
		&resolution.WorldID, &resolution.CharacterID, &resolution.SourceMessageID,
		&resolution.PreviousStateID, &resolution.CurrentStateID,
		&resolution.PreviousOngoingAction, &resolution.OngoingAction,
		&resolution.Changed, &resolution.Authority, &resolution.CreatedAt,
	); err != nil {
		return OngoingActionResolution{}, err
	}
	if err := validateOngoingActionResolution(resolution); err != nil {
		return OngoingActionResolution{}, err
	}
	return resolution, nil
}

func validateOngoingActionResolution(resolution OngoingActionResolution) error {
	if validateCompletionOperationID(resolution.CompletionOperation) != nil ||
		validateOngoingActionSource(resolution.SourceKind, resolution.SourcePosition) != nil ||
		validateIdentity(resolution.WorldID, worldIdentity) != nil ||
		validateIdentity(resolution.CharacterID, characterIdentity) != nil ||
		resolution.SourceMessageID < 1 ||
		resolution.Authority != AuthoritySimulationState ||
		resolution.CreatedAt.IsZero() {
		return fmt.Errorf("roleplay ongoing-action resolution has invalid exact authority")
	}
	for _, id := range []*string{resolution.PreviousStateID, resolution.CurrentStateID} {
		if id != nil && validateIdentity(*id, ongoingActionStateIdentity) != nil {
			return fmt.Errorf("roleplay ongoing-action resolution has an invalid state identity")
		}
	}
	for _, value := range []*string{
		resolution.PreviousOngoingAction, resolution.OngoingAction,
	} {
		if value != nil {
			if err := ValidateOngoingActionText(*value); err != nil {
				return err
			}
		}
	}
	changed := !equalOptionalOngoingAction(
		resolution.PreviousOngoingAction, resolution.OngoingAction,
	)
	if resolution.Changed != changed ||
		(!resolution.Changed && !equalOptionalOngoingAction(
			resolution.PreviousStateID, resolution.CurrentStateID,
		)) || (resolution.Changed && resolution.CurrentStateID == nil) {
		return fmt.Errorf("roleplay ongoing-action resolution has invalid delta authority")
	}
	return nil
}

func equalOptionalOngoingAction(left, right *string) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func cloneOptionalOngoingAction(value *string) *string {
	if value == nil {
		return nil
	}
	clone := *value
	return &clone
}
