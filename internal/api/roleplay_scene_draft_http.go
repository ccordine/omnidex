package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

type roleplaySceneDraftParticipantRequest struct {
	ExpectedRevision *int64 `json:"expected_revision"`
	Selected         *bool  `json:"selected"`
	CharactersOffset *int   `json:"characters_offset"`
}

func (s *Server) writeRoleplaySceneDraftParticipant(
	w http.ResponseWriter,
	r *http.Request,
	channelID model.ChannelID,
	characterID string,
) {
	if !roleplayCharacterIdentityPattern.MatchString(characterID) {
		writeError(w, http.StatusBadRequest, "roleplay character identity is invalid")
		return
	}
	channel, world, ok := s.roleplayMutationAuthority(w, r, channelID)
	if !ok {
		return
	}
	var request roleplaySceneDraftParticipantRequest
	if err := decodeExactRoleplayJSON(w, r, "roleplay scene draft participant request", &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	if request.ExpectedRevision == nil || *request.ExpectedRevision < 0 || request.Selected == nil ||
		request.CharactersOffset == nil || *request.CharactersOffset < 0 ||
		*request.CharactersOffset%roleplaySimulationPageSize != 0 {
		writeError(w, http.StatusBadRequest, "scene draft revision, selected state, and character page are required")
		return
	}
	character, err := s.roleplaySimulation.ProjectChannelCharacterContext(
		r.Context(), string(channel.ID), characterID, 1,
	)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	if character.WorldID != world.ID || character.CharacterID != characterID || character.CharacterName == "" {
		writeRoleplaySimulationError(w, fmt.Errorf("%w: scene draft character changed authority", roleplay.ErrSimulationIllegal))
		return
	}
	if _, err := s.roleplaySimulation.ProjectPersona(r.Context(), characterID); err != nil {
		if errors.Is(err, roleplay.ErrSimulationNotConfigured) {
			err = fmt.Errorf("%w: scene draft character requires a character sheet", roleplay.ErrSimulationIllegal)
		}
		writeRoleplaySimulationError(w, err)
		return
	}
	sessionID := s.ensureUISessionCookie(w, r)
	s.roleplaySceneDraftMu.Lock()
	draft, err := s.loadRoleplaySceneDraftLocked(r.Context(), sessionID, channel.ID, world.ID)
	if err == nil && draft.Revision != *request.ExpectedRevision {
		err = roleplay.ErrSimulationStaleRevision
	}
	if err == nil {
		var scene roleplay.SceneSheet
		scene, err = s.roleplaySimulation.ProjectCurrentScene(r.Context(), world.ID)
		if errors.Is(err, roleplay.ErrSimulationNotConfigured) {
			err = nil
			if draft.SceneRevision != 0 {
				err = roleplay.ErrSimulationStaleRevision
			}
		} else if err == nil && draft.SceneRevision != scene.Revision {
			err = roleplay.ErrSimulationStaleRevision
		}
	}
	if err == nil {
		draft, err = mutateRoleplaySceneDraft(draft, roleplaySceneDraftParticipant{
			CharacterID: characterID,
		}, *request.Selected)
	}
	if err == nil {
		err = s.persistRoleplaySceneDraftLocked(r.Context(), sessionID, draft)
	}
	s.roleplaySceneDraftMu.Unlock()
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	s.writeRoleplaySimulationComponent(w, r, http.StatusOK, channel, world, roleplaySimulationPageState{
		Characters: *request.CharactersOffset,
	})
}

func mutateRoleplaySceneDraft(
	draft roleplaySceneDraft,
	participant roleplaySceneDraftParticipant,
	selected bool,
) (roleplaySceneDraft, error) {
	index := -1
	for candidateIndex, candidate := range draft.Participants {
		if candidate.CharacterID == participant.CharacterID {
			index = candidateIndex
			break
		}
	}
	if selected && index < 0 {
		if len(draft.Participants) >= roleplay.MaxSceneParticipants {
			return roleplaySceneDraft{}, fmt.Errorf("%w: scene draft participant bound reached", roleplay.ErrSimulationConflict)
		}
		draft.Participants = append(draft.Participants, participant)
		draft.Revision++
	} else if !selected && index >= 0 {
		draft.Participants = append(draft.Participants[:index], draft.Participants[index+1:]...)
		draft.Revision++
	}
	return draft, nil
}
