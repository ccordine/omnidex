package cognitionreplay

import (
	"fmt"
	"time"
)

type BaseInput struct {
	TerminalAuthority     TerminalAuthority
	PublicWorldSHA256     string
	PublicWorldSchema     string
	PublicAuthoritySHA256 string
	Sources               []SourceRecord
	Events                []Event
	Checkpoints           []KnowledgeCheckpoint
	ChunkedBlobs          []ChunkedBlobBinding
	Blobs                 []Blob
}

type SourceRecord struct {
	Ordinal     uint64  `json:"ordinal"`
	CallOrdinal int64   `json:"call_ordinal"`
	Phase       int     `json:"phase"`
	Sequence    int64   `json:"sequence"`
	Kind        string  `json:"kind"`
	ID          string  `json:"id"`
	Payload     BlobRef `json:"payload"`
}

type SourceRef struct {
	Ordinal       uint64 `json:"ordinal"`
	Kind          string `json:"kind"`
	ID            string `json:"id"`
	PayloadSHA256 string `json:"payload_sha256"`
}

func (record SourceRecord) Ref() SourceRef {
	return SourceRef{
		Ordinal: record.Ordinal, Kind: record.Kind, ID: record.ID,
		PayloadSHA256: record.Payload.SHA256,
	}
}

func (record SourceRecord) Validate() error {
	if record.Ordinal == 0 || record.CallOrdinal < 0 || record.Phase < 1 || record.Phase > 100 ||
		record.Sequence < 0 || requireExact(record.Kind, "source record kind") != nil ||
		requireExact(record.ID, "source record ID") != nil || record.Payload.Validate() != nil {
		return fmt.Errorf("replay source record authority is invalid")
	}
	return nil
}

func (ref SourceRef) Validate() error {
	if ref.Ordinal == 0 || requireExact(ref.Kind, "source reference kind") != nil ||
		requireExact(ref.ID, "source reference ID") != nil || !validDigest(ref.PayloadSHA256) {
		return fmt.Errorf("replay source reference authority is invalid")
	}
	return nil
}

func sourceRecordLess(left, right SourceRecord) bool {
	if left.CallOrdinal != right.CallOrdinal {
		return left.CallOrdinal < right.CallOrdinal
	}
	if left.Phase != right.Phase {
		return left.Phase < right.Phase
	}
	if left.Sequence != right.Sequence {
		return left.Sequence < right.Sequence
	}
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return left.ID < right.ID
}

type EventKind string

const (
	EventWorldStarted               EventKind = "world.started"
	EventGoalActivated              EventKind = "goal.activated"
	EventObservationAcquired        EventKind = "observation.acquired"
	EventEvidenceAcquired           EventKind = "evidence.acquired"
	EventFactAccepted               EventKind = "fact.accepted"
	EventWorkingSetAttached         EventKind = "working_set.attached"
	EventWorkingSetReleased         EventKind = "working_set.released"
	EventWorkingSetReacquired       EventKind = "working_set.reacquired"
	EventWorkingSetRetained         EventKind = "working_set.retained"
	EventWorkingSetTouched          EventKind = "working_set.touched"
	EventWorkingSetInvalidated      EventKind = "working_set.invalidated"
	EventWorkingSetScopeClosed      EventKind = "working_set.scope_closed"
	EventWorkingSetSnapshot         EventKind = "working_set.snapshot"
	EventHypothesisCreated          EventKind = "hypothesis.created"
	EventHypothesisRejected         EventKind = "hypothesis.rejected"
	EventDecisionAccepted           EventKind = "decision.accepted"
	EventDecisionRejected           EventKind = "decision.rejected"
	EventActionSelected             EventKind = "action.selected"
	EventWorldTransition            EventKind = "world.transition"
	EventObligationCreated          EventKind = "obligation.created"
	EventObligationChanged          EventKind = "obligation.changed"
	EventPlanRevised                EventKind = "plan.revised"
	EventContextProjected           EventKind = "context.projected"
	EventModelCalled                EventKind = "model.called"
	EventModelCallDisposition       EventKind = "model.call_disposition"
	EventProviderRequestDisposition EventKind = "provider.request_disposition"
	EventProviderProcessObserved    EventKind = "provider.process_observed"
	EventFailureRecorded            EventKind = "failure.recorded"
	EventEpisodeRestarted           EventKind = "episode.restarted"
	EventLeaseChanged               EventKind = "lease.changed"
	EventStaleWriteRejected         EventKind = "stale_write.rejected"
	EventGoalSatisfied              EventKind = "goal.satisfied"
	EventGoalFailed                 EventKind = "goal.failed"
	EventEpisodeCanceled            EventKind = "episode.canceled"
	EventEpisodeSealed              EventKind = "episode.sealed"
)

