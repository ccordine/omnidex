package roleplay

import "time"

const (
	CharacterProjectionSchemaV1   = "omnidex.roleplay-character-context.v1"
	CanonProjectionSchemaV1       = "omnidex.roleplay-canon-context.v1"
	MaxProjectionEvents           = 16
	MaxCanonEventBytes            = 512
	MaxProjectionContentBytes     = 8 * 1024
	MaxCanonFactsPerTurn          = 8
	MaxKnowledgeRecipientsPerTurn = 16
)

type AuthorityNamespace string

const (
	AuthorityFictionalCanon     AuthorityNamespace = "FICTIONAL_CANON"
	AuthorityCharacterKnowledge AuthorityNamespace = "CHARACTER_KNOWLEDGE"
)

type World struct {
	ID        string
	ChannelID string
	Name      string
	Authority AuthorityNamespace
	CreatedAt time.Time
}

type Character struct {
	ID        string
	WorldID   string
	Name      string
	Authority AuthorityNamespace
	CreatedAt time.Time
}

type CanonEvent struct {
	ID              string
	WorldID         string
	SourceMessageID int64
	Ordinal         int64
	Content         string
	Authority       AuthorityNamespace
	CreatedAt       time.Time
}

type KnowledgeGrant struct {
	ID          string
	WorldID     string
	CharacterID string
	EventID     string
	Authority   AuthorityNamespace
	CreatedAt   time.Time
}

type ContextFact struct {
	EventID string `json:"event_id"`
	Content string `json:"content"`
}

type CharacterProjection struct {
	Schema        string             `json:"schema"`
	Authority     AuthorityNamespace `json:"authority"`
	WorldID       string             `json:"world_id"`
	WorldName     string             `json:"world_name"`
	CharacterID   string             `json:"character_id"`
	CharacterName string             `json:"character_name"`
	Facts         []ContextFact      `json:"facts"`
	Fingerprint   string             `json:"fingerprint"`
}

type CanonProjection struct {
	Schema      string             `json:"schema"`
	Authority   AuthorityNamespace `json:"authority"`
	WorldID     string             `json:"world_id"`
	WorldName   string             `json:"world_name"`
	Facts       []ContextFact      `json:"facts"`
	Fingerprint string             `json:"fingerprint"`
}

type projectedEvent struct {
	ID      string
	content string
}

func CloneCharacterProjection(value CharacterProjection) CharacterProjection {
	value.Facts = append([]ContextFact(nil), value.Facts...)
	return value
}
