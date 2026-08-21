package roleplay

import "time"

const (
	SimulationAdminProjectionSchemaV1     = "omnidex.roleplay-simulation-admin.v1"
	NarrativeSimulationProjectionSchemaV1 = "omnidex.roleplay-simulation-narrative.v1"
	SimulationTransitionSchemaV1          = "omnidex.roleplay-simulation-transition.v1"
	MaxSceneParticipants                  = 16
	MaxSimulationMeters                   = 16
	MaxInteractionCommands                = 32
	MaxWorldItemTemplates                 = 64
	MaxInventoryItems                     = 64
	MaxDefinitionEffects                  = 8
	MaxTransitionEffects                  = 32
	MaxSimulationHistory                  = 16
	MaxPersonaListEntries                 = 16
	MaxSimulationTextBytes                = 1024
)

const AuthorityCharacterMemory AuthorityNamespace = "CHARACTER_MEMORY"

type CommandArgumentMode string

const (
	CommandArgumentNone     CommandArgumentMode = "none"
	CommandArgumentRequired CommandArgumentMode = "required"
)

type ThresholdDirection string

const (
	ThresholdAtOrBelow ThresholdDirection = "at_or_below"
	ThresholdAtOrAbove ThresholdDirection = "at_or_above"
)

type ItemUsePolicy string

const (
	ItemUseFinite   ItemUsePolicy = "finite"
	ItemUseInfinite ItemUsePolicy = "infinite"
)

type SimulationActionKind string

const (
	SimulationActionGive        SimulationActionKind = "give"
	SimulationActionTake        SimulationActionKind = "take"
	SimulationActionInteraction SimulationActionKind = "interaction"
	SimulationActionAutomatic   SimulationActionKind = "automatic"
)

type SimulationTurnInputKind string

const (
	SimulationTurnProse           SimulationTurnInputKind = "prose"
	SimulationTurnAction          SimulationTurnInputKind = "simulation_action"
	SimulationTurnExternalCommand SimulationTurnInputKind = "external_command"
)

type PersonaSheet struct {
	Summary string   `json:"summary"`
	Voice   string   `json:"voice"`
	Traits  []string `json:"traits"`
	Goals   []string `json:"goals"`
}

type PersonaProjection struct {
	CharacterID string       `json:"character_id"`
	Revision    int64        `json:"revision"`
	Sheet       PersonaSheet `json:"sheet"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

type PersonaWriteRequest struct {
	CharacterID      string       `json:"character_id"`
	ExpectedRevision int64        `json:"expected_revision"`
	Sheet            PersonaSheet `json:"sheet"`
}

type SceneSetup struct {
	ID             string   `json:"id"`
	WorldID        string   `json:"world_id"`
	Title          string   `json:"title"`
	Description    string   `json:"description"`
	ParticipantIDs []string `json:"participant_ids"`
}

type SceneSheet struct {
	ID                string    `json:"id"`
	WorldID           string    `json:"world_id"`
	Title             string    `json:"title"`
	Description       string    `json:"description"`
	Revision          int64     `json:"revision"`
	ActiveCharacterID string    `json:"active_character_id"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type SceneUpdate struct {
	WorldID          string   `json:"world_id"`
	SceneID          string   `json:"scene_id"`
	ExpectedRevision int64    `json:"expected_revision"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	ParticipantIDs   []string `json:"participant_ids"`
}

type MeterDelta struct {
	MeterKey string `json:"meter_key"`
	Delta    int    `json:"delta"`
}

type MeterDefinition struct {
	WorldID      string `json:"world_id"`
	Key          string `json:"key"`
	Name         string `json:"name"`
	Minimum      int    `json:"minimum"`
	Maximum      int    `json:"maximum"`
	InitialValue int    `json:"initial_value"`
}

type MeterProjection struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Minimum  int    `json:"minimum"`
	Maximum  int    `json:"maximum"`
	Value    int    `json:"value"`
	Revision int64  `json:"revision"`
}

type MeterValueUpdate struct {
	WorldID          string `json:"world_id"`
	CharacterID      string `json:"character_id"`
	MeterKey         string `json:"meter_key"`
	ExpectedRevision int64  `json:"expected_revision"`
	Value            int    `json:"value"`
}

type InteractionCommandDefinition struct {
	ID           string              `json:"id"`
	WorldID      string              `json:"world_id"`
	Key          string              `json:"key"`
	Name         string              `json:"name"`
	Description  string              `json:"description"`
	ArgumentMode CommandArgumentMode `json:"argument_mode"`
	Effects      []MeterDelta        `json:"effects"`
}

type ItemTrigger struct {
	MeterKey  string             `json:"meter_key"`
	Direction ThresholdDirection `json:"direction"`
	Threshold int                `json:"threshold"`
}

type ItemTemplateDefinition struct {
	ID          string        `json:"id"`
	WorldID     string        `json:"world_id"`
	Name        string        `json:"name"`
	Description string        `json:"description"`
	UsePolicy   ItemUsePolicy `json:"use_policy"`
	InitialUses int           `json:"initial_uses"`
	Trigger     *ItemTrigger  `json:"trigger"`
	Priority    int           `json:"priority"`
	Effects     []MeterDelta  `json:"effects"`
}

type InventoryItemProjection struct {
	ID            string        `json:"id"`
	TemplateID    string        `json:"template_id"`
	Name          string        `json:"name"`
	Description   string        `json:"description"`
	UsePolicy     ItemUsePolicy `json:"use_policy"`
	RemainingUses int           `json:"remaining_uses"`
}

type SceneParticipantProjection struct {
	CharacterID  string `json:"character_id"`
	Name         string `json:"name"`
	TurnPosition int    `json:"turn_position"`
}

type SimulationCharacterSummary struct {
	ID        string    `json:"id"`
	WorldID   string    `json:"world_id"`
	LibraryID string    `json:"library_id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
}

type CharacterMemory struct {
	ID            string    `json:"id"`
	SourceEventID string    `json:"source_event_id"`
	Content       string    `json:"content"`
	CreatedAt     time.Time `json:"created_at"`
}
