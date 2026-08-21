package api

import (
	"context"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/jackc/pgx/v5"
)

type roleplaySimulationTestStore struct {
	world                    roleplay.World
	scene                    *roleplay.SceneSheet
	characters               roleplay.SimulationCharacterPage
	worldCharacters          []roleplay.SimulationCharacterSummary
	library                  roleplay.LibraryCharacterPage
	worlds                   roleplay.WorldPage
	personas                 roleplay.PersonaPage
	participants             roleplay.SceneParticipantPage
	allParticipants          []roleplay.SceneParticipantProjection
	meters                   roleplay.MeterPage
	inventory                roleplay.InventoryPage
	interactions             roleplay.InteractionCommandPage
	itemTemplates            roleplay.ItemTemplatePage
	names                    map[string]string
	characterWorldIDs        map[string]string
	personaConfigured        map[string]bool
	researchEnabled          map[string]bool
	generation               map[string]roleplay.CharacterGenerationProjection
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
	libraryWorldID           string
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
		names: make(map[string]string), characterWorldIDs: make(map[string]string),
		personaConfigured: make(map[string]bool),
		researchEnabled:   make(map[string]bool),
		generation:        make(map[string]roleplay.CharacterGenerationProjection),
	}
}

func testRoleplayLibraryID(characterID string) string {
	return "rpl_" + strings.TrimPrefix(characterID, "rpc_")
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
		CharacterGeneration:   make(map[string]roleplay.CharacterGenerationProjection, len(s.worldCharacters)),
		CharacterNames:        make(map[string]string, len(s.names)),
	}
	for id, configured := range s.personaConfigured {
		projection.CharacterHasPersona[id] = configured
	}
	for id, name := range s.names {
		projection.CharacterNames[id] = name
	}
	for index := range s.characters.Items {
		character := s.characters.Items[index]
		if character.LibraryID == "" {
			character.LibraryID = testRoleplayLibraryID(character.ID)
			s.characters.Items[index] = character
		}
		projection.CharacterCapabilities[character.ID] = roleplay.CharacterCapabilityProjection{
			WorldID: worldID, CharacterID: character.ID, WebResearch: s.researchEnabled[character.ID],
		}
	}
	projection.UserPersonaCharacters = make([]roleplay.SimulationCharacterSummary, len(s.worldCharacters))
	for index, character := range s.worldCharacters {
		if character.LibraryID == "" {
			character.LibraryID = testRoleplayLibraryID(character.ID)
			s.worldCharacters[index] = character
		}
		projection.UserPersonaCharacters[index] = character
		generation, exists := s.generation[character.ID]
		if !exists {
			generation = roleplay.CharacterGenerationProjection{
				CharacterID: character.ID,
				Config: roleplay.CharacterGenerationConfig{
					Schema:             roleplay.CharacterGenerationConfigSchemaV2,
					LibraryCharacterID: character.LibraryID,
					Revision:           1,
				},
				UpdatedAt: time.Now().UTC(),
			}
			s.generation[character.ID] = generation
		}
		projection.CharacterGeneration[character.ID] = generation
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
	activeGeneration := s.generation[scene.ActiveCharacterID]
	projection.ActiveGeneration = &activeGeneration
	s.meterCharacterID = scene.ActiveCharacterID
	s.inventoryCharacterID = scene.ActiveCharacterID
	return projection, nil
}

func (s *roleplaySimulationTestStore) CreateCharacter(
	_ context.Context, worldID, name string,
) (roleplay.Character, error) {
	s.characterCreateCalls++
	id := "rpc_cccccccccccccccccccccccccccccccc"
	libraryID := "rpl_cccccccccccccccccccccccccccccccc"
	character := roleplay.Character{
		ID: id, WorldID: worldID, LibraryID: libraryID, Name: name,
		Authority: roleplay.AuthorityFictionalCanon, CreatedAt: time.Now().UTC(),
	}
	s.characters.Items = append(s.characters.Items, roleplay.SimulationCharacterSummary{
		ID: character.ID, WorldID: character.WorldID, LibraryID: character.LibraryID,
		Name: character.Name, CreatedAt: character.CreatedAt,
	})
	s.worldCharacters = append(s.worldCharacters, s.characters.Items[len(s.characters.Items)-1])
	s.names[id] = name
	return character, nil
}

