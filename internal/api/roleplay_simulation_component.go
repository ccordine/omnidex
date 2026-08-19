package api

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	roleplaySimulationTarget   = "roleplay-simulation"
	roleplaySimulationPageSize = 4
)

type roleplaySimulationPageState struct {
	Characters    int
	Personas      int
	TurnOrder     int
	Meters        int
	Inventory     int
	Interactions  int
	ItemTemplates int
}

type roleplayNamedPersona struct {
	Name       string
	Projection roleplay.PersonaProjection
}

type roleplaySimulationComponentState struct {
	Channel               model.Channel
	World                 roleplay.World
	Scene                 *roleplay.SceneSheet
	Characters            []roleplay.SimulationCharacterSummary
	CharactersMore        bool
	CharacterHasPersona   map[string]bool
	CharacterCapabilities map[string]roleplay.CharacterCapabilityProjection
	CharacterNames        map[string]string
	Personas              []roleplayNamedPersona
	PersonasMore          bool
	Participants          []roleplay.SceneParticipantProjection
	ParticipantsMore      bool
	Meters                []roleplay.MeterProjection
	MetersMore            bool
	Inventory             []roleplay.InventoryItemProjection
	InventoryMore         bool
	Interactions          []roleplay.InteractionCommandDefinition
	InteractionsMore      bool
	ItemTemplates         []roleplay.ItemTemplateDefinition
	ItemTemplatesMore     bool
	AllParticipants       []roleplay.SceneParticipantProjection
	ActiveCharacterName   string
	SceneDraft            roleplaySceneDraft
	Page                  roleplaySimulationPageState
}

type roleplaySimulationComponentResponse struct {
	ChannelID          model.ChannelID   `json:"channel_id"`
	WorldID            string            `json:"world_id"`
	Configured         bool              `json:"configured"`
	SceneRevision      *int64            `json:"scene_revision,omitempty"`
	SceneDraftRevision int64             `json:"scene_draft_revision"`
	HTML               chatComponentHTML `json:"html"`
}

func renderRoleplaySimulationComponent(
	state roleplaySimulationComponentState,
) (roleplaySimulationComponentResponse, error) {
	if err := validateRoleplaySimulationComponentState(state); err != nil {
		return roleplaySimulationComponentResponse{}, err
	}
	configured := state.Scene != nil
	var body strings.Builder
	body.WriteString(`<div class="space-y-3" data-roleplay-world-id="` + html.EscapeString(state.World.ID) +
		`" data-roleplay-scene-draft-revision="` + strconv.FormatInt(state.SceneDraft.Revision, 10) + `">`)
	body.WriteString(`<header class="rounded-md border border-violet-300/20 bg-violet-300/5 p-3">`)
	body.WriteString(`<p class="text-[10px] uppercase tracking-[.16em] text-violet-200/80">Roleplay simulation</p>`)
	body.WriteString(`<h3 class="mt-1 text-base font-semibold text-zinc-100">` + html.EscapeString(state.World.Name) + `</h3>`)
	if configured {
		body.WriteString(`<p class="mt-1 text-xs text-emerald-200">Server-authoritative scene is ready.</p>`)
	} else {
		body.WriteString(`<p role="status" class="mt-1 text-xs text-amber-200">Simulation setup required before sending a turn.</p>`)
	}
	body.WriteString(`</header>`)
	var sections string
	var err error
	if configured {
		sections, err = renderRoleplayConfiguredSections(state)
	} else {
		sections, err = renderRoleplaySetupSections(state)
	}
	if err != nil {
		return roleplaySimulationComponentResponse{}, err
	}
	body.WriteString(sections)
	body.WriteString(`</div>`)
	bundle := renderRecyclrTemplateHTML(roleplaySimulationTarget, body.String(), "innerHTML")
	response := roleplaySimulationComponentResponse{
		ChannelID: state.Channel.ID, WorldID: state.World.ID, Configured: configured,
		SceneDraftRevision: state.SceneDraft.Revision,
		HTML:               chatComponentHTML{Bundle: bundle},
	}
	if configured {
		revision := state.Scene.Revision
		response.SceneRevision = &revision
	}
	return response, nil
}

