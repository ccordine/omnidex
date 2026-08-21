package roleplay

import "fmt"

// Responder returns the frozen authority for one character in this response
// round. Response order and membership remain owned by the preparation.
func (authority SimulationTurnAuthority) Responder(characterID string) (SimulationResponderAuthority, error) {
	if err := validateIdentity(characterID, characterIdentity); err != nil {
		return SimulationResponderAuthority{}, err
	}
	for _, responder := range authority.Responders {
		if responder.CharacterID == characterID {
			return responder, nil
		}
	}
	return SimulationResponderAuthority{}, fmt.Errorf(
		"simulation response round has no responder %q", characterID,
	)
}
