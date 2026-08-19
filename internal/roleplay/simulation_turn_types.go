package roleplay

import "time"

type SimulationTurnAuthority struct {
	PreparationID           string                        `json:"preparation_id"`
	ChannelID               string                        `json:"channel_id"`
	UserMessageID           int64                         `json:"user_message_id"`
	WorldID                 string                        `json:"world_id"`
	SceneID                 string                        `json:"scene_id"`
	BaseSceneRevision       int64                         `json:"base_scene_revision"`
	SceneRevision           int64                         `json:"scene_revision"`
	ActiveCharacterID       string                        `json:"active_character_id"`
	InputKind               SimulationTurnInputKind       `json:"input_kind"`
	ExplicitAction          bool                          `json:"explicit_action"`
	PendingTransition       *SimulationTransitionResult   `json:"pending_transition,omitempty"`
	ParticipantCharacterIDs []string                      `json:"participant_character_ids"`
	NarrativeProjection     NarrativeSimulationProjection `json:"narrative_projection"`
	NarrativeAuthority      SimulationNarrativeAuthority  `json:"narrative_authority"`
	NarrativeFingerprint    string                        `json:"narrative_fingerprint"`
	CreatedAt               time.Time                     `json:"created_at"`
}

type SimulationTurnPreparationRequest struct {
	OperationID   string                  `json:"operation_id"`
	ChannelID     string                  `json:"channel_id"`
	UserMessageID int64                   `json:"user_message_id"`
	InputKind     SimulationTurnInputKind `json:"input_kind"`
}

type SimulationTurnAdvanceRequest struct {
	OperationID      string `json:"operation_id"`
	PreparationID    string `json:"preparation_id"`
	ChannelID        string `json:"channel_id"`
	UserMessageID    int64  `json:"user_message_id"`
	JobID            int64  `json:"job_id"`
	ExpectedRevision int64  `json:"expected_revision"`
}

type SimulationTurnMaterializationRequest struct {
	PreparationID string `json:"preparation_id"`
	ChannelID     string `json:"channel_id"`
	UserMessageID int64  `json:"user_message_id"`
	JobID         int64  `json:"job_id"`
}

type SimulationTurnAdvanceResult struct {
	OperationID             string    `json:"operation_id"`
	PreparationID           string    `json:"preparation_id"`
	WorldID                 string    `json:"world_id"`
	SceneID                 string    `json:"scene_id"`
	PreviousCharacterID     string    `json:"previous_character_id"`
	ActiveCharacterID       string    `json:"active_character_id"`
	BeforeRevision          int64     `json:"before_revision"`
	AfterRevision           int64     `json:"after_revision"`
	ParticipantCharacterIDs []string  `json:"participant_character_ids"`
	NarrativeFingerprint    string    `json:"narrative_fingerprint"`
	CreatedAt               time.Time `json:"created_at"`
}
