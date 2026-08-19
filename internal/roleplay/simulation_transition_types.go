package roleplay

import "time"

type SimulationAction struct {
	Kind        SimulationActionKind `json:"kind"`
	CommandKey  string               `json:"command_key"`
	Argument    string               `json:"argument"`
	HasArgument bool                 `json:"has_argument"`
}

type SimulationEffect struct {
	Sequence        int    `json:"sequence"`
	Kind            string `json:"kind"`
	SourceID        string `json:"source_id,omitempty"`
	CharacterID     string `json:"character_id,omitempty"`
	MeterKey        string `json:"meter_key,omitempty"`
	RequestedDelta  int    `json:"requested_delta,omitempty"`
	BeforeValue     int    `json:"before_value,omitempty"`
	AfterValue      int    `json:"after_value,omitempty"`
	InventoryItemID string `json:"inventory_item_id,omitempty"`
	RemainingUses   *int   `json:"remaining_uses,omitempty"`
}

type SimulationTransitionResult struct {
	Schema           string             `json:"schema"`
	OperationID      string             `json:"operation_id"`
	WorldID          string             `json:"world_id"`
	SceneID          string             `json:"scene_id"`
	ActorCharacterID string             `json:"actor_character_id"`
	BeforeRevision   int64              `json:"before_revision"`
	AfterRevision    int64              `json:"after_revision"`
	Action           SimulationAction   `json:"action"`
	Effects          []SimulationEffect `json:"effects"`
	NarrativeEvents  []string           `json:"narrative_events"`
	CreatedAt        time.Time          `json:"created_at"`
}

type AppliedSimulationChange struct {
	Kind            string `json:"kind"`
	CharacterID     string `json:"character_id,omitempty"`
	MeterKey        string `json:"meter_key,omitempty"`
	BeforeValue     int    `json:"before_value,omitempty"`
	AfterValue      int    `json:"after_value,omitempty"`
	InventoryItemID string `json:"inventory_item_id,omitempty"`
}

type AdminAppliedTransitionProjection struct {
	TransitionID     string                    `json:"transition_id"`
	ActorCharacterID string                    `json:"actor_character_id"`
	ExactAction      string                    `json:"exact_action"`
	BeforeRevision   int64                     `json:"before_revision"`
	AfterRevision    int64                     `json:"after_revision"`
	Changes          []AppliedSimulationChange `json:"changes"`
	CreatedAt        time.Time                 `json:"created_at"`
}
