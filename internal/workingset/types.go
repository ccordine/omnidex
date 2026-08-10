package workingset

import "github.com/gryph/omnidex/internal/taskstate"

// Ref is the single stable-reference contract shared with the task ledger. Working
// sets add attention roles and scope memberships without creating another reference
// identity.
type Ref = taskstate.Ref

type SetID string
type ItemID string
type ScopeID string
type CommandID string

type ScopeKind string

const WorkingSetSchemaV1 = "omnidex.working-set.v1"

type Owner struct {
	LedgerID   taskstate.LedgerID `json:"ledger_id"`
	JobID      int64              `json:"job_id"`
	Generation int64              `json:"generation"`
}

type Status string

const (
	StatusActive Status = "active"
	StatusClosed Status = "closed"
)

const (
	ScopeCall      ScopeKind = "call"
	ScopeStep      ScopeKind = "step"
	ScopePhase     ScopeKind = "phase"
	ScopeTask      ScopeKind = "task"
	ScopeObjective ScopeKind = "objective"
	ScopeJob       ScopeKind = "job"
)

type Scope struct {
	Kind ScopeKind `json:"kind"`
	ID   ScopeID   `json:"id"`
}

type Retention string

const (
	RetentionCall      Retention = "call"
	RetentionStep      Retention = "step"
	RetentionPhase     Retention = "phase"
	RetentionTask      Retention = "task"
	RetentionObjective Retention = "objective"
	RetentionJob       Retention = "job"
	RetentionPinned    Retention = "pinned"
)

type Role string

const (
	RoleUserAuthority       Role = "user_authority"
	RoleGoal                Role = "goal"
	RoleObjective           Role = "objective"
	RoleTask                Role = "task"
	RoleAcceptanceCriterion Role = "acceptance_criterion"
	RoleConstraint          Role = "constraint"
	RoleFact                Role = "fact"
	RoleHypothesis          Role = "hypothesis"
	RoleDecision            Role = "decision"
	RoleInvariant           Role = "invariant"
	RoleFailure             Role = "failure"
	RoleQuestion            Role = "question"
	RoleEvidence            Role = "evidence"
	RoleRepositoryEvidence  Role = "repository_evidence"
	RoleDependency          Role = "dependency"
	RoleVerification        Role = "verification"
	RoleHistorical          Role = "historical"
)

type ItemState string

const (
	ItemResident    ItemState = "resident"
	ItemReleased    ItemState = "released"
	ItemInvalidated ItemState = "invalidated"
)

type Provider string

const (
	ProviderUser          Provider = "user"
	ProviderTaskState     Provider = "task_state"
	ProviderRepository    Provider = "repository"
	ProviderArtifact      Provider = "artifact"
	ProviderEvidence      Provider = "evidence"
	ProviderDurableMemory Provider = "durable_memory"
	ProviderWeb           Provider = "web"
	ProviderCompiler      Provider = "compiler"
	ProviderTest          Provider = "test"
	ProviderCommand       Provider = "command"
)

type Acquisition struct {
	Provider    Provider `json:"provider"`
	OperationID string   `json:"operation_id"`
	Reason      string   `json:"reason"`
}

type Membership struct {
	Scope     Scope     `json:"scope"`
	Retention Retention `json:"retention"`
}

type Item struct {
	ID                 ItemID       `json:"id"`
	Ref                Ref          `json:"ref"`
	Role               Role         `json:"role"`
	Retention          Retention    `json:"retention"`
	Priority           int          `json:"priority"`
	State              ItemState    `json:"state"`
	ByteCost           int          `json:"byte_cost"`
	Acquisition        Acquisition  `json:"acquisition"`
	Memberships        []Membership `json:"memberships"`
	UseCount           uint64       `json:"use_count"`
	ReacquisitionCount uint64       `json:"reacquisition_count"`
	CreatedTick        uint64       `json:"created_tick"`
	LastUsedTick       uint64       `json:"last_used_tick"`
	ReleasedTick       uint64       `json:"released_tick,omitempty"`
	DispositionReason  string       `json:"disposition_reason,omitempty"`
}

func (item Item) SourceHash() string { return item.Ref.Hash }

type Budget struct {
	MaxItems       int `json:"max_items"`
	MaxBytes       int `json:"max_bytes"`
	MaxPinnedItems int `json:"max_pinned_items"`
	MaxPinnedBytes int `json:"max_pinned_bytes"`
}

type Usage struct {
	ResidentItems int `json:"resident_items"`
	ResidentBytes int `json:"resident_bytes"`
	PinnedItems   int `json:"pinned_items"`
	PinnedBytes   int `json:"pinned_bytes"`
}

type AcquireRequest struct {
	ID          ItemID      `json:"id"`
	Ref         Ref         `json:"ref"`
	Role        Role        `json:"role"`
	Retention   Retention   `json:"retention"`
	Scope       Scope       `json:"scope"`
	Priority    int         `json:"priority"`
	ByteCost    int         `json:"byte_cost"`
	Acquisition Acquisition `json:"acquisition"`
}

type AcquireResult struct {
	Item    Item
	Evicted []Item
}

type ReacquireRequest struct {
	ItemID                     ItemID    `json:"item_id"`
	Ref                        Ref       `json:"ref"`
	Scope                      Scope     `json:"scope"`
	Retention                  Retention `json:"retention"`
	ExpectedReacquisitionCount uint64    `json:"expected_reacquisition_count"`
	Reason                     string    `json:"reason"`
}

type ReacquireResult struct {
	Item    Item
	Evicted []Item
}

type ScopeCloseResult struct {
	Updated  []Item
	Released []Item
}

type Snapshot struct {
	Schema       string  `json:"schema"`
	ID           SetID   `json:"id"`
	Owner        Owner   `json:"owner"`
	Scope        Scope   `json:"scope"`
	Budget       Budget  `json:"budget"`
	Status       Status  `json:"status"`
	Version      uint64  `json:"version"`
	Clock        uint64  `json:"clock"`
	Items        []Item  `json:"items"`
	ClosedScopes []Scope `json:"closed_scopes"`
	ClosedTick   uint64  `json:"closed_tick,omitempty"`
	CloseReason  string  `json:"close_reason,omitempty"`
}

type Set struct {
	id            SetID
	owner         Owner
	scope         Scope
	budget        Budget
	status        Status
	version       uint64
	clock         uint64
	items         map[ItemID]Item
	refs          map[string]ItemID
	closedScopes  map[string]Scope
	closedTick    uint64
	closeReason   string
	commandEvents map[CommandID]Event
}
