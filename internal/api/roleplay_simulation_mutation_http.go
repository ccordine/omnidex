package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

type roleplayUserPersonaCreationReceipt struct {
	ChannelID   model.ChannelID `json:"channel_id"`
	CharacterID string          `json:"character_id"`
}

func (s *Server) handleRoleplaySimulationChannel(
	w http.ResponseWriter,
	r *http.Request,
	channelID model.ChannelID,
	parts []string,
) {
	switch {
	case len(parts) == 1 && parts[0] == "user-personas":
		if requireRoleplayMethod(w, r, http.MethodPost) {
			s.createRoleplayUserPersona(w, r, channelID)
		}
	case len(parts) == 2 && parts[0] == "personas":
		if requireRoleplayMethod(w, r, http.MethodPut) {
			s.writeRoleplayPersona(w, r, channelID, parts[1])
		}
	case len(parts) == 2 && parts[0] == "generation":
		if requireRoleplayMethod(w, r, http.MethodPut) {
			s.writeRoleplayCharacterGeneration(w, r, channelID, parts[1])
		}
	case len(parts) == 2 && parts[0] == "library":
		if requireRoleplayMethod(w, r, http.MethodPost) {
			s.placeRoleplayLibraryCharacter(w, r, channelID, parts[1])
		}
	case len(parts) == 1 && parts[0] == "scene":
		if requireRoleplayMethod(w, r, http.MethodPost, http.MethodPut) {
			s.writeRoleplayScene(w, r, channelID)
		}
	case len(parts) == 1 && parts[0] == "responders":
		if requireRoleplayMethod(w, r, http.MethodPut) {
			s.writeRoleplayResponders(w, r, channelID)
		}
	case len(parts) == 3 && parts[0] == "scene-draft" && parts[1] == "participants":
		if requireRoleplayMethod(w, r, http.MethodPut) {
			s.writeRoleplaySceneDraftParticipant(w, r, channelID, parts[2])
		}
	case len(parts) == 1 && parts[0] == "meters":
		if requireRoleplayMethod(w, r, http.MethodPost) {
			s.registerRoleplayMeter(w, r, channelID)
		}
	case len(parts) == 3 && parts[0] == "meters":
		if requireRoleplayMethod(w, r, http.MethodPut) {
			s.setRoleplayMeter(w, r, channelID, parts[1], parts[2])
		}
	case len(parts) == 1 && parts[0] == "interactions":
		if requireRoleplayMethod(w, r, http.MethodPost) {
			s.registerRoleplayInteraction(w, r, channelID)
		}
	case len(parts) == 1 && parts[0] == "items":
		if requireRoleplayMethod(w, r, http.MethodPost) {
			s.registerRoleplayItem(w, r, channelID)
		}
	case len(parts) == 3 && parts[0] == "capabilities" && parts[2] == "web-research":
		if requireRoleplayMethod(w, r, http.MethodPut) {
			s.configureRoleplayResearch(w, r, channelID, parts[1])
		}
	default:
		writeError(w, http.StatusNotFound, "roleplay configuration route not found")
	}
}