func validateRoleplaySimulationComponentState(state roleplaySimulationComponentState) error {
	if err := state.Channel.ValidateStored(); err != nil {
		return fmt.Errorf("roleplay component channel: %w", err)
	}
	if state.Channel.Mode != model.ChannelModeRoleplay || state.Channel.RoleplayViewpointCharacterID == "" {
		return fmt.Errorf("roleplay component requires persisted roleplay channel authority")
	}
	if state.World.ChannelID != string(state.Channel.ID) || state.World.Authority != roleplay.AuthorityFictionalCanon ||
		state.World.ID == "" || state.World.Name == "" {
		return fmt.Errorf("roleplay component world authority does not match its channel")
	}
	if err := validateRoleplaySceneDraft(state.SceneDraft, state.Channel.ID, state.World.ID); err != nil {
		return err
	}
	for _, participant := range state.SceneDraft.Participants {
		name := state.CharacterNames[participant.CharacterID]
		if name == "" || name != strings.TrimSpace(name) {
			return fmt.Errorf("roleplay component omitted a scene-draft participant name")
		}
	}
	counts := []struct {
		name    string
		count   int
		offset  int
		hasMore bool
	}{
		{"characters", len(state.Characters), state.Page.Characters, state.CharactersMore},
		{"personas", len(state.Personas), state.Page.Personas, state.PersonasMore},
		{"turn order", len(state.Participants), state.Page.TurnOrder, state.ParticipantsMore},
		{"meters", len(state.Meters), state.Page.Meters, state.MetersMore},
		{"inventory", len(state.Inventory), state.Page.Inventory, state.InventoryMore},
		{"interactions", len(state.Interactions), state.Page.Interactions, state.InteractionsMore},
		{"item templates", len(state.ItemTemplates), state.Page.ItemTemplates, state.ItemTemplatesMore},
	}
	for _, page := range counts {
		if page.offset < 0 || page.offset%roleplaySimulationPageSize != 0 || page.count > roleplaySimulationPageSize ||
			(page.hasMore && page.count != roleplaySimulationPageSize) {
			return fmt.Errorf("roleplay %s page is invalid", page.name)
		}
	}
	if state.Scene == nil {
		if state.SceneDraft.SceneRevision != 0 {
			return fmt.Errorf("unconfigured roleplay component contains a configured scene draft")
		}
		if len(state.Participants) != 0 || len(state.Meters) != 0 || len(state.Inventory) != 0 || len(state.Interactions) != 0 {
			return fmt.Errorf("unconfigured roleplay component contains configured scene state")
		}
		return nil
	}
	if state.Scene.WorldID != state.World.ID || state.Scene.ID == "" || state.Scene.Revision < 1 {
		return fmt.Errorf("roleplay component scene does not share exact world authority")
	}
	if state.SceneDraft.SceneRevision != state.Scene.Revision {
		return fmt.Errorf("roleplay component scene draft is not fenced to the configured scene revision")
	}
	if state.ActiveCharacterName == "" || state.ActiveCharacterName != strings.TrimSpace(state.ActiveCharacterName) {
		return fmt.Errorf("roleplay component requires the active character's exact server name")
	}
	if len(state.AllParticipants) < 1 || len(state.AllParticipants) > roleplay.MaxSceneParticipants {
		return fmt.Errorf("roleplay component requires its bounded complete participant authority")
	}
	for index, participant := range state.AllParticipants {
		if participant.TurnPosition != index {
			return fmt.Errorf("roleplay complete participant authority is not contiguous")
		}
	}
	for index, participant := range state.Participants {
		if participant.TurnPosition != state.Page.TurnOrder+index {
			return fmt.Errorf("roleplay turn-order page is not contiguous")
		}
	}
	for _, meter := range state.Meters {
		if meter.Revision < 1 || meter.Minimum >= meter.Maximum || meter.Value < meter.Minimum || meter.Value > meter.Maximum {
			return fmt.Errorf("roleplay meter page contains invalid persisted state")
		}
	}
	for _, item := range state.Inventory {
		if item.UsePolicy != roleplay.ItemUseFinite && item.UsePolicy != roleplay.ItemUseInfinite {
			return fmt.Errorf("roleplay inventory page contains invalid use policy")
		}
	}
	for _, command := range state.Interactions {
		if command.ArgumentMode != roleplay.CommandArgumentNone && command.ArgumentMode != roleplay.CommandArgumentRequired {
			return fmt.Errorf("roleplay interaction page contains invalid argument mode")
		}
	}
	return nil
}

func renderRoleplaySetupSections(state roleplaySimulationComponentState) (string, error) {
	roster, err := renderRoleplayCharacterRoster(state)
	if err != nil {
		return "", err
	}
	personas, err := renderRoleplayPersonaSheets(state)
	if err != nil {
		return "", err
	}
	return roster + personas + renderRoleplaySceneForm(state), nil
}

func renderRoleplayConfiguredSections(state roleplaySimulationComponentState) (string, error) {
	roster, err := renderRoleplayCharacterRoster(state)
	if err != nil {
		return "", err
	}
	turnOrder, err := renderRoleplayTurnOrder(state)
	if err != nil {
		return "", err
	}
	personas, err := renderRoleplayPersonaSheets(state)
	if err != nil {
		return "", err
	}
	meters, err := renderRoleplayMeters(state)
	if err != nil {
		return "", err
	}
	inventory, err := renderRoleplayInventory(state)
	if err != nil {
		return "", err
	}
	interactions, err := renderRoleplayInteractions(state)
	if err != nil {
		return "", err
	}
	items, err := renderRoleplayItemTemplates(state)
	if err != nil {
		return "", err
	}
	return renderRoleplaySceneSheet(state) + roster + turnOrder + personas + meters +
		inventory + interactions + items + renderRoleplayDefinitionForms(), nil
}
