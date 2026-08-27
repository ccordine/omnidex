package roleplay

import "fmt"

func validateNarrativeOngoingActions(
	projection NarrativeSimulationProjection,
	authority SimulationNarrativeAuthority,
) error {
	if len(projection.OngoingActions) != len(authority.OngoingActionStateIDs) ||
		len(projection.OngoingActions) != len(authority.OngoingActionCharacterIDs) ||
		len(projection.OngoingActions) > MaxSceneParticipants {
		return fmt.Errorf("narrative ongoing actions differ from their exact source identities")
	}
	if len(projection.Participants) != len(authority.ParticipantIDs) {
		return fmt.Errorf("narrative participants differ from their exact character identities")
	}
	participantNames := make(map[string]string, len(authority.ParticipantIDs))
	for index, characterID := range authority.ParticipantIDs {
		if validateIdentity(characterID, characterIdentity) != nil {
			return fmt.Errorf("narrative participant character identity is invalid")
		}
		participantNames[characterID] = projection.Participants[index]
	}
	seenStates := make(map[string]struct{}, len(projection.OngoingActions))
	seenCharacters := make(map[string]struct{}, len(projection.OngoingActions))
	for index, action := range projection.OngoingActions {
		stateID := authority.OngoingActionStateIDs[index]
		characterID := authority.OngoingActionCharacterIDs[index]
		if validateIdentity(stateID, ongoingActionStateIdentity) != nil ||
			validateIdentity(characterID, characterIdentity) != nil {
			return fmt.Errorf("narrative ongoing action source identity is invalid")
		}
		name, present := participantNames[characterID]
		if !present || name != action.CharacterName {
			return fmt.Errorf("narrative ongoing action differs from its exact character identity")
		}
		if _, duplicate := seenStates[stateID]; duplicate {
			return fmt.Errorf("narrative ongoing action state identity is duplicated")
		}
		if _, duplicate := seenCharacters[characterID]; duplicate {
			return fmt.Errorf("narrative ongoing action character identity is duplicated")
		}
		seenStates[stateID] = struct{}{}
		seenCharacters[characterID] = struct{}{}
		if err := ValidateOngoingActionText(action.Action); err != nil {
			return err
		}
	}
	return nil
}

// CurrentOngoingActionForCharacter resolves only by opaque character identity.
// Model-visible names have no routing authority.
func CurrentOngoingActionForCharacter(
	projection NarrativeSimulationProjection,
	authority SimulationNarrativeAuthority,
	characterID string,
) (*string, error) {
	if validateIdentity(characterID, characterIdentity) != nil {
		return nil, fmt.Errorf("ongoing-action lookup character identity is invalid")
	}
	if err := validateNarrativeOngoingActions(projection, authority); err != nil {
		return nil, err
	}
	for index, sourceCharacterID := range authority.OngoingActionCharacterIDs {
		if sourceCharacterID == characterID {
			return cloneOptionalOngoingAction(&projection.OngoingActions[index].Action), nil
		}
	}
	return nil, nil
}
