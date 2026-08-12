package cognitionreplay

import "errors"

const (
	BaseSchemaV2           = "omnidex-replay/v2"
	PrivateOverlaySchemaV2 = "omnidex-replay-private/v2"

	StructuralMappingSchemaV1       = "omnidex.replay.structural-source.v1"
	SemanticMappingSchemaV1         = "omnidex.replay.semantic-source.v1"
	AblationSemanticMappingSchemaV1 = "omnidex.replay.ablation-semantic-source.v1"
	KnowledgeStateSchemaV1          = "omnidex.replay-public-knowledge.v1"
	KnowledgeDeltaSchemaV1          = "omnidex.replay-public-knowledge-delta.v1"

	maxSources            = 1_000_000
	maxEvents             = 1_000_000
	maxCheckpoints        = 100_002
	maxBlobs              = 2_000_000
	maxPageItems          = 64
	maxPageBytes          = 8 * 1024 * 1024
	maxBlobBytes          = 2 * 1024 * 1024
	maxContainerBytes     = 512 * 1024 * 1024
	maxCheckpointInterval = 100
	maxExactBytes         = 4096

	MaxDirectBlobBytes    = maxBlobBytes
	MaxContainerBytes     = maxContainerBytes
	MaxCheckpointInterval = maxCheckpointInterval
)

var ErrSemanticMappingIncomplete = errors.New(
	"cognition replay semantic mapping is incomplete; serious execution is unavailable",
)

type SemanticStatus string

const (
	SemanticStructural SemanticStatus = "structural_only"
	// SemanticProjection is an unqualified deterministic projection. Only a
	// production-specific verifier may turn it into serious execution evidence.
	SemanticProjection SemanticStatus = "semantic_projection"
	// SemanticAblationProjection is a distinct unqualified adapter projection.
	// Generic replay verification cannot promote it to serious evidence.
	SemanticAblationProjection SemanticStatus = "ablation_semantic_projection"
)

type ContainerKind string

const (
	containerBase    ContainerKind = "public_base"
	containerPrivate ContainerKind = "private_overlay"
)

type EntryKind string

const (
	entrySourcePage     EntryKind = "source_page"
	entryEventPage      EntryKind = "event_page"
	entryCheckpointPage EntryKind = "checkpoint_page"
	entryPrivateSource  EntryKind = "private_source_page"
	entryPrivateEvent   EntryKind = "private_event_page"
	entryPrivateFrame   EntryKind = "private_frame_page"
	entryBlob           EntryKind = "blob"
)
