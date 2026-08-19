package api

import (
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

func (s *Server) handleChatSlashCommands(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := validateExactQuery(r, "channel_id"); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	channelID := model.ChannelID(r.URL.Query().Get("channel_id"))
	if err := channelID.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if s.channelStore == nil {
		writeRoleplaySimulationError(w, errRoleplayChannelStoreUnavailable)
		return
	}
	channel, err := s.channelStore.GetChannel(r.Context(), channelID)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	if err := channel.ValidateStored(); err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	if channel.Scope != model.ChannelScopeUser {
		writeRoleplaySimulationError(w, pgx.ErrNoRows)
		return
	}

	var projection *roleplay.SimulationSlashCommandProjection
	if channel.Mode == model.ChannelModeRoleplay {
		if s.roleplaySimulation == nil {
			writeRoleplaySimulationError(w, errRoleplaySimulationUnavailable)
			return
		}
		world, found, worldErr := s.roleplaySimulation.FindWorldByChannel(r.Context(), string(channel.ID))
		if worldErr != nil {
			writeRoleplaySimulationError(w, worldErr)
			return
		}
		if !found {
			writeRoleplaySimulationError(w, fmt.Errorf("%w: fictional world is absent", roleplay.ErrSimulationNotConfigured))
			return
		}
		if world.ChannelID != string(channel.ID) || world.Authority != roleplay.AuthorityFictionalCanon {
			writeRoleplaySimulationError(w, fmt.Errorf("roleplay slash command world changed channel authority"))
			return
		}
		projected, projectErr := s.roleplaySimulation.ProjectSimulationSlashCommands(r.Context(), world.ID)
		if projectErr != nil {
			writeRoleplaySimulationError(w, projectErr)
			return
		}
		if projected.WorldID != world.ID {
			writeRoleplaySimulationError(w, fmt.Errorf("roleplay slash command projection changed world authority"))
			return
		}
		projection = &projected
	}
	payload, err := renderChatSlashCommandsComponent(channel, projection)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, payload)
}
