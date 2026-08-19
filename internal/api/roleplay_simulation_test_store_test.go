package api

import (
	"context"
	"time"

	"github.com/gryph/omnidex/internal/roleplay"
)

type roleplaySimulationTestStore struct {
	world                    roleplay.World
	scene                    *roleplay.SceneSheet
	characters               roleplay.SimulationCharacterPage
	personas                 roleplay.PersonaPage
	participants             roleplay.SceneParticipantPage
	allParticipants          []roleplay.SceneParticipantProjection
	meters                   roleplay.MeterPage
	inventory                roleplay.InventoryPage
	interactions             roleplay.InteractionCommandPage
	itemTemplates            roleplay.ItemTemplatePage
	names                    map[string]string
	personaConfigured        map[string]bool
	researchEnabled          map[string]bool
	characterCreateCalls     int
	configuredPageCalls      int
	meterCharacterID         string
	inventoryCharacterID     string
	characterOffset          int
	personaOffset            int
	participantOffset        int
	meterOffset              int
	inventoryOffset          int
	interactionOffset        int
	itemTemplateOffset       int
	snapshotCalls            int
	writePersonaErr          error
	sceneUpdateErr           error
	lastPersona              roleplay.PersonaWriteRequest
	lastSceneSetup           roleplay.SceneSetup
	lastSceneUpdate          roleplay.SceneUpdate
	lastMeterDefinition      roleplay.MeterDefinition
	lastMeterUpdate          roleplay.MeterValueUpdate
	lastInteraction          roleplay.InteractionCommandDefinition
	lastItem                 roleplay.ItemTemplateDefinition
	capabilityConfigureCalls int
	lastCapability           roleplay.CharacterCapabilityProjection
	slashCommands            *roleplay.SimulationSlashCommandProjection
	slashCommandCalls        int
}

func (s *roleplaySimulationTestStore) ProjectSimulationSlashCommands(
	_ context.Context,
	worldID string,
) (roleplay.SimulationSlashCommandProjection, error) {
	s.slashCommandCalls++
	if s.slashCommands != nil {
		projection := *s.slashCommands
		projection.Commands = append([]roleplay.SimulationSlashCommand(nil), projection.Commands...)
		return projection, nil
	}
	if s.scene == nil {
		return roleplay.SimulationSlashCommandProjection{}, roleplay.ErrSimulationNotConfigured
	}
	activeName := s.names[s.scene.ActiveCharacterID]
	return roleplay.SimulationSlashCommandProjection{
		Schema:  roleplay.SimulationSlashCommandProjectionSchemaV1,
		WorldID: worldID, SceneID: s.scene.ID, SceneRevision: s.scene.Revision,
		ActiveCharacterID: s.scene.ActiveCharacterID, ActiveCharacterName: activeName,
	}, nil
}

func newRoleplaySimulationTestStore(channelID string) *roleplaySimulationTestStore {
	return &roleplaySimulationTestStore{
		world: roleplay.World{
			ID: "rpw_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", ChannelID: channelID,
			Name: "Server World", Authority: roleplay.AuthorityFictionalCanon, CreatedAt: time.Now().UTC(),
		},
		names: make(map[string]string), personaConfigured: make(map[string]bool),
		researchEnabled: make(map[string]bool),
	}
}

func (s *roleplaySimulationTestStore) FindWorldByChannel(
	_ context.Context, channelID string,
) (roleplay.World, bool, error) {
	if channelID != s.world.ChannelID {
		return roleplay.World{}, false, nil
	}
	return s.world, true, nil
}

