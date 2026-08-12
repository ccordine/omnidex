package cognitiongauntlet

import (
	"github.com/gryph/omnidex/internal/cognitionreplay"
)

const ablationSemanticSourceSchemaV1 = "omnidex.ablation-semantic-source.v1"

type ablationSemanticSourcePayload struct {
	Schema             string `json:"schema"`
	EvidenceRootSHA256 string `json:"evidence_root_sha256"`
	Kind               string `json:"kind"`
	ID                 string `json:"id"`
	UnitSHA256         string `json:"unit_sha256"`
	Revision           uint64 `json:"revision,omitempty"`
}

type ablationSemanticProjectionVerification struct {
	verified cognitionreplay.VerifiedBase
	bundle   PublicInferenceBundle
	episode  SealedEpisode
	evidence ablationEvidenceArtifact
	class    AblationReplayClass
}

type ablationSemanticBuild struct {
	sources         []cognitionreplay.SourceRecord
	events          []cognitionreplay.Event
	checkpoints     []cognitionreplay.KnowledgeCheckpoint
	blobs           []cognitionreplay.Blob
	entries         map[string]cognitionreplay.KnowledgeEntry
	intervalUpserts map[string]cognitionreplay.KnowledgeEntry
}

type ablationSemanticUnit struct {
	callOrdinal int64
	phase       int
	sequence    int64
	kind        string
	id          string
	value       any
	events      []cognitionreplay.EventKind
	knowledge   []*semanticKnowledgeChange
	revision    uint64
	revisionSHA string
}