type Event struct {
	Sequence       uint64          `json:"sequence"`
	Kind           EventKind       `json:"kind"`
	MappingSchema  string          `json:"mapping_schema"`
	Revision       *PublicRevision `json:"revision,omitempty"`
	Timing         *EventTiming    `json:"timing,omitempty"`
	Sources        []SourceRef     `json:"sources"`
	Payload        BlobRef         `json:"payload"`
	PreviousSHA256 string          `json:"previous_sha256,omitempty"`
	EventSHA256    string          `json:"event_sha256"`
}

// EventTiming is optional because the sealed trace has no universal per-record
// timestamp. A semantic adapter may populate it only from one cited typed
// source timestamp. Structural events always omit it.
type EventTiming struct {
	Timestamp          time.Time `json:"timestamp"`
	ElapsedNanoseconds int64     `json:"elapsed_nanoseconds"`
	Source             SourceRef `json:"source"`
}

func validPublicEventKind(kind EventKind) bool {
	switch kind {
	case EventWorldStarted, EventGoalActivated, EventObservationAcquired, EventEvidenceAcquired,
		EventFactAccepted, EventWorkingSetAttached, EventWorkingSetReleased,
		EventWorkingSetReacquired, EventWorkingSetRetained, EventWorkingSetTouched,
		EventWorkingSetInvalidated, EventWorkingSetScopeClosed, EventWorkingSetSnapshot,
		EventHypothesisCreated, EventHypothesisRejected,
		EventDecisionAccepted, EventDecisionRejected, EventActionSelected, EventWorldTransition,
		EventObligationCreated, EventObligationChanged, EventPlanRevised, EventContextProjected,
		EventModelCalled, EventModelCallDisposition, EventProviderRequestDisposition,
		EventProviderProcessObserved, EventFailureRecorded,
		EventEpisodeRestarted, EventLeaseChanged, EventStaleWriteRejected, EventGoalSatisfied,
		EventGoalFailed, EventEpisodeCanceled, EventEpisodeSealed:
		return true
	default:
		return false
	}
}

type ContainerEntry struct {
	Path        string    `json:"path"`
	Kind        EntryKind `json:"kind"`
	SHA256      string    `json:"sha256"`
	ByteCount   int64     `json:"byte_count"`
	First       uint64    `json:"first,omitempty"`
	Last        uint64    `json:"last,omitempty"`
	RecordCount int       `json:"record_count,omitempty"`
}

type MappingDisposition string

const (
	MappingStructuralOpaque MappingDisposition = "structural_opaque"
	MappingSemanticTyped    MappingDisposition = "semantic_typed"
	MappingSemanticOpaque   MappingDisposition = "semantic_opaque"
)

type SourceMapping struct {
	SourceKind    string             `json:"source_kind"`
	MappingSchema string             `json:"mapping_schema"`
	Disposition   MappingDisposition `json:"disposition"`
	EventKinds    []EventKind        `json:"event_kinds"`
}

