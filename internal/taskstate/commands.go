package taskstate

type Command interface {
	taskStateCommand()
	commandID() CommandID
	expectedVersion() uint64
	actor() Authority
	kind() CommandKind
	decide(*Ledger) (Event, error)
}

type AddNodeCommand struct {
	CommandID          CommandID  `json:"command_id"`
	ExpectedVersion    uint64     `json:"expected_version"`
	Actor              Authority  `json:"actor"`
	ID                 NodeID     `json:"id"`
	ParentID           NodeID     `json:"parent_id,omitempty"`
	ObjectiveID        NodeID     `json:"objective_id,omitempty"`
	Kind               NodeKind   `json:"kind"`
	Title              string     `json:"title"`
	Priority           int        `json:"priority"`
	CreatedStepID      *int64     `json:"created_step_id,omitempty"`
	AcceptanceCriteria []string   `json:"acceptance_criteria"`
	Metadata           JSONObject `json:"metadata"`
}

type AddEdgeCommand struct {
	CommandID       CommandID `json:"command_id"`
	ExpectedVersion uint64    `json:"expected_version"`
	Actor           Authority `json:"actor"`
	ID              EdgeID    `json:"id"`
	Kind            EdgeKind  `json:"kind"`
	From            NodeID    `json:"from_node_id"`
	To              NodeID    `json:"to_node_id"`
}

type AddEntryCommand struct {
	CommandID       CommandID       `json:"command_id"`
	ExpectedVersion uint64          `json:"expected_version"`
	Actor           Authority       `json:"actor"`
	ID              EntryID         `json:"id"`
	ScopeNodeID     NodeID          `json:"scope_node_id,omitempty"`
	Kind            EntryKind       `json:"kind"`
	FeedbackPurpose FeedbackPurpose `json:"feedback_purpose,omitempty"`
	Content         string          `json:"content"`
	Confidence      *float64        `json:"confidence,omitempty"`
	CreatedStepID   *int64          `json:"created_step_id,omitempty"`
	Metadata        JSONObject      `json:"metadata"`
	Refs            []Ref           `json:"refs"`
}

type RejectEntryCommand struct {
	CommandID       CommandID `json:"command_id"`
	ExpectedVersion uint64    `json:"expected_version"`
	Actor           Authority `json:"actor"`
	EntryID         EntryID   `json:"entry_id"`
	Reason          string    `json:"reason"`
}

type ResolveEntryCommand struct {
	CommandID       CommandID `json:"command_id"`
	ExpectedVersion uint64    `json:"expected_version"`
	Actor           Authority `json:"actor"`
	EntryID         EntryID   `json:"entry_id"`
	Reason          string    `json:"reason"`
	Refs            []Ref     `json:"refs"`
}

type SupersedeEntryCommand struct {
	CommandID       CommandID `json:"command_id"`
	ExpectedVersion uint64    `json:"expected_version"`
	Actor           Authority `json:"actor"`
	EntryID         EntryID   `json:"entry_id"`
	ReplacementID   EntryID   `json:"replacement_id"`
	Reason          string    `json:"reason"`
}

type AcceptDecisionCommand struct {
	CommandID        CommandID  `json:"command_id"`
	ExpectedVersion  uint64     `json:"expected_version"`
	Actor            Authority  `json:"actor"`
	CandidateID      EntryID    `json:"candidate_id"`
	AcceptedEntryID  EntryID    `json:"accepted_entry_id"`
	AcceptancePolicy string     `json:"acceptance_policy"`
	AcceptanceRefs   []Ref      `json:"acceptance_refs"`
	CreatedStepID    *int64     `json:"created_step_id,omitempty"`
	Metadata         JSONObject `json:"metadata"`
}

type PromoteReadyNodesCommand struct {
	CommandID       CommandID `json:"command_id"`
	ExpectedVersion uint64    `json:"expected_version"`
	Actor           Authority `json:"actor"`
}

type AssignNodeStepCommand struct {
	CommandID       CommandID `json:"command_id"`
	ExpectedVersion uint64    `json:"expected_version"`
	Actor           Authority `json:"actor"`
	NodeID          NodeID    `json:"node_id"`
	StepID          int64     `json:"step_id"`
}

type TransitionNodeCommand struct {
	CommandID        CommandID  `json:"command_id"`
	ExpectedVersion  uint64     `json:"expected_version"`
	Actor            Authority  `json:"actor"`
	NodeID           NodeID     `json:"node_id"`
	To               NodeStatus `json:"to_status"`
	CompletedStepID  *int64     `json:"completed_step_id,omitempty"`
	VerificationRefs []Ref      `json:"verification_refs"`
	Reason           string     `json:"reason,omitempty"`
}

