package api

import (
	"net/http"

	"github.com/gryph/omnidex/internal/model"
)

func (s *Server) handleRoleplayWorldsComponent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit, offset, ok := exactChatComponentPage(w, r)
	if !ok {
		return
	}
	if s.roleplaySimulation == nil {
		writeError(w, http.StatusServiceUnavailable, errRoleplaySimulationUnavailable.Error())
		return
	}
	page, err := s.roleplaySimulation.ListWorldsPage(r.Context(), limit, offset)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	payload, err := renderRoleplayWorldPage(page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeChatComponentJSON(w, payload)
}

func (s *Server) handleRoleplayLibraryComponent(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if err := validateExactQuery(r, "limit", "offset", "channel_id"); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		s.writeRoleplayLibraryPage(w, r, http.StatusOK)
	case http.MethodPost:
		if err := validateExactQuery(r, "channel_id"); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if s.roleplaySimulation == nil {
			writeError(w, http.StatusServiceUnavailable, errRoleplaySimulationUnavailable.Error())
			return
		}
		var request roleplayCharacterRequest
		if err := decodeExactRoleplayJSON(w, r, "roleplay library character request", &request); err != nil {
			writeChannelBodyError(w, err)
			return
		}
		if err := validateRoleplayCharacterRequest(request); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if _, err := s.roleplaySimulation.CreateLibraryCharacter(r.Context(), request.Name); err != nil {
			writeRoleplaySimulationError(w, err)
			return
		}
		s.writeRoleplayLibraryPage(w, r, http.StatusCreated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) writeRoleplayLibraryPage(w http.ResponseWriter, r *http.Request, status int) {
	limit, offset, ok := exactChatComponentPage(w, r)
	if !ok {
		return
	}
	if status == http.StatusCreated && offset != 0 {
		writeError(w, http.StatusBadRequest, "created character reconciliation requires library offset zero")
		return
	}
	if s.roleplaySimulation == nil {
		writeError(w, http.StatusServiceUnavailable, errRoleplaySimulationUnavailable.Error())
		return
	}
	selectedWorldID, ok := s.roleplayLibrarySelectedWorldID(w, r)
	if !ok {
		return
	}
	page, err := s.roleplaySimulation.ListLibraryCharactersPage(
		r.Context(), selectedWorldID, limit, offset,
	)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	payload, err := renderRoleplayLibraryPage(page, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, payload)
}

func (s *Server) roleplayLibrarySelectedWorldID(w http.ResponseWriter, r *http.Request) (string, bool) {
	channelValue, err := exactChatComponentString(r, "channel_id", 96)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	if channelValue == "" {
		return "", true
	}
	channelID := model.ChannelID(channelValue)
	if err := channelID.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	world, found, err := s.roleplaySimulation.FindWorldByChannel(r.Context(), channelValue)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return "", false
	}
	if !found {
		writeError(w, http.StatusNotFound, "roleplay world not found")
		return "", false
	}
	return world.ID, true
}
