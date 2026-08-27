package api

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/modelref"
	"github.com/gryph/omnidex/internal/roleplay"
)

const (
	roleplaySimulationTarget        = "roleplay-simulation"
	roleplayComposerAuthorityTarget = "roleplay-composer-authority"
	roleplayCastSidebarTarget       = "roleplay-cast-sidebar"
	roleplaySimulationPageSize      = 4
)

type roleplaySimulationPageState struct {
	Characters               int
	Personas                 int
	TurnOrder                int
	Meters                   int
	Inventory                int
	Interactions             int
	ItemTemplates            int
	ComposerPersonaCharacter string
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
	UserPersonaCharacters []roleplay.SimulationCharacterSummary
	CharactersMore        bool
	CharacterHasPersona   map[string]bool
	CharacterCapabilities map[string]roleplay.CharacterCapabilityProjection
	CharacterGeneration   map[string]roleplay.CharacterGenerationProjection
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
	ActiveGeneration      *roleplay.CharacterGenerationProjection
	LastUserTurn          *roleplay.UserTurnAuthority
	InstalledModelNames   []string
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
	body.WriteString(`<div class="flex h-full min-h-0 flex-col" data-roleplay-world-id="` + html.EscapeString(state.World.ID) +
		`" data-roleplay-scene-draft-revision="` + strconv.FormatInt(state.SceneDraft.Revision, 10) + `">`)
	body.WriteString(`<header class="shrink-0 border-b border-violet-300/15 bg-violet-300/[.04] px-4 py-3 md:px-5">`)
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
	composer, err := renderRoleplayComposerAuthority(state)
	if err != nil {
		return roleplaySimulationComponentResponse{}, err
	}
	cast, err := renderRoleplayCastSidebar(state)
	if err != nil {
		return roleplaySimulationComponentResponse{}, err
	}
	bundle := renderRecyclrTemplateHTML(roleplaySimulationTarget, body.String(), "innerHTML") +
		renderRecyclrTemplateHTML(roleplayComposerAuthorityTarget, composer, "innerHTML") +
		renderRecyclrTemplateHTML(roleplayCastSidebarTarget, cast, "innerHTML")
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
	seenModels := make(map[string]struct{}, len(state.InstalledModelNames))
	for _, modelName := range state.InstalledModelNames {
		if err := modelref.ValidateOllamaName(modelName); err != nil {
			return fmt.Errorf("roleplay component installed model: %w", err)
		}
		if _, duplicate := seenModels[modelName]; duplicate {
			return fmt.Errorf("roleplay component duplicates an installed model")
		}
		seenModels[modelName] = struct{}{}
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
	if len(state.UserPersonaCharacters) < 1 {
		return fmt.Errorf("roleplay component requires world character authority")
	}
	if err := validateRequestedRoleplayComposerPersona(state); err != nil {
		return err
	}
	seenPersonaCharacters := make(map[string]struct{}, len(state.UserPersonaCharacters))
	activeCharacterPresent := false
	for _, character := range state.UserPersonaCharacters {
		if character.WorldID != state.World.ID || character.ID == "" || character.Name == "" ||
			character.Name != strings.TrimSpace(character.Name) {
			return fmt.Errorf("roleplay user-persona character authority is invalid")
		}
		if _, duplicate := seenPersonaCharacters[character.ID]; duplicate {
			return fmt.Errorf("roleplay user-persona character authority is duplicated")
		}
		seenPersonaCharacters[character.ID] = struct{}{}
		generation, exists := state.CharacterGeneration[character.ID]
		if !exists || generation.CharacterID != character.ID || generation.Config.LibraryCharacterID != character.LibraryID {
			return fmt.Errorf("roleplay component omitted world character generation authority")
		}
		if err := generation.Config.Validate(); err != nil {
			return fmt.Errorf("roleplay character generation: %w", err)
		}
		activeCharacterPresent = activeCharacterPresent ||
			(state.Scene != nil && character.ID == state.Scene.ActiveCharacterID)
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
	if !activeCharacterPresent {
		return fmt.Errorf("roleplay user-persona authority omitted the initiative character")
	}
	if state.Scene.WorldID != state.World.ID || state.Scene.ID == "" || state.Scene.Revision < 1 {
		return fmt.Errorf("roleplay component scene does not share exact world authority")
	}
	if state.SceneDraft.SceneRevision != state.Scene.Revision {
		return fmt.Errorf("roleplay component scene draft is not fenced to the configured scene revision")
	}
	if state.ActiveCharacterName == "" || state.ActiveCharacterName != strings.TrimSpace(state.ActiveCharacterName) {
		return fmt.Errorf("roleplay component requires the initiative character's exact server name")
	}
	if state.ActiveGeneration == nil ||
		state.ActiveGeneration.CharacterID != state.Scene.ActiveCharacterID {
		return fmt.Errorf("roleplay component requires the initiative character's current generation authority")
	}
	if err := state.ActiveGeneration.Config.Validate(); err != nil {
		return fmt.Errorf("roleplay active generation: %w", err)
	}
	if listed := state.CharacterGeneration[state.Scene.ActiveCharacterID]; listed.Config != state.ActiveGeneration.Config {
		return fmt.Errorf("roleplay active generation differs from world character authority")
	}
	if state.LastUserTurn != nil {
		if err := state.LastUserTurn.Validate(); err != nil {
			return fmt.Errorf("roleplay component latest user turn: %w", err)
		}
		if state.LastUserTurn.PersonaKind == roleplay.UserPersonaLegacy ||
			state.LastUserTurn.ContributionKind == roleplay.UserContributionCommand {
			return fmt.Errorf("roleplay component latest user turn is not a reusable composer selection")
		}
	}
	if len(state.AllParticipants) < 1 || len(state.AllParticipants) > roleplay.MaxSceneParticipants {
		return fmt.Errorf("roleplay component requires its bounded complete participant authority")
	}
	for index, participant := range state.AllParticipants {
		if _, exists := seenPersonaCharacters[participant.CharacterID]; !exists {
			return fmt.Errorf("roleplay responder is not a current world character")
		}
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
	return renderRoleplaySetupFlow("cast", []roleplaySetupSection{
		{
			Key: "cast", Label: "1 · Cast",
			Description: "Choose scene participation. Select a character in the sidebar to edit them.",
			Body:        roster,
		},
		{
			Key: "scene", Label: "2 · Scene",
			Description: "Create the opening scene after selecting at least one cast member.",
			Body:        renderRoleplaySceneForm(state),
		},
	})
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
	return renderRoleplaySetupFlow("scene", []roleplaySetupSection{
		{
			Key: "scene", Label: "Scene",
			Description: "Edit the current scene and review its server-owned turn order.",
			Body:        renderRoleplaySceneSheet(state) + turnOrder,
		},
		{
			Key: "cast", Label: "Cast",
			Description: "Choose scene participation. Character editing opens from the sidebar.",
			Body:        roster,
		},
		{
			Key: "state", Label: "State",
			Description: "Review and update the initiative character's meters and inventory.",
			Body:        meters + inventory,
		},
		{
			Key: "actions", Label: "Actions & rules",
			Description: "Manage interaction commands, item templates, and simulation definitions.",
			Body:        interactions + items + renderRoleplayDefinitionForms(),
		},
	})
}
