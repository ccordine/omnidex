package cognition

type ScenarioID string
type EpisodeID string
type ActionID string
type ActionCatalogID string
type ActionSchemaID string
type ActionKind string
type ActionArgumentName string
type ObservationID string
type ObservationKind string
type EffectKind string
type PredicateName string
type ObligationID string

// ScenarioRef identifies only a public scenario artifact. Benchmark secrets are
// intentionally not representable by this production contract.
type ScenarioRef struct {
	ID     ScenarioID `json:"id"`
	SHA256 string     `json:"sha256"`
}

type EpisodeRef struct {
	ID EpisodeID `json:"id"`
}

type WorldRevision struct {
	EpisodeID EpisodeID `json:"episode_id"`
	Number    uint64    `json:"number"`
	SHA256    string    `json:"sha256"`
}

type Predicate struct {
	Name PredicateName `json:"name"`
	Args []string      `json:"args"`
}

type GoalExpression struct {
	All []Predicate `json:"all,omitempty"`
	Any []Predicate `json:"any,omitempty"`
	Not []Predicate `json:"not,omitempty"`
}

type EvidenceRef struct {
	ObservationID ObservationID `json:"observation_id"`
	Revision      WorldRevision `json:"revision"`
	SHA256        string        `json:"sha256"`
}

type Observation struct {
	ID            ObservationID   `json:"id"`
	ActionID      ActionID        `json:"action_id,omitempty"`
	Revision      WorldRevision   `json:"revision"`
	Kind          ObservationKind `json:"kind"`
	Content       string          `json:"content"`
	ContentSHA256 string          `json:"content_sha256"`
}

// Effect is a bounded public result summary. It deliberately cannot carry
// environment-internal predicates or latent state changes.
type Effect struct {
	ActionID      ActionID      `json:"action_id"`
	Revision      WorldRevision `json:"revision"`
	Kind          EffectKind    `json:"kind"`
	Content       string        `json:"content"`
	ContentSHA256 string        `json:"content_sha256"`
}

const (
	EffectStateChanged        EffectKind = "state_changed"
	EffectObservationProduced EffectKind = "observation_produced"
	EffectNoChange            EffectKind = "no_change"
)

type EvidencePolicy string

const (
	EvidenceOptional  EvidencePolicy = "optional"
	EvidenceRequired  EvidencePolicy = "required"
	EvidenceForbidden EvidencePolicy = "forbidden"
)

type ActionParameterSpec struct {
	Name     ActionArgumentName `json:"name"`
	Required bool               `json:"required"`
	MaxBytes int                `json:"max_bytes"`
}

type ActionSchemaRef struct {
	ID      ActionSchemaID `json:"id"`
	Version string         `json:"version"`
	SHA256  string         `json:"sha256"`
}

type ActionSchema struct {
	ID             ActionSchemaID        `json:"id"`
	Version        string                `json:"version"`
	SHA256         string                `json:"sha256"`
	Kind           ActionKind            `json:"kind"`
	Parameters     []ActionParameterSpec `json:"parameters"`
	EvidencePolicy EvidencePolicy        `json:"evidence_policy"`
}

type ActionCatalog struct {
	ID      ActionCatalogID `json:"id"`
	Version string          `json:"version"`
	SHA256  string          `json:"sha256"`
	Schemas []ActionSchema  `json:"schemas"`
}

type ActionArgument struct {
	Name  ActionArgumentName `json:"name"`
	Value string             `json:"value"`
}

// ActionRequest is model-proposable and deliberately has no action identity.
type ActionRequest struct {
	Kind      ActionKind       `json:"kind"`
	Arguments []ActionArgument `json:"arguments"`
}

// RegisteredAction is the code-authorized action passed to an Environment.
// ID is its sole idempotency identity.
type RegisteredAction struct {
	ID           ActionID        `json:"id"`
	Actor        AttemptRef      `json:"actor"`
	Schema       ActionSchemaRef `json:"schema"`
	Request      ActionRequest   `json:"request"`
	EvidenceRefs []EvidenceRef   `json:"evidence_refs"`
}

// AttemptRef carries the code-owned lease generation needed by an environment
// host to fence stale workers before applying an action.
type AttemptRef struct {
	JobID      int64  `json:"job_id"`
	Generation int64  `json:"generation"`
	StepID     int64  `json:"step_id"`
	Attempt    uint64 `json:"attempt"`
	WorkerID   string `json:"worker_id"`
}

type Transition struct {
	ActionID      ActionID       `json:"action_id,omitempty"`
	Previous      *WorldRevision `json:"previous,omitempty"`
	Current       WorldRevision  `json:"current"`
	Observations  []Observation  `json:"observations"`
	Effects       []Effect       `json:"effects"`
	Cost          int            `json:"cost"`
	Terminal      bool           `json:"terminal"`
	PublicOutcome string         `json:"public_outcome,omitempty"`
}
