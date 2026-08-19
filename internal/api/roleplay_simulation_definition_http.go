package api

import (
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

func (s *Server) registerRoleplayMeter(w http.ResponseWriter, r *http.Request, channelID model.ChannelID) {
	channel, world, ok := s.roleplayMutationAuthority(w, r, channelID)
	if !ok {
		return
	}
	var request roleplayMeterRequest
	if err := decodeExactRoleplayJSON(w, r, "roleplay meter request", &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	definition, err := request.definition(world.ID)
	if err == nil {
		err = validateRoleplayMeterDefinition(definition)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.roleplaySimulation.RegisterMeter(r.Context(), definition); err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	s.writeRoleplaySimulationComponent(w, r, http.StatusCreated, channel, world, roleplaySimulationPageState{})
}

func (s *Server) setRoleplayMeter(
	w http.ResponseWriter,
	r *http.Request,
	channelID model.ChannelID,
	characterID, meterKey string,
) {
	if !roleplayCharacterIdentityPattern.MatchString(characterID) || !roleplaySimulationKeyPattern.MatchString(meterKey) {
		writeError(w, http.StatusBadRequest, "roleplay meter route identities are invalid")
		return
	}
	channel, world, ok := s.roleplayMutationAuthority(w, r, channelID)
	if !ok {
		return
	}
	var request roleplayMeterValueRequest
	if err := decodeExactRoleplayJSON(w, r, "roleplay meter value request", &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	if err := validateRoleplayMeterValueRequest(request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.roleplaySimulation.SetCharacterMeter(r.Context(), roleplay.MeterValueUpdate{
		WorldID: world.ID, CharacterID: characterID, MeterKey: meterKey,
		ExpectedRevision: *request.ExpectedRevision, Value: *request.Value,
	}); err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	s.writeRoleplaySimulationComponent(w, r, http.StatusOK, channel, world, roleplaySimulationPageState{})
}

func (s *Server) registerRoleplayInteraction(w http.ResponseWriter, r *http.Request, channelID model.ChannelID) {
	channel, world, ok := s.roleplayMutationAuthority(w, r, channelID)
	if !ok {
		return
	}
	var request roleplayInteractionRequest
	if err := decodeExactRoleplayJSON(w, r, "roleplay interaction request", &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	id, err := roleplay.NewInteractionCommandIdentity()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	definition, err := request.definition(id, world.ID)
	if err == nil {
		err = validateRoleplayInteractionDefinition(definition)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.roleplaySimulation.RegisterInteractionCommand(r.Context(), definition); err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	s.writeRoleplaySimulationComponent(w, r, http.StatusCreated, channel, world, roleplaySimulationPageState{})
}

func (s *Server) registerRoleplayItem(w http.ResponseWriter, r *http.Request, channelID model.ChannelID) {
	channel, world, ok := s.roleplayMutationAuthority(w, r, channelID)
	if !ok {
		return
	}
	var request roleplayItemRequest
	if err := decodeExactRoleplayJSON(w, r, "roleplay item request", &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	id, err := roleplay.NewItemTemplateIdentity()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	definition, err := request.definition(id, world.ID)
	if err == nil {
		err = validateRoleplayItemDefinition(definition)
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.roleplaySimulation.RegisterItemTemplate(r.Context(), definition); err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	s.writeRoleplaySimulationComponent(w, r, http.StatusCreated, channel, world, roleplaySimulationPageState{})
}
