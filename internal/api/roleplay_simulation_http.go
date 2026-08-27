package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

var errRoleplaySimulationUnavailable = errors.New("roleplay simulation store is unavailable")

func (s *Server) handleChatRoleplaySimulation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	page, channelID, err := roleplayComponentQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	channel, world, err := s.resolveRoleplayChannel(r.Context(), channelID)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	s.writeRoleplaySimulationComponent(w, r, http.StatusOK, channel, world, page)
}

func (s *Server) writeRoleplaySimulationComponent(
	w http.ResponseWriter,
	r *http.Request,
	status int,
	channel model.Channel,
	world roleplay.World,
	page roleplaySimulationPageState,
) {
	sessionID := s.ensureUISessionCookie(w, r)
	s.roleplaySceneDraftMu.Lock()
	defer s.roleplaySceneDraftMu.Unlock()
	draft, err := s.loadRoleplaySceneDraftLocked(r.Context(), sessionID, channel.ID, world.ID)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	state, err := s.loadRoleplaySimulationState(r.Context(), channel, world, page, draft)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	draft, err = s.reconcileRoleplaySceneDraftLocked(r.Context(), sessionID, draft, state)
	if err != nil {
		writeRoleplaySimulationError(w, err)
		return
	}
	state.SceneDraft = draft
	payload, err := renderRoleplaySimulationComponent(state)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, status, payload)
}

func (s *Server) loadRoleplaySimulationState(
	ctx context.Context,
	channel model.Channel,
	world roleplay.World,
	page roleplaySimulationPageState,
	draft roleplaySceneDraft,
) (roleplaySimulationComponentState, error) {
	namedCharacterIDs := make([]string, len(draft.Participants))
	for index, participant := range draft.Participants {
		namedCharacterIDs[index] = participant.CharacterID
	}
	projection, err := s.roleplaySimulation.ProjectSimulationUI(ctx, world.ID, roleplay.SimulationUIPageRequest{
		Limit: roleplaySimulationPageSize, CharactersOffset: page.Characters,
		PersonasOffset: page.Personas, TurnOrderOffset: page.TurnOrder, MetersOffset: page.Meters,
		InventoryOffset: page.Inventory, InteractionsOffset: page.Interactions,
		ItemTemplatesOffset: page.ItemTemplates,
		NamedCharacterIDs:   namedCharacterIDs,
	})
	if err != nil {
		return roleplaySimulationComponentState{}, err
	}
	if projection.WorldID != world.ID {
		return roleplaySimulationComponentState{}, fmt.Errorf("roleplay UI projection changed world authority")
	}
	state := roleplaySimulationComponentState{
		Channel: channel, World: world, Characters: projection.Characters.Items,
		UserPersonaCharacters: projection.UserPersonaCharacters,
		CharactersMore:        projection.Characters.HasMore,
		CharacterHasPersona:   projection.CharacterHasPersona,
		CharacterCapabilities: projection.CharacterCapabilities,
		CharacterGeneration:   projection.CharacterGeneration,
		CharacterNames:        projection.CharacterNames,
		PersonasMore:          projection.Personas.HasMore, Page: page,
		Participants: projection.Participants.Items, ParticipantsMore: projection.Participants.HasMore,
		AllParticipants: projection.AllParticipants,
		Meters:          projection.Meters.Items, MetersMore: projection.Meters.HasMore,
		Inventory: projection.Inventory.Items, InventoryMore: projection.Inventory.HasMore,
		Interactions: projection.Interactions.Items, InteractionsMore: projection.Interactions.HasMore,
		ItemTemplates: projection.ItemTemplates.Items, ItemTemplatesMore: projection.ItemTemplates.HasMore,
		ActiveCharacterName: projection.ActiveCharacterName,
		ActiveGeneration:    projection.ActiveGeneration,
		LastUserTurn:        projection.LastUserTurn,
	}
	if projection.Scene != nil {
		scene := *projection.Scene
		state.Scene = &scene
	}
	if err := validateRequestedRoleplayComposerPersona(state); err != nil {
		return roleplaySimulationComponentState{}, err
	}
	state.InstalledModelNames, err = s.loadInstalledRoleplayModelNames(ctx)
	if err != nil {
		return roleplaySimulationComponentState{}, err
	}
	for _, persona := range projection.Personas.Items {
		name := projection.CharacterNames[persona.CharacterID]
		if name == "" {
			return roleplaySimulationComponentState{}, fmt.Errorf("roleplay UI projection omitted persona character name")
		}
		state.Personas = append(state.Personas, roleplayNamedPersona{Name: name, Projection: persona})
	}
	return state, nil
}

func (s *Server) resolveRoleplayChannel(
	ctx context.Context,
	channelID model.ChannelID,
) (model.Channel, roleplay.World, error) {
	if s.roleplaySimulation == nil {
		return model.Channel{}, roleplay.World{}, errRoleplaySimulationUnavailable
	}
	if s.channelStore == nil {
		return model.Channel{}, roleplay.World{}, errRoleplayChannelStoreUnavailable
	}
	channel, err := s.channelStore.GetChannel(ctx, channelID)
	if err != nil {
		return model.Channel{}, roleplay.World{}, err
	}
	if err := channel.ValidateStored(); err != nil {
		return model.Channel{}, roleplay.World{}, err
	}
	if channel.Scope != model.ChannelScopeUser {
		return model.Channel{}, roleplay.World{}, pgx.ErrNoRows
	}
	if channel.Mode != model.ChannelModeRoleplay || channel.RoleplayViewpointCharacterID == "" {
		return model.Channel{}, roleplay.World{}, fmt.Errorf("%w: channel is not a roleplay channel", roleplay.ErrSimulationIllegal)
	}
	world, found, err := s.roleplaySimulation.FindWorldByChannel(ctx, string(channel.ID))
	if err != nil {
		return model.Channel{}, roleplay.World{}, err
	}
	if !found {
		return model.Channel{}, roleplay.World{}, fmt.Errorf("%w: fictional world is absent", roleplay.ErrSimulationNotConfigured)
	}
	if world.ChannelID != string(channel.ID) || world.Authority != roleplay.AuthorityFictionalCanon {
		return model.Channel{}, roleplay.World{}, fmt.Errorf("roleplay world changed channel authority")
	}
	return channel, world, nil
}