func (s *roleplaySimulationTestStore) CreateLibraryCharacter(
	_ context.Context, name string,
) (roleplay.LibraryCharacterSummary, error) {
	character := roleplay.LibraryCharacterSummary{
		ID: "rpl_dddddddddddddddddddddddddddddddd", Name: name,
		Authority: roleplay.AuthorityCharacterIdentity, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	s.library.Items = append(s.library.Items, character)
	return character, nil
}

func (s *roleplaySimulationTestStore) PlaceLibraryCharacter(
	_ context.Context, worldID, libraryID string,
) (roleplay.Character, error) {
	return roleplay.Character{
		ID: "rpc_eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee", WorldID: worldID, LibraryID: libraryID,
		Name: "Portable", Authority: roleplay.AuthorityFictionalCanon, CreatedAt: time.Now().UTC(),
	}, nil
}

func (s *roleplaySimulationTestStore) ListLibraryCharactersPage(
	_ context.Context, selectedWorldID string, _, offset int,
) (roleplay.LibraryCharacterPage, error) {
	s.libraryWorldID = selectedWorldID
	s.library.Offset = offset
	s.library.SelectedWorldID = selectedWorldID
	return s.library, nil
}

func (s *roleplaySimulationTestStore) ListWorldsPage(
	_ context.Context, _, offset int,
) (roleplay.WorldPage, error) {
	s.worlds.Offset = offset
	if len(s.worlds.Items) == 0 {
		s.worlds.Items = []roleplay.WorldSummary{{World: s.world, CharacterCount: 1}}
	}
	return s.worlds, nil
}

func (s *roleplaySimulationTestStore) ProjectChannelCharacterContext(
	_ context.Context, _ string, characterID string, _ int,
) (roleplay.CharacterProjection, error) {
	name, exists := s.names[characterID]
	if !exists {
		return roleplay.CharacterProjection{}, pgx.ErrNoRows
	}
	worldID := s.world.ID
	if selected := s.characterWorldIDs[characterID]; selected != "" {
		if selected != s.world.ID {
			return roleplay.CharacterProjection{}, pgx.ErrNoRows
		}
		worldID = selected
	}
	projection := roleplay.CharacterProjection{
		Schema: roleplay.CharacterProjectionSchemaV1, Authority: roleplay.AuthorityCharacterKnowledge,
		WorldID: worldID, WorldName: s.world.Name, CharacterID: characterID, CharacterName: name,
		Facts: []roleplay.ContextFact{},
	}
	fingerprint, err := roleplay.ExactCharacterProjectionFingerprint(projection)
	if err != nil {
		return roleplay.CharacterProjection{}, err
	}
	projection.Fingerprint = fingerprint
	return projection, nil
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
	for _, projection := range s.personas.Items {
		if projection.CharacterID == characterID {
			return projection, nil
		}
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

func (s *roleplaySimulationTestStore) ProjectCharacterGeneration(
	_ context.Context, _ string, characterID string,
) (roleplay.CharacterGenerationProjection, error) {
	return s.generation[characterID], nil
}

func (s *roleplaySimulationTestStore) WriteCharacterGeneration(
	_ context.Context, request roleplay.CharacterGenerationWriteRequest,
) (roleplay.CharacterGenerationProjection, error) {
	current := s.generation[request.CharacterID]
	current.CharacterID = request.CharacterID
	current.Config.Schema = roleplay.CharacterGenerationConfigSchemaV2
	if current.Config.LibraryCharacterID == "" {
		current.Config.LibraryCharacterID = "rpl_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	}
	current.Config.Revision = request.ExpectedRevision + 1
	current.Config.NarrativeModel = request.NarrativeModel
	current.UpdatedAt = time.Now().UTC()
	s.generation[request.CharacterID] = current
	return current, nil
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
