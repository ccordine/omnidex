package api

import (
	"context"

	"github.com/gryph/omnidex/internal/roleplay"
)

// RoleplaySimulationStore is the database-backed simulation boundary consumed
// by HTTP transport. The queue repository deliberately does not implement it.
type RoleplaySimulationStore interface {
	FindWorldByChannel(ctx context.Context, channelID string) (roleplay.World, bool, error)
	ProjectSimulationUI(ctx context.Context, worldID string, page roleplay.SimulationUIPageRequest) (roleplay.SimulationUIProjection, error)
	ProjectSimulationSlashCommands(ctx context.Context, worldID string) (roleplay.SimulationSlashCommandProjection, error)
	CreateCharacter(ctx context.Context, worldID, name string) (roleplay.Character, error)
	CreateLibraryCharacter(ctx context.Context, name string) (roleplay.LibraryCharacterSummary, error)
	PlaceLibraryCharacter(ctx context.Context, worldID, libraryID string) (roleplay.Character, error)
	ListLibraryCharactersPage(ctx context.Context, selectedWorldID string, limit, offset int) (roleplay.LibraryCharacterPage, error)
	ListWorldsPage(ctx context.Context, limit, offset int) (roleplay.WorldPage, error)
	ProjectChannelCharacterContext(ctx context.Context, channelID, characterID string, limit int) (roleplay.CharacterProjection, error)
	ProjectCurrentScene(ctx context.Context, worldID string) (roleplay.SceneSheet, error)
	WritePersona(ctx context.Context, request roleplay.PersonaWriteRequest) (roleplay.PersonaProjection, error)
	ProjectPersona(ctx context.Context, characterID string) (roleplay.PersonaProjection, error)
	ProjectCharacterCapability(ctx context.Context, worldID, characterID string) (roleplay.CharacterCapabilityProjection, error)
	ConfigureCharacterCapability(ctx context.Context, worldID, characterID string, capability roleplay.CharacterCapability, enabled bool) (roleplay.CharacterCapabilityProjection, error)
	ProjectCharacterGeneration(ctx context.Context, worldID, characterID string) (roleplay.CharacterGenerationProjection, error)
	WriteCharacterGeneration(ctx context.Context, request roleplay.CharacterGenerationWriteRequest) (roleplay.CharacterGenerationProjection, error)
	CreateCurrentScene(ctx context.Context, setup roleplay.SceneSetup) (roleplay.SceneSheet, error)
	UpdateCurrentScene(ctx context.Context, update roleplay.SceneUpdate) (roleplay.SceneSheet, error)
	RegisterMeter(ctx context.Context, definition roleplay.MeterDefinition) error
	SetCharacterMeter(ctx context.Context, update roleplay.MeterValueUpdate) (roleplay.MeterProjection, error)
	RegisterInteractionCommand(ctx context.Context, definition roleplay.InteractionCommandDefinition) error
	RegisterItemTemplate(ctx context.Context, definition roleplay.ItemTemplateDefinition) error
}