type BaseManifest struct {
	Schema                      string                       `json:"schema"`
	Container                   ContainerKind                `json:"container"`
	SemanticStatus              SemanticStatus               `json:"semantic_status"`
	PrivateData                 bool                         `json:"private_data"`
	TerminalAuthority           TerminalAuthority            `json:"terminal_authority"`
	TerminalAuthoritySHA256     string                       `json:"terminal_authority_sha256"`
	PublicWorldSHA256           string                       `json:"public_world_sha256"`
	PublicWorldSchema           string                       `json:"public_world_schema"`
	PublicAuthoritySHA256       string                       `json:"public_authority_sha256"`
	ProjectionAuthority         *ProjectionAuthority         `json:"projection_authority,omitempty"`
	AblationProjectionAuthority *AblationProjectionAuthority `json:"ablation_projection_authority,omitempty"`
	SourceCount                 int                          `json:"source_count"`
	EventCount                  int                          `json:"event_count"`
	CheckpointCount             int                          `json:"checkpoint_count"`
	ChunkedBlobCount            int                          `json:"chunked_blob_count"`
	BlobCount                   int                          `json:"blob_count"`
	SourceIndexSHA256           string                       `json:"source_index_sha256"`
	EventIndexSHA256            string                       `json:"event_index_sha256"`
	CheckpointIndexSHA256       string                       `json:"checkpoint_index_sha256"`
	ChunkedBlobIndexSHA256      string                       `json:"chunked_blob_index_sha256"`
	SourceMappings              []SourceMapping              `json:"source_mappings"`
	ChunkedBlobs                []ChunkedBlobBinding         `json:"chunked_blobs"`
	Entries                     []ContainerEntry             `json:"entries"`
}

type Artifact struct {
	Bytes  []byte
	SHA256 string
}

type VerifiedBase struct {
	manifest    BaseManifest
	sha256      string
	sources     []SourceRecord
	events      []Event
	checkpoints []KnowledgeCheckpoint
	blobs       map[string]Blob
}

func (verified VerifiedBase) Manifest() BaseManifest { return cloneBaseManifest(verified.manifest) }
func (verified VerifiedBase) SHA256() string         { return verified.sha256 }

func (verified VerifiedBase) Sources() []SourceRecord { return cloneSourceRecords(verified.sources) }
func (verified VerifiedBase) Events() []Event         { return cloneEvents(verified.events) }
func (verified VerifiedBase) Checkpoints() []KnowledgeCheckpoint {
	return cloneKnowledgeCheckpoints(verified.checkpoints)
}
func (verified VerifiedBase) Blob(ref BlobRef) ([]byte, bool) {
	blob, exists := verified.blobs[ref.SHA256]
	if !exists || !ref.matches(blob) {
		return nil, false
	}
	return append([]byte(nil), blob.Data...), true
}

func cloneBaseManifest(value BaseManifest) BaseManifest {
	value.TerminalAuthority = cloneTerminalAuthority(value.TerminalAuthority)
	if value.ProjectionAuthority != nil {
		copy := *value.ProjectionAuthority
		copy.PublicBundle = cloneProjectionContent(copy.PublicBundle)
		copy.SealedEpisode = cloneProjectionContent(copy.SealedEpisode)
		copy.ProductionTrace = cloneProjectionContent(copy.ProductionTrace)
		copy.Sidecars = cloneProjectionSidecars(copy.Sidecars)
		value.ProjectionAuthority = &copy
	}
	value.AblationProjectionAuthority = cloneAblationProjectionAuthority(
		value.AblationProjectionAuthority,
	)
	value.SourceMappings = cloneSourceMappings(value.SourceMappings)
	value.ChunkedBlobs = cloneChunkedBlobBindings(value.ChunkedBlobs)
	value.Entries = append([]ContainerEntry(nil), value.Entries...)
	return value
}

func cloneSourceMappings(values []SourceMapping) []SourceMapping {
	result := make([]SourceMapping, len(values))
	for index, value := range values {
		value.EventKinds = append([]EventKind(nil), value.EventKinds...)
		result[index] = value
	}
	return result
}
