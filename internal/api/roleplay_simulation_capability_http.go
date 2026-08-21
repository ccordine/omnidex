package api

import (
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func (s *Server) configureRoleplayResearch(
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
	var request roleplayResearchCapabilityRequest
	if err := decodeExactRoleplayJSON(w, r, "roleplay research capability request", &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	if err := validateRoleplayResearchCapabilityRequest(request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
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
		writeError(w, http.StatusInternalServerError, "roleplay research character authority changed during projection")
		return
	}
	projection, err := s.roleplaySimulation.ConfigureCharacterCapability(
		r.Context(), world.ID, characterID, roleplay.CapabilityWebResearch, *request.Enabled,
	)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	if projection.WorldID != world.ID || projection.CharacterID != characterID || projection.WebResearch != *request.Enabled {
		writeError(w, http.StatusInternalServerError, "research capability reconciliation changed server authority")
		return
	}
	s.writeRoleplaySimulationComponent(w, r, http.StatusOK, channel, world, roleplaySimulationPageState{})
}
