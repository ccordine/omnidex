package taskstate

type LedgerID string
type CommandID string
type NodeID string
type EdgeID string
type EntryID string

const LedgerSchemaV1 = "omnidex.task-state-ledger.v1"

type OwnerKind string

const OwnerJob OwnerKind = "job"

type LedgerOwner struct {
	Kind  OwnerKind `json:"kind"`
	JobID int64     `json:"job_id"`
	RunID string    `json:"run_id"`
}

type LedgerStatus string

const (
	LedgerActive   LedgerStatus = "active"
	LedgerClosed   LedgerStatus = "closed"
	LedgerFailed   LedgerStatus = "failed"
	LedgerCanceled LedgerStatus = "canceled"
)

type Authority string

const (
	AuthorityUser                  Authority = "user"
	AuthorityCode                  Authority = "code"
	AuthorityToolEvidence          Authority = "tool_evidence"
	AuthorityModelProposal         Authority = "model_proposal"
	AuthorityAcceptedModelDecision Authority = "accepted_model_decision"
)

type NodeKind string

const (
	NodeGoal        NodeKind = "goal"
	NodeObjective   NodeKind = "objective"
	NodeTask        NodeKind = "task"
	NodeCheckpoint  NodeKind = "checkpoint"
	NodeChangeGroup NodeKind = "change_group"
)

type NodeStatus string

const (
	NodePending  NodeStatus = "pending"
	NodeReady    NodeStatus = "ready"
	NodeActive   NodeStatus = "active"
	NodeBlocked  NodeStatus = "blocked"
	NodeDone     NodeStatus = "done"
	NodeFailed   NodeStatus = "failed"
	NodeCanceled NodeStatus = "canceled"
)

type EdgeKind string

const (
	EdgeDependsOn  EdgeKind = "depends_on"
	EdgeBlocks     EdgeKind = "blocks"
	EdgeDecomposes EdgeKind = "decomposes_to"
	EdgeVerifies   EdgeKind = "verifies"
)

type EntryKind string

const (
	EntryConstraint        EntryKind = "constraint"
	EntryFact              EntryKind = "fact"
	EntryObservation       EntryKind = "observation"
	EntryHypothesis        EntryKind = "hypothesis"
	EntryDecisionCandidate EntryKind = "decision_candidate"
	EntryAcceptedDecision  EntryKind = "accepted_decision"
	EntryQuestion          EntryKind = "question"
	EntryFailure           EntryKind = "failure"
	EntryCheckpoint        EntryKind = "checkpoint"
	EntryNote              EntryKind = "note"
	EntryFeedback          EntryKind = "feedback"
)

type EntryStatus string

const (
	EntryActive     EntryStatus = "active"
	EntryResolved   EntryStatus = "resolved"
	EntryRejected   EntryStatus = "rejected"
	EntrySuperseded EntryStatus = "superseded"
)

type FeedbackPurpose string

const (
	FeedbackReplan        FeedbackPurpose = "replan"
	FeedbackInterrupt     FeedbackPurpose = "interrupt"
	FeedbackInputResponse FeedbackPurpose = "input_response"
)

type RefRelation string

const (
	RefEvidence    RefRelation = "evidence"
	RefSource      RefRelation = "source"
	RefSupports    RefRelation = "supports"
	RefContradicts RefRelation = "contradicts"
	RefConcerns    RefRelation = "concerns"
	RefVerifies    RefRelation = "verifies"
	RefSupersedes  RefRelation = "supersedes"
)

type CommandKind string

const (
	CommandAddNode                 CommandKind = "add_node"
	CommandAddEdge                 CommandKind = "add_edge"
	CommandAddEntry                CommandKind = "add_entry"
	CommandRejectEntry             CommandKind = "reject_entry"
	CommandResolveEntry            CommandKind = "resolve_entry"
	CommandSupersedeEntry          CommandKind = "supersede_entry"
	CommandAcceptDecision          CommandKind = "accept_decision"
	CommandPromoteReady            CommandKind = "promote_ready_nodes"
	CommandAssignStep              CommandKind = "assign_node_step"
	CommandTransitionNode          CommandKind = "transition_node"
	CommandSupersedeNodeGeneration CommandKind = "supersede_node_generation"
	CommandCloseLedger             CommandKind = "close_ledger"
)

type EventKind string

const (
	EventNodeAdded                EventKind = "node_added"
	EventEdgeAdded                EventKind = "edge_added"
	EventEntryAdded               EventKind = "entry_added"
	EventEntryRejected            EventKind = "entry_rejected"
	EventEntryResolved            EventKind = "entry_resolved"
	EventEntrySuperseded          EventKind = "entry_superseded"
	EventDecisionAccepted         EventKind = "decision_accepted"
	EventNodesReadied             EventKind = "nodes_readied"
	EventNodeStepAssigned         EventKind = "node_step_assigned"
	EventNodeTransitioned         EventKind = "node_transitioned"
	EventNodeGenerationSuperseded EventKind = "node_generation_superseded"
	EventLedgerClosed             EventKind = "ledger_closed"
)

type Ref struct {
	URI      string      `json:"uri"`
	Version  string      `json:"version"`
	Hash     string      `json:"content_sha256"`
	Relation RefRelation `json:"relation"`
}

