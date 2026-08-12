package cognitionreplay

import "fmt"

type PublicRevision struct {
	Number uint64 `json:"number"`
	SHA256 string `json:"sha256"`
}

func (revision PublicRevision) Validate() error {
	if revision.Number == 0 || !validDigest(revision.SHA256) {
		return fmt.Errorf("public replay revision is invalid")
	}
	return nil
}

type KnowledgeKind string

const (
	KnowledgeGoal        KnowledgeKind = "goal"
	KnowledgeObservation KnowledgeKind = "observation"
	KnowledgeEvidence    KnowledgeKind = "evidence"
	KnowledgeBelief      KnowledgeKind = "belief"
	KnowledgeObligation  KnowledgeKind = "obligation"
	KnowledgeWorkingSet  KnowledgeKind = "working_set"
	KnowledgeProjection  KnowledgeKind = "projection"
	KnowledgeFailure     KnowledgeKind = "failure"
)

type KnowledgeStatus string

const (
	KnowledgePending    KnowledgeStatus = "pending"
	KnowledgeReady      KnowledgeStatus = "ready"
	KnowledgeActive     KnowledgeStatus = "active"
	KnowledgeBlocked    KnowledgeStatus = "blocked"
	KnowledgeSatisfied  KnowledgeStatus = "satisfied"
	KnowledgeRejected   KnowledgeStatus = "rejected"
	KnowledgeReleased   KnowledgeStatus = "released"
	KnowledgeResolved   KnowledgeStatus = "resolved"
	KnowledgeSuperseded KnowledgeStatus = "superseded"
	KnowledgeFailed     KnowledgeStatus = "failed"
	KnowledgeStale      KnowledgeStatus = "stale"
)

type KnowledgeAuthority string

const (
	AuthorityUser             KnowledgeAuthority = "user"
	AuthorityCode             KnowledgeAuthority = "code"
	AuthorityTool             KnowledgeAuthority = "tool_evidence"
	AuthorityEnvironment      KnowledgeAuthority = "environment_observation"
	AuthorityModelProposal    KnowledgeAuthority = "model_proposal"
	AuthorityAcceptedDecision KnowledgeAuthority = "accepted_model_decision"
)

type KnowledgeEntry struct {
	Kind         KnowledgeKind      `json:"kind"`
	Ref          string             `json:"ref"`
	Status       KnowledgeStatus    `json:"status"`
	Authority    KnowledgeAuthority `json:"authority"`
	Content      BlobRef            `json:"content"`
	SourceEvents []uint64           `json:"source_events"`
}

type KnowledgeRelease struct {
	Kind        KnowledgeKind `json:"kind"`
	Ref         string        `json:"ref"`
	SourceEvent uint64        `json:"source_event"`
}

type KnowledgeState struct {
	Schema   string           `json:"schema"`
	Revision *PublicRevision  `json:"revision,omitempty"`
	Entries  []KnowledgeEntry `json:"entries"`
}

type KnowledgeDelta struct {
	Schema       string             `json:"schema"`
	FromEvent    uint64             `json:"from_event"`
	ThroughEvent uint64             `json:"through_event"`
	SetRevision  *PublicRevision    `json:"set_revision,omitempty"`
	Upserts      []KnowledgeEntry   `json:"upserts"`
	Releases     []KnowledgeRelease `json:"releases"`
}

type KnowledgeCheckpoint struct {
	Sequence         uint64          `json:"sequence"`
	AfterEvent       uint64          `json:"after_event"`
	PreviousSHA256   string          `json:"previous_sha256,omitempty"`
	State            KnowledgeState  `json:"state"`
	StateSHA256      string          `json:"state_sha256"`
	Delta            *KnowledgeDelta `json:"delta,omitempty"`
	DeltaSHA256      string          `json:"delta_sha256,omitempty"`
	CheckpointSHA256 string          `json:"checkpoint_sha256"`
}

func validKnowledgeKind(value KnowledgeKind) bool {
	switch value {
	case KnowledgeGoal, KnowledgeObservation, KnowledgeEvidence, KnowledgeBelief,
		KnowledgeObligation, KnowledgeWorkingSet, KnowledgeProjection, KnowledgeFailure:
		return true
	default:
		return false
	}
}

func validKnowledgeStatus(value KnowledgeStatus) bool {
	switch value {
	case KnowledgePending, KnowledgeReady, KnowledgeActive, KnowledgeBlocked,
		KnowledgeSatisfied, KnowledgeRejected, KnowledgeReleased, KnowledgeResolved,
		KnowledgeSuperseded, KnowledgeFailed, KnowledgeStale:
		return true
	default:
		return false
	}
}

func validKnowledgeAuthority(value KnowledgeAuthority) bool {
	switch value {
	case AuthorityUser, AuthorityCode, AuthorityTool, AuthorityEnvironment,
		AuthorityModelProposal, AuthorityAcceptedDecision:
		return true
	default:
		return false
	}
}

func knowledgeKey(kind KnowledgeKind, ref string) string { return string(kind) + "\x00" + ref }

func knowledgeEntryLess(left, right KnowledgeEntry) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return left.Ref < right.Ref
}

func knowledgeReleaseLess(left, right KnowledgeRelease) bool {
	if left.Kind != right.Kind {
		return left.Kind < right.Kind
	}
	return left.Ref < right.Ref
}