func (s *Server) writeRoleplayResponders(
	w http.ResponseWriter,
	r *http.Request,
	channelID model.ChannelID,
) {
	channel, world, ok := s.roleplayMutationAuthority(w, r, channelID)
	if !ok {
		return
	}
	var request roleplayResponderOrderRequest
	if err := decodeExactRoleplayJSON(w, r, "roleplay responder order request", &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	if err := validateRoleplayResponderOrderRequest(request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	current, err := s.roleplaySimulation.ProjectCurrentScene(r.Context(), world.ID)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	if current.Revision != *request.ExpectedRevision {
		writeRoleplaySimulationError(w, fmt.Errorf("%w: scene revision changed", roleplay.ErrSimulationStaleRevision))
		return
	}
	if _, err := s.roleplaySimulation.UpdateCurrentScene(r.Context(), roleplay.SceneUpdate{
		WorldID: world.ID, SceneID: current.ID, ExpectedRevision: current.Revision,
		Title: current.Title, Description: current.Description,
		ParticipantIDs: append([]string(nil), request.CharacterIDs...),
	}); err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	s.writeRoleplaySimulationComponent(w, r, http.StatusOK, channel, world, roleplaySimulationPageState{})
}

func (s *Server) createRoleplayUserPersona(
	w http.ResponseWriter,
	r *http.Request,
	channelID model.ChannelID,
) {
	channel, world, ok := s.roleplayMutationAuthority(w, r, channelID)
	if !ok {
		return
	}
	var request roleplayCharacterRequest
	if err := decodeExactRoleplayJSON(w, r, "roleplay user persona request", &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	if err := validateRoleplayCharacterRequest(request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	character, err := s.roleplaySimulation.CreateSceneParticipant(r.Context(), world.ID, request.Name)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, roleplayUserPersonaCreationReceipt{
		ChannelID: channel.ID, CharacterID: character.ID,
	})
}

func (s *Server) writeRoleplayCharacterGeneration(
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
	var request roleplayGenerationRequest
	if err := decodeExactRoleplayJSON(w, r, "roleplay character generation request", &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	if err := validateRoleplayGenerationRequest(request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	for _, modelName := range []string{request.NarrativeModel} {
		if modelName == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		installed, err := s.hasInstalledOllamaModel(ctx, modelName)
		cancel()
		if err != nil {
			writeError(w, http.StatusBadGateway, err.Error())
			return
		}
		if !installed {
			writeError(w, http.StatusConflict, "Ollama model is not installed: "+modelName)
			return
		}
	}
	if _, err := s.roleplaySimulation.WriteCharacterGeneration(r.Context(), roleplay.CharacterGenerationWriteRequest{
		WorldID: world.ID, CharacterID: characterID, ExpectedRevision: *request.ExpectedRevision,
		NarrativeModel: request.NarrativeModel,
	}); err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	s.writeRoleplaySimulationComponent(w, r, http.StatusOK, channel, world, roleplaySimulationPageState{})
}

func (s *Server) placeRoleplayLibraryCharacter(
	w http.ResponseWriter,
	r *http.Request,
	channelID model.ChannelID,
	libraryID string,
) {
	if !roleplayLibraryIdentityPattern.MatchString(libraryID) {
		writeError(w, http.StatusBadRequest, "roleplay library character identity is invalid")
		return
	}
	channel, world, ok := s.roleplayMutationAuthority(w, r, channelID)
	if !ok {
		return
	}
	if _, err := s.roleplaySimulation.PlaceLibraryCharacter(r.Context(), world.ID, libraryID); err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	s.writeRoleplaySimulationComponent(w, r, http.StatusCreated, channel, world, roleplaySimulationPageState{})
}

func (s *Server) writeRoleplayPersona(
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
	var request roleplayPersonaRequest
	if err := decodeExactRoleplayJSON(w, r, "roleplay persona request", &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	if err := validateRoleplayPersonaRequest(request); err != nil {
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
	if character.CharacterID != characterID || character.WorldID != world.ID || character.CharacterName == "" {
		writeRoleplaySimulationError(w, fmt.Errorf(
			"%w: persona character changed authority", roleplay.ErrSimulationIllegal,
		))
		return
	}
	if _, err := s.roleplaySimulation.WritePersona(r.Context(), roleplay.PersonaWriteRequest{
		CharacterID: characterID, ExpectedRevision: *request.ExpectedRevision, Sheet: request.sheet(),
	}); err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	s.writeRoleplaySimulationComponent(w, r, http.StatusOK, channel, world, roleplaySimulationPageState{})
}

func (s *Server) writeRoleplayScene(w http.ResponseWriter, r *http.Request, channelID model.ChannelID) {
	channel, world, ok := s.roleplayMutationAuthority(w, r, channelID)
	if !ok {
		return
	}
	var request roleplaySceneRequest
	if err := decodeExactRoleplayJSON(w, r, "roleplay scene request", &request); err != nil {
		writeChannelBodyError(w, err)
		return
	}
	if err := validateRoleplaySceneRequest(request, r.Method == http.MethodPut); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := http.StatusCreated
	if r.Method == http.MethodPost {
		sessionID := s.ensureUISessionCookie(w, r)
		createScene := func() error {
			id, identityErr := roleplay.NewSceneIdentity()
			if identityErr != nil {
				return identityErr
			}
			_, createErr := s.roleplaySimulation.CreateCurrentScene(r.Context(), roleplay.SceneSetup{
				ID: id, WorldID: world.ID, Title: request.Title,
				Description: request.Description, ParticipantIDs: request.ParticipantIDs,
			})
			return createErr
		}
		var err error
		if request.ExpectedDraftRevision == nil {
			err = createScene()
		} else {
			s.roleplaySceneDraftMu.Lock()
			err = s.validateRoleplaySceneDraftSelectionLocked(
				r.Context(), sessionID, channel.ID, world.ID, *request.ExpectedDraftRevision, 0, request.ParticipantIDs,
			)
			if err == nil {
				err = createScene()
			}
			s.roleplaySceneDraftMu.Unlock()
		}
		if err != nil {
			writeRoleplaySimulationError(w, err)
			return
		}
	} else {
		updateScene := func() error {
			current, err := s.roleplaySimulation.ProjectCurrentScene(r.Context(), world.ID)
			if err != nil {
				return err
			}
			_, err = s.roleplaySimulation.UpdateCurrentScene(r.Context(), roleplay.SceneUpdate{
				WorldID: world.ID, SceneID: current.ID, ExpectedRevision: *request.ExpectedRevision,
				Title: request.Title, Description: request.Description, ParticipantIDs: request.ParticipantIDs,
			})
			return err
		}
		var err error
		if request.ExpectedDraftRevision == nil {
			err = updateScene()
		} else {
			sessionID := s.ensureUISessionCookie(w, r)
			s.roleplaySceneDraftMu.Lock()
			err = s.validateRoleplaySceneDraftSelectionLocked(
				r.Context(), sessionID, channel.ID, world.ID, *request.ExpectedDraftRevision,
				*request.ExpectedRevision, request.ParticipantIDs,
			)
			if err == nil {
				err = updateScene()
			}
			s.roleplaySceneDraftMu.Unlock()
		}
		if err != nil {
			writeRoleplaySimulationError(w, err)
			return
		}
		status = http.StatusOK
	}
	s.writeRoleplaySimulationComponent(w, r, status, channel, world, roleplaySimulationPageState{})
}

func (s *Server) roleplayMutationAuthority(
	w http.ResponseWriter,
	r *http.Request,
	channelID model.ChannelID,
) (model.Channel, roleplay.World, bool) {
	if err := validateExactQuery(r); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return model.Channel{}, roleplay.World{}, false
	}
	channel, world, err := s.resolveRoleplayChannel(r.Context(), channelID)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return model.Channel{}, roleplay.World{}, false
	}
	return channel, world, true
}

func requireRoleplayMethod(w http.ResponseWriter, r *http.Request, methods ...string) bool {
	for _, method := range methods {
		if r.Method == method {
			return true
		}
	}
	http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	return false
}