type CloseLedgerCommand struct {
	CommandID       CommandID    `json:"command_id"`
	ExpectedVersion uint64       `json:"expected_version"`
	Actor           Authority    `json:"actor"`
	Status          LedgerStatus `json:"status"`
	StepID          *int64       `json:"step_id,omitempty"`
	Reason          string       `json:"reason"`
}

func (AddNodeCommand) taskStateCommand()           {}
func (AddEdgeCommand) taskStateCommand()           {}
func (AddEntryCommand) taskStateCommand()          {}
func (RejectEntryCommand) taskStateCommand()       {}
func (ResolveEntryCommand) taskStateCommand()      {}
func (SupersedeEntryCommand) taskStateCommand()    {}
func (AcceptDecisionCommand) taskStateCommand()    {}
func (PromoteReadyNodesCommand) taskStateCommand() {}
func (AssignNodeStepCommand) taskStateCommand()    {}
func (TransitionNodeCommand) taskStateCommand()    {}
func (CloseLedgerCommand) taskStateCommand()       {}

func (c AddNodeCommand) commandID() CommandID           { return c.CommandID }
func (c AddEdgeCommand) commandID() CommandID           { return c.CommandID }
func (c AddEntryCommand) commandID() CommandID          { return c.CommandID }
func (c RejectEntryCommand) commandID() CommandID       { return c.CommandID }
func (c ResolveEntryCommand) commandID() CommandID      { return c.CommandID }
func (c SupersedeEntryCommand) commandID() CommandID    { return c.CommandID }
func (c AcceptDecisionCommand) commandID() CommandID    { return c.CommandID }
func (c PromoteReadyNodesCommand) commandID() CommandID { return c.CommandID }
func (c AssignNodeStepCommand) commandID() CommandID    { return c.CommandID }
func (c TransitionNodeCommand) commandID() CommandID    { return c.CommandID }
func (c CloseLedgerCommand) commandID() CommandID       { return c.CommandID }

func (c AddNodeCommand) expectedVersion() uint64           { return c.ExpectedVersion }
func (c AddEdgeCommand) expectedVersion() uint64           { return c.ExpectedVersion }
func (c AddEntryCommand) expectedVersion() uint64          { return c.ExpectedVersion }
func (c RejectEntryCommand) expectedVersion() uint64       { return c.ExpectedVersion }
func (c ResolveEntryCommand) expectedVersion() uint64      { return c.ExpectedVersion }
func (c SupersedeEntryCommand) expectedVersion() uint64    { return c.ExpectedVersion }
func (c AcceptDecisionCommand) expectedVersion() uint64    { return c.ExpectedVersion }
func (c PromoteReadyNodesCommand) expectedVersion() uint64 { return c.ExpectedVersion }
func (c AssignNodeStepCommand) expectedVersion() uint64    { return c.ExpectedVersion }
func (c TransitionNodeCommand) expectedVersion() uint64    { return c.ExpectedVersion }
func (c CloseLedgerCommand) expectedVersion() uint64       { return c.ExpectedVersion }

func (c AddNodeCommand) actor() Authority           { return c.Actor }
func (c AddEdgeCommand) actor() Authority           { return c.Actor }
func (c AddEntryCommand) actor() Authority          { return c.Actor }
func (c RejectEntryCommand) actor() Authority       { return c.Actor }
func (c ResolveEntryCommand) actor() Authority      { return c.Actor }
func (c SupersedeEntryCommand) actor() Authority    { return c.Actor }
func (c AcceptDecisionCommand) actor() Authority    { return c.Actor }
func (c PromoteReadyNodesCommand) actor() Authority { return c.Actor }
func (c AssignNodeStepCommand) actor() Authority    { return c.Actor }
func (c TransitionNodeCommand) actor() Authority    { return c.Actor }
func (c CloseLedgerCommand) actor() Authority       { return c.Actor }

func (AddNodeCommand) kind() CommandKind           { return CommandAddNode }
func (AddEdgeCommand) kind() CommandKind           { return CommandAddEdge }
func (AddEntryCommand) kind() CommandKind          { return CommandAddEntry }
func (RejectEntryCommand) kind() CommandKind       { return CommandRejectEntry }
func (ResolveEntryCommand) kind() CommandKind      { return CommandResolveEntry }
func (SupersedeEntryCommand) kind() CommandKind    { return CommandSupersedeEntry }
func (AcceptDecisionCommand) kind() CommandKind    { return CommandAcceptDecision }
func (PromoteReadyNodesCommand) kind() CommandKind { return CommandPromoteReady }
func (AssignNodeStepCommand) kind() CommandKind    { return CommandAssignStep }
func (TransitionNodeCommand) kind() CommandKind    { return CommandTransitionNode }
func (CloseLedgerCommand) kind() CommandKind       { return CommandCloseLedger }