func (s *roleplaySimulationTestStore) ProjectSimulationUI(
	_ context.Context, worldID string, page roleplay.SimulationUIPageRequest,
) (roleplay.SimulationUIProjection, error) {
	s.snapshotCalls++
	s.characterOffset = page.CharactersOffset
	s.personaOffset = page.PersonasOffset
	s.participantOffset = page.TurnOrderOffset
	s.meterOffset = page.MetersOffset
	s.inventoryOffset = page.InventoryOffset
	s.interactionOffset = page.InteractionsOffset
	s.itemTemplateOffset = page.ItemTemplatesOffset
	projection := roleplay.SimulationUIProjection{
		WorldID: worldID, Characters: s.characters, Personas: s.personas,
		CharacterHasPersona:   make(map[string]bool, len(s.personaConfigured)),
		CharacterCapabilities: make(map[string]roleplay.CharacterCapabilityProjection, len(s.characters.Items)),
		CharacterNames:        make(map[string]string, len(s.names)),
	}
	for id, configured := range s.personaConfigured {
		projection.CharacterHasPersona[id] = configured
	}
	for id, name := range s.names {
		projection.CharacterNames[id] = name
	}
	for _, character := range s.characters.Items {
		projection.CharacterCapabilities[character.ID] = roleplay.CharacterCapabilityProjection{
			WorldID: worldID, CharacterID: character.ID, WebResearch: s.researchEnabled[character.ID],
		}
	}
	if s.scene == nil {
		return projection, nil
	}
	scene := *s.scene
	projection.Scene = &scene
	projection.Participants = s.participants
	if len(projection.Participants.Items) > page.Limit {
		start := page.TurnOrderOffset
		end := start + page.Limit
		if end > len(projection.Participants.Items) {
			end = len(projection.Participants.Items)
		}
		projection.Participants = roleplay.SceneParticipantPage{
			Items:   append([]roleplay.SceneParticipantProjection(nil), projection.Participants.Items[start:end]...),
			HasMore: end < len(s.participants.Items),
		}
	}
	projection.AllParticipants = append([]roleplay.SceneParticipantProjection(nil), s.allParticipants...)
	if len(projection.AllParticipants) == 0 {
		projection.AllParticipants = append([]roleplay.SceneParticipantProjection(nil), s.participants.Items...)
	}
	projection.Meters = s.meters
	projection.Inventory = s.inventory
	projection.Interactions = s.interactions
	projection.ItemTemplates = s.itemTemplates
	for _, participant := range s.participants.Items {
		if participant.CharacterID == scene.ActiveCharacterID {
			projection.ActiveCharacterName = participant.Name
		}
	}
	s.meterCharacterID = scene.ActiveCharacterID
	s.inventoryCharacterID = scene.ActiveCharacterID
	return projection, nil
}

func (s *roleplaySimulationTestStore) CreateCharacter(
	_ context.Context, worldID, name string,
) (roleplay.Character, error) {
	s.characterCreateCalls++
	id := "rpc_cccccccccccccccccccccccccccccccc"
	character := roleplay.Character{
		ID: id, WorldID: worldID, Name: name, Authority: roleplay.AuthorityFictionalCanon, CreatedAt: time.Now().UTC(),
	}
	s.characters.Items = append(s.characters.Items, roleplay.SimulationCharacterSummary{
		ID: character.ID, WorldID: character.WorldID, Name: character.Name, CreatedAt: character.CreatedAt,
	})
	s.names[id] = name
	return character, nil
}

func (s *roleplaySimulationTestStore) ProjectChannelCharacterContext(
	_ context.Context, _ string, characterID string, _ int,
) (roleplay.CharacterProjection, error) {
	return roleplay.CharacterProjection{
		Schema: roleplay.CharacterProjectionSchemaV1, Authority: roleplay.AuthorityCharacterKnowledge,
		WorldID: s.world.ID, WorldName: s.world.Name, CharacterID: characterID, CharacterName: s.names[characterID],
	}, nil
}

func (s *roleplaySimulationTestStore) ListSimulationCharactersPage(
	_ context.Context, _ string, _, offset int,
) (roleplay.SimulationCharacterPage, error) {
	s.characterOffset = offset
	return s.characters, nil
}

func (s *roleplaySimulationTestStore) ListPersonaPage(
	_ context.Context, _ string, _, offset int,
) (roleplay.PersonaPage, error) {
	s.personaOffset = offset
	return s.personas, nil
}

func (s *roleplaySimulationTestStore) ProjectCurrentScene(
	_ context.Context, _ string,
) (roleplay.SceneSheet, error) {
	if s.scene == nil {
		return roleplay.SceneSheet{}, roleplay.ErrSimulationNotConfigured
	}
	return *s.scene, nil
}

func (s *roleplaySimulationTestStore) ListSceneParticipantsPage(
	_ context.Context, _, _ string, _, offset int,
) (roleplay.SceneParticipantPage, error) {
	s.configuredPageCalls++
	s.participantOffset = offset
	return s.participants, nil
}

func (s *roleplaySimulationTestStore) ListViewpointMetersPage(
	_ context.Context, _, characterID string, _, offset int,
) (roleplay.MeterPage, error) {
	s.configuredPageCalls++
	s.meterCharacterID, s.meterOffset = characterID, offset
	return s.meters, nil
}

func (s *roleplaySimulationTestStore) ListInventoryPage(
	_ context.Context, _, characterID string, _, offset int,
) (roleplay.InventoryPage, error) {
	s.configuredPageCalls++
	s.inventoryCharacterID, s.inventoryOffset = characterID, offset
	return s.inventory, nil
}

func (s *roleplaySimulationTestStore) ListInteractionCommandsPage(
	_ context.Context, _ string, _, offset int,
) (roleplay.InteractionCommandPage, error) {
	s.configuredPageCalls++
	s.interactionOffset = offset
	return s.interactions, nil
}

func (s *roleplaySimulationTestStore) ListItemTemplatesPage(
	_ context.Context, _ string, _, offset int,
) (roleplay.ItemTemplatePage, error) {
	s.itemTemplateOffset = offset
	return s.itemTemplates, nil
}

