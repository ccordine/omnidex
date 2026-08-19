package api

import (
	"fmt"
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
	page, err := s.roleplaySimulation.ListSimulationCharactersPage(
		r.Context(), world.ID, roleplaySimulationPageSize, *request.CharactersOffset,
	)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	visible := false
	for _, character := range page.Items {
		if character.ID == characterID && character.WorldID == world.ID {
			visible = true
			break
		}
	}
	if !visible {
		writeRoleplaySimulationError(w, fmt.Errorf(
			"%w: research access character is not visible on the submitted server page",
			roleplay.ErrSimulationIllegal,
		))
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
	s.writeRoleplaySimulationComponent(w, r, http.StatusOK, channel, world, roleplaySimulationPageState{
		Characters: *request.CharactersOffset,
	})
}