type EntryProvenance struct {
	SourceEntryID    EntryID   `json:"source_entry_id,omitempty"`
	AcceptancePolicy string    `json:"acceptance_policy,omitempty"`
	AcceptedBy       Authority `json:"accepted_by,omitempty"`
}

type Node struct {
	ID                 NodeID     `json:"id"`
	ParentID           NodeID     `json:"parent_id,omitempty"`
	ObjectiveID        NodeID     `json:"objective_id,omitempty"`
	Kind               NodeKind   `json:"kind"`
	Title              string     `json:"title"`
	Status             NodeStatus `json:"status"`
	Priority           int        `json:"priority"`
	CreatedBy          Authority  `json:"created_by"`
	AssignedStepID     *int64     `json:"assigned_step_id,omitempty"`
	CreatedStepID      *int64     `json:"created_step_id,omitempty"`
	CompletedStepID    *int64     `json:"completed_step_id,omitempty"`
	VerificationRefs   []Ref      `json:"verification_refs"`
	AcceptanceCriteria []string   `json:"acceptance_criteria"`
	Metadata           JSONObject `json:"metadata"`
	StatusReason       string     `json:"status_reason,omitempty"`
	CreatedVersion     uint64     `json:"created_version"`
	UpdatedVersion     uint64     `json:"updated_version"`
}

type NodeGenerationSupersession struct {
	NodeID                 NodeID `json:"node_id"`
	RetiringGeneration     int64  `json:"retiring_generation"`
	SupersededAtGeneration int64  `json:"superseded_at_generation"`
	Reason                 string `json:"reason"`
	CreatedVersion         uint64 `json:"created_version"`
}

type Edge struct {
	ID             EdgeID   `json:"id"`
	Kind           EdgeKind `json:"kind"`
	From           NodeID   `json:"from_node_id"`
	To             NodeID   `json:"to_node_id"`
	CreatedVersion uint64   `json:"created_version"`
}

type Entry struct {
	ID                EntryID         `json:"id"`
	ScopeNodeID       NodeID          `json:"scope_node_id,omitempty"`
	Kind              EntryKind       `json:"kind"`
	FeedbackPurpose   FeedbackPurpose `json:"feedback_purpose,omitempty"`
	Status            EntryStatus     `json:"status"`
	Authority         Authority       `json:"authority"`
	Content           string          `json:"content"`
	ContentSHA256     string          `json:"content_sha256"`
	Confidence        *float64        `json:"confidence,omitempty"`
	CreatedBy         Authority       `json:"created_by"`
	CreatedStepID     *int64          `json:"created_step_id,omitempty"`
	SupersedesID      EntryID         `json:"supersedes_id,omitempty"`
	SupersededBy      EntryID         `json:"superseded_by,omitempty"`
	Metadata          JSONObject      `json:"metadata"`
	Provenance        EntryProvenance `json:"provenance"`
	Refs              []Ref           `json:"refs"`
	DispositionReason string          `json:"disposition_reason,omitempty"`
	DispositionBy     Authority       `json:"disposition_by,omitempty"`
	CreatedVersion    uint64          `json:"created_version"`
	UpdatedVersion    uint64          `json:"updated_version"`
}

func (entry Entry) Active() bool { return entry.Status == EntryActive }

type Event struct {
	LedgerID               LedgerID     `json:"ledger_id"`
	Version                uint64       `json:"ledger_version"`
	CommandID              CommandID    `json:"command_id"`
	CommandSHA256          string       `json:"command_sha256"`
	CommandKind            CommandKind  `json:"command_kind"`
	Kind                   EventKind    `json:"event_kind"`
	Authority              Authority    `json:"actor"`
	StepID                 *int64       `json:"step_id,omitempty"`
	Node                   *Node        `json:"node,omitempty"`
	Edge                   *Edge        `json:"edge,omitempty"`
	Entry                  *Entry       `json:"entry,omitempty"`
	NodeID                 NodeID       `json:"node_id,omitempty"`
	NodeIDs                []NodeID     `json:"node_ids,omitempty"`
	EntryID                EntryID      `json:"entry_id,omitempty"`
	ReplacementID          EntryID      `json:"replacement_id,omitempty"`
	FromStatus             NodeStatus   `json:"from_status,omitempty"`
	ToStatus               NodeStatus   `json:"to_status,omitempty"`
	VerificationRefs       []Ref        `json:"verification_refs,omitempty"`
	LedgerStatus           LedgerStatus `json:"ledger_status,omitempty"`
	Reason                 string       `json:"reason,omitempty"`
	RetiringGeneration     int64        `json:"retiring_generation,omitempty"`
	SupersededAtGeneration int64        `json:"superseded_at_generation,omitempty"`
}

type Ledger struct {
	id                LedgerID
	owner             LedgerOwner
	version           uint64
	status            LedgerStatus
	nodes             map[NodeID]Node
	edges             map[EdgeID]Edge
	entries           map[EntryID]Entry
	nodeSupersessions map[NodeID]NodeGenerationSupersession
	nodeRefCount      int
	entryRefCount     int
	events            []Event
	commandEvents     map[CommandID]Event
}