func (s *roleplaySimulationTestStore) WritePersona(
	_ context.Context, request roleplay.PersonaWriteRequest,
) (roleplay.PersonaProjection, error) {
	s.lastPersona = request
	if s.writePersonaErr == nil {
		s.personaConfigured[request.CharacterID] = true
	}
	return roleplay.PersonaProjection{CharacterID: request.CharacterID, Revision: request.ExpectedRevision + 1, Sheet: request.Sheet}, s.writePersonaErr
}

func (s *roleplaySimulationTestStore) ProjectPersona(
	_ context.Context, characterID string,
) (roleplay.PersonaProjection, error) {
	if !s.personaConfigured[characterID] {
		return roleplay.PersonaProjection{}, roleplay.ErrSimulationNotConfigured
	}
	return roleplay.PersonaProjection{CharacterID: characterID, Revision: 1}, nil
}

func (s *roleplaySimulationTestStore) ProjectCharacterCapability(
	_ context.Context, worldID, characterID string,
) (roleplay.CharacterCapabilityProjection, error) {
	return roleplay.CharacterCapabilityProjection{
		WorldID: worldID, CharacterID: characterID, WebResearch: s.researchEnabled[characterID],
	}, nil
}

func (s *roleplaySimulationTestStore) ConfigureCharacterCapability(
	_ context.Context,
	worldID, characterID string,
	_ roleplay.CharacterCapability,
	enabled bool,
) (roleplay.CharacterCapabilityProjection, error) {
	s.capabilityConfigureCalls++
	s.researchEnabled[characterID] = enabled
	s.lastCapability = roleplay.CharacterCapabilityProjection{
		WorldID: worldID, CharacterID: characterID, WebResearch: enabled,
	}
	return s.lastCapability, nil
}

func (s *roleplaySimulationTestStore) CreateCurrentScene(
	_ context.Context, setup roleplay.SceneSetup,
) (roleplay.SceneSheet, error) {
	s.lastSceneSetup = setup
	s.scene = &roleplay.SceneSheet{
		ID: setup.ID, WorldID: setup.WorldID, Title: setup.Title, Description: setup.Description,
		Revision: 1, ActiveCharacterID: setup.ParticipantIDs[0], CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	s.participants.Items = make([]roleplay.SceneParticipantProjection, len(setup.ParticipantIDs))
	for index, characterID := range setup.ParticipantIDs {
		s.participants.Items[index] = roleplay.SceneParticipantProjection{
			CharacterID: characterID, Name: s.names[characterID], TurnPosition: index,
		}
	}
	s.allParticipants = append([]roleplay.SceneParticipantProjection(nil), s.participants.Items...)
	return *s.scene, nil
}

func (s *roleplaySimulationTestStore) UpdateCurrentScene(
	_ context.Context, update roleplay.SceneUpdate,
) (roleplay.SceneSheet, error) {
	s.lastSceneUpdate = update
	if s.sceneUpdateErr != nil {
		return roleplay.SceneSheet{}, s.sceneUpdateErr
	}
	if s.scene == nil || update.ExpectedRevision != s.scene.Revision {
		return roleplay.SceneSheet{}, roleplay.ErrSimulationStaleRevision
	}
	s.scene.Title = update.Title
	s.scene.Description = update.Description
	s.scene.Revision++
	participants := make([]roleplay.SceneParticipantProjection, len(update.ParticipantIDs))
	for index, characterID := range update.ParticipantIDs {
		participants[index] = roleplay.SceneParticipantProjection{
			CharacterID: characterID, Name: s.names[characterID], TurnPosition: index,
		}
	}
	s.participants = roleplay.SceneParticipantPage{Items: participants}
	s.allParticipants = append([]roleplay.SceneParticipantProjection(nil), participants...)
	activePresent := false
	for _, characterID := range update.ParticipantIDs {
		if characterID == s.scene.ActiveCharacterID {
			activePresent = true
			break
		}
	}
	if !activePresent {
		s.scene.ActiveCharacterID = update.ParticipantIDs[0]
	}
	return *s.scene, nil
}

func (s *roleplaySimulationTestStore) RegisterMeter(_ context.Context, definition roleplay.MeterDefinition) error {
	s.lastMeterDefinition = definition
	return nil
}

func (s *roleplaySimulationTestStore) SetCharacterMeter(
	_ context.Context, update roleplay.MeterValueUpdate,
) (roleplay.MeterProjection, error) {
	s.lastMeterUpdate = update
	return roleplay.MeterProjection{Key: update.MeterKey, Minimum: 0, Maximum: 10, Value: update.Value, Revision: update.ExpectedRevision + 1}, nil
}

func (s *roleplaySimulationTestStore) RegisterInteractionCommand(
	_ context.Context, definition roleplay.InteractionCommandDefinition,
) error {
	s.lastInteraction = definition
	return nil
}

func (s *roleplaySimulationTestStore) RegisterItemTemplate(
	_ context.Context, definition roleplay.ItemTemplateDefinition,
) error {
	s.lastItem = definition
	return nil
}
