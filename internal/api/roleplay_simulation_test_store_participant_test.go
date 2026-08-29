package api

import (
	"context"
	"time"

	"github.com/gryph/omnidex/internal/roleplay"
)

func (s *roleplaySimulationTestStore) CreateSceneParticipant(
	ctx context.Context, worldID, name string,
) (roleplay.Character, error) {
	if s.scene == nil {
		return roleplay.Character{}, roleplay.ErrSimulationNotConfigured
	}
	if len(s.allParticipants) >= roleplay.MaxSceneParticipants {
		return roleplay.Character{}, roleplay.ErrSimulationConflict
	}
	character, err := s.CreateCharacter(ctx, worldID, name)
	if err != nil {
		return roleplay.Character{}, err
	}
	s.personaConfigured[character.ID] = true
	s.personas.Items = append(s.personas.Items, roleplay.PersonaProjection{
		CharacterID: character.ID, Revision: 1,
		Sheet: roleplay.PersonaSheet{
			Summary: name, Voice: "", Traits: []string{}, Goals: []string{},
		},
		UpdatedAt: time.Now().UTC(),
	})
	participant := roleplay.SceneParticipantProjection{
		CharacterID: character.ID, Name: character.Name, TurnPosition: len(s.allParticipants),
	}
	s.allParticipants = append(s.allParticipants, participant)
	s.participants.Items = append(s.participants.Items, participant)
	s.scene.Revision++
	return character, nil
}
