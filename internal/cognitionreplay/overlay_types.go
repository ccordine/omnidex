package cognitionreplay

import "fmt"

type PrivateSourceKind string

const (
	PrivateSourceOracle     PrivateSourceKind = "private_oracle"
	PrivateSourceEvaluation PrivateSourceKind = "private_evaluation"
	PrivateSourceWorld      PrivateSourceKind = "private_world"
)

type PrivateEventKind string

const (
	PrivateEventWorldTruth PrivateEventKind = "world.truth"
	PrivateEventEvaluation PrivateEventKind = "evaluation.attached"
)

type PrivateSource struct {
	Ordinal uint64            `json:"ordinal"`
	Kind    PrivateSourceKind `json:"kind"`
	ID      string            `json:"id"`
	Payload BlobRef           `json:"payload"`
}

type PrivateSourceRef struct {
	Ordinal       uint64            `json:"ordinal"`
	Kind          PrivateSourceKind `json:"kind"`
	ID            string            `json:"id"`
	PayloadSHA256 string            `json:"payload_sha256"`
}

func (source PrivateSource) Ref() PrivateSourceRef {
	return PrivateSourceRef{
		Ordinal: source.Ordinal, Kind: source.Kind, ID: source.ID,
		PayloadSHA256: source.Payload.SHA256,
	}
}

type PrivateEvent struct {
	Sequence       uint64             `json:"sequence"`
	Kind           PrivateEventKind   `json:"kind"`
	Sources        []PrivateSourceRef `json:"sources"`
	Payload        BlobRef            `json:"payload"`
	PreviousSHA256 string             `json:"previous_sha256,omitempty"`
	EventSHA256    string             `json:"event_sha256"`
}

type PrivateFrame struct {
	Sequence       uint64   `json:"sequence"`
	AfterEvent     uint64   `json:"after_event"`
	PreviousSHA256 string   `json:"previous_sha256,omitempty"`
	Snapshot       BlobRef  `json:"snapshot"`
	Delta          *BlobRef `json:"delta,omitempty"`
	FrameSHA256    string   `json:"frame_sha256"`
}

type PrivateOverlayInput struct {
	BaseReplaySHA256        string
	TerminalAuthoritySHA256 string
	OracleSHA256            string
	EvaluationSHA256        string
	Sources                 []PrivateSource
	Events                  []PrivateEvent
	Frames                  []PrivateFrame
	ChunkedBlobs            []ChunkedBlobBinding
	Blobs                   []Blob
}

type PrivateOverlayManifest struct {
	Schema                  string               `json:"schema"`
	Container               ContainerKind        `json:"container"`
	BaseReplaySHA256        string               `json:"base_replay_sha256"`
	TerminalAuthoritySHA256 string               `json:"terminal_authority_sha256"`
	OracleSHA256            string               `json:"oracle_sha256"`
	EvaluationSHA256        string               `json:"evaluation_sha256"`
	SourceCount             int                  `json:"source_count"`
	EventCount              int                  `json:"event_count"`
	FrameCount              int                  `json:"frame_count"`
	ChunkedBlobCount        int                  `json:"chunked_blob_count"`
	BlobCount               int                  `json:"blob_count"`
	SourceIndexSHA256       string               `json:"source_index_sha256"`
	EventIndexSHA256        string               `json:"event_index_sha256"`
	FrameIndexSHA256        string               `json:"frame_index_sha256"`
	ChunkedBlobIndexSHA256  string               `json:"chunked_blob_index_sha256"`
	ChunkedBlobs            []ChunkedBlobBinding `json:"chunked_blobs"`
	Entries                 []ContainerEntry     `json:"entries"`
}

type VerifiedPrivateOverlay struct {
	manifest PrivateOverlayManifest
	sha256   string
}

func (verified VerifiedPrivateOverlay) Manifest() PrivateOverlayManifest {
	value := verified.manifest
	value.ChunkedBlobs = cloneChunkedBlobBindings(value.ChunkedBlobs)
	value.Entries = append([]ContainerEntry(nil), value.Entries...)
	return value
}

func (verified VerifiedPrivateOverlay) SHA256() string { return verified.sha256 }

func validPrivateSourceKind(value PrivateSourceKind) bool {
	switch value {
	case PrivateSourceOracle, PrivateSourceEvaluation, PrivateSourceWorld:
		return true
	default:
		return false
	}
}

func validPrivateEventKind(value PrivateEventKind) bool {
	return value == PrivateEventWorldTruth || value == PrivateEventEvaluation
}

func (source PrivateSource) Validate() error {
	if source.Ordinal == 0 || !validPrivateSourceKind(source.Kind) ||
		requireExact(source.ID, "private replay source ID") != nil || source.Payload.Validate() != nil {
		return fmt.Errorf("private replay source authority is invalid")
	}
	return nil
}

func (ref PrivateSourceRef) Validate() error {
	if ref.Ordinal == 0 || !validPrivateSourceKind(ref.Kind) ||
		requireExact(ref.ID, "private replay source reference ID") != nil ||
		!validDigest(ref.PayloadSHA256) {
		return fmt.Errorf("private replay source reference is invalid")
	}
	return nil
}
