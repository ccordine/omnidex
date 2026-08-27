package roleplay

import "fmt"

func initialSimulationInitiativeClock() SimulationInitiativeClock {
	return SimulationInitiativeClock{Round: 1, Turn: 1, FictionalTimeTick: 0}
}

func (clock SimulationInitiativeClock) Validate() error {
	if clock.Round < 1 || clock.Round > MaxSimulationInitiativeValue ||
		clock.Turn < 1 || clock.Turn > MaxSimulationInitiativeValue ||
		clock.FictionalTimeTick < 0 || clock.FictionalTimeTick >= MaxSimulationInitiativeValue ||
		clock.Turn != clock.FictionalTimeTick+1 || clock.Round > clock.Turn {
		return fmt.Errorf("simulation initiative clock is outside its exact bounds")
	}
	return nil
}

func advanceSimulationInitiative(
	scene lockedSimulationScene,
	excludedCharacterID string,
) (string, SimulationInitiativeClock, error) {
	if err := scene.Sheet.Initiative.Validate(); err != nil {
		return "", SimulationInitiativeClock{}, err
	}
	currentIndex, found := simulationParticipantIndex(scene.Participants, scene.Sheet.ActiveCharacterID)
	if !found {
		return "", SimulationInitiativeClock{}, fmt.Errorf(
			"%w: active character is not a scene participant", ErrSimulationNotConfigured,
		)
	}
	if excludedCharacterID != "" {
		_, excludedFound := simulationParticipantIndex(scene.Participants, excludedCharacterID)
		if !excludedFound {
			return "", SimulationInitiativeClock{}, fmt.Errorf(
				"%w: acting character is not a scene participant", ErrSimulationIllegal,
			)
		}
	}
	nextCharacterID, err := nextSceneCharacterExcept(scene, excludedCharacterID)
	if err != nil {
		return "", SimulationInitiativeClock{}, err
	}
	nextIndex, found := simulationParticipantIndex(scene.Participants, nextCharacterID)
	if !found {
		return "", SimulationInitiativeClock{}, fmt.Errorf(
			"%w: next character is not a scene participant", ErrSimulationNotConfigured,
		)
	}
	if scene.Sheet.Initiative.Turn == MaxSimulationInitiativeValue ||
		scene.Sheet.Initiative.FictionalTimeTick == MaxSimulationInitiativeValue-1 {
		return "", SimulationInitiativeClock{}, fmt.Errorf("simulation initiative clock reached its maximum")
	}
	next := SimulationInitiativeClock{
		Round:             scene.Sheet.Initiative.Round,
		Turn:              scene.Sheet.Initiative.Turn + 1,
		FictionalTimeTick: scene.Sheet.Initiative.FictionalTimeTick + 1,
	}
	if nextIndex <= currentIndex {
		if next.Round == MaxSimulationInitiativeValue {
			return "", SimulationInitiativeClock{}, fmt.Errorf("simulation initiative round reached its maximum")
		}
		next.Round++
	}
	if err := next.Validate(); err != nil {
		return "", SimulationInitiativeClock{}, err
	}
	return nextCharacterID, next, nil
}

func simulationParticipantIndex(
	participants []SceneParticipantProjection,
	characterID string,
) (int, bool) {
	for index, participant := range participants {
		if participant.CharacterID == characterID {
			return index, true
		}
	}
	return 0, false
}

func validateSimulationInitiativeAdvance(
	before SimulationInitiativeClock,
	after SimulationInitiativeClock,
	previousCharacterID string,
	activeCharacterID string,
	participantIDs []string,
	excludedCharacterID string,
) error {
	if err := before.Validate(); err != nil {
		return err
	}
	if err := after.Validate(); err != nil {
		return err
	}
	if after.Turn != before.Turn+1 ||
		after.FictionalTimeTick != before.FictionalTimeTick+1 {
		return fmt.Errorf("simulation initiative turn and fictional time must advance exactly once")
	}
	previousIndex := -1
	activeIndex := -1
	seen := make(map[string]struct{}, len(participantIDs))
	for index, characterID := range participantIDs {
		if validateIdentity(characterID, characterIdentity) != nil {
			return fmt.Errorf("simulation initiative participant is invalid")
		}
		if _, duplicate := seen[characterID]; duplicate {
			return fmt.Errorf("simulation initiative participant is duplicated")
		}
		seen[characterID] = struct{}{}
		if characterID == previousCharacterID {
			previousIndex = index
		}
		if characterID == activeCharacterID {
			activeIndex = index
		}
	}
	if previousIndex < 0 || activeIndex < 0 {
		return fmt.Errorf("simulation initiative cursor is not a participant")
	}
	expectedActiveCharacterID, err := nextSimulationParticipantID(
		participantIDs, previousCharacterID, excludedCharacterID,
	)
	if err != nil {
		return err
	}
	if activeCharacterID != expectedActiveCharacterID {
		return fmt.Errorf("simulation initiative cursor did not advance to the exact next eligible participant")
	}
	expectedRound := before.Round
	if activeIndex <= previousIndex {
		expectedRound++
	}
	if after.Round != expectedRound {
		return fmt.Errorf("simulation initiative round does not match cursor wraparound")
	}
	return nil
}

func nextSimulationParticipantID(
	participantIDs []string,
	currentCharacterID string,
	excludedCharacterID string,
) (string, error) {
	currentIndex := -1
	excludedFound := excludedCharacterID == ""
	for index, characterID := range participantIDs {
		if characterID == currentCharacterID {
			currentIndex = index
		}
		if characterID == excludedCharacterID {
			excludedFound = true
		}
	}
	if currentIndex < 0 {
		return "", fmt.Errorf("simulation initiative cursor is not a participant")
	}
	if !excludedFound {
		return "", fmt.Errorf("simulation acting character is not a participant")
	}
	for offset := 1; offset <= len(participantIDs); offset++ {
		candidate := participantIDs[(currentIndex+offset)%len(participantIDs)]
		if candidate != excludedCharacterID {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("simulation initiative has no eligible participant")
}
