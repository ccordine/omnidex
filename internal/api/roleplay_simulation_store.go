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
	ProjectChannelCharacterContext(ctx context.Context, channelID, characterID string, limit int) (roleplay.CharacterProjection, error)
	ListSimulationCharactersPage(ctx context.Context, worldID string, limit, offset int) (roleplay.SimulationCharacterPage, error)
	ProjectCurrentScene(ctx context.Context, worldID string) (roleplay.SceneSheet, error)
	WritePersona(ctx context.Context, request roleplay.PersonaWriteRequest) (roleplay.PersonaProjection, error)
	ProjectPersona(ctx context.Context, characterID string) (roleplay.PersonaProjection, error)
	ProjectCharacterCapability(ctx context.Context, worldID, characterID string) (roleplay.CharacterCapabilityProjection, error)
	ConfigureCharacterCapability(ctx context.Context, worldID, characterID string, capability roleplay.CharacterCapability, enabled bool) (roleplay.CharacterCapabilityProjection, error)
	CreateCurrentScene(ctx context.Context, setup roleplay.SceneSetup) (roleplay.SceneSheet, error)
	UpdateCurrentScene(ctx context.Context, update roleplay.SceneUpdate) (roleplay.SceneSheet, error)
	RegisterMeter(ctx context.Context, definition roleplay.MeterDefinition) error
	SetCharacterMeter(ctx context.Context, update roleplay.MeterValueUpdate) (roleplay.MeterProjection, error)
	RegisterInteractionCommand(ctx context.Context, definition roleplay.InteractionCommandDefinition) error
	RegisterItemTemplate(ctx context.Context, definition roleplay.ItemTemplateDefinition) error
}
