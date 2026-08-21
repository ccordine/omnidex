package api

import (
	"errors"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func (s *Server) handleRoleplayCharacterEditor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := validateExactQuery(r, "channel_id", "character_id"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	channelValue, err := exactChatComponentString(r, "channel_id", 96)
	if err != nil || channelValue == "" {
		if err == nil {
			err = errors.New("channel_id is required")
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	channelID := model.ChannelID(channelValue)
	if err := channelID.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	characterID, err := exactChatComponentString(r, "character_id", 36)
	if err != nil || !roleplayCharacterIdentityPattern.MatchString(characterID) {
		writeError(w, http.StatusBadRequest, "roleplay character identity is invalid")
		return
	}
	channel, world, err := s.resolveRoleplayChannel(r.Context(), channelID)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	character, err := s.roleplaySimulation.ProjectChannelCharacterContext(
		r.Context(), string(channel.ID), characterID, 1,
	)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	if character.CharacterID != characterID || character.WorldID != world.ID {
		writeError(w, http.StatusInternalServerError, "roleplay character editor authority changed during projection")
		return
	}
	persona, personaErr := s.roleplaySimulation.ProjectPersona(r.Context(), characterID)
	personaConfigured := personaErr == nil
	if personaErr != nil && !errors.Is(personaErr, roleplay.ErrSimulationNotConfigured) {
		writeRoleplaySimulationError(w, personaErr)
		return
	}
	capability, err := s.roleplaySimulation.ProjectCharacterCapability(r.Context(), world.ID, characterID)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	generation, err := s.roleplaySimulation.ProjectCharacterGeneration(r.Context(), world.ID, characterID)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	installedModels, err := s.loadInstalledRoleplayModelNames(r.Context())
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	payload, err := renderRoleplayCharacterEditor(roleplayCharacterEditorState{
		Channel: channel, World: world, Character: character,
		Persona: persona, PersonaConfigured: personaConfigured,
		Capability: capability, Generation: generation, InstalledModelNames: installedModels,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, payload)
}
