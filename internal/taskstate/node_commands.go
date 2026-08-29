package taskstate

import (
	"fmt"
	"sort"
)

func (command AddNodeCommand) decide(ledger *Ledger) (Event, error) {
	if err := requireCode(command.Actor, "create task nodes"); err != nil {
		return Event{}, err
	}
	if err := requireExactText(string(command.ID), "node ID"); err != nil {
		return Event{}, err
	}
	if _, exists := ledger.nodes[command.ID]; exists {
		return Event{}, fmt.Errorf("%w: node %q already exists", ErrInvalidState, command.ID)
	}
	if err := validateNodeKind(command.Kind); err != nil {
		return Event{}, err
	}
	if command.InlineExecution && command.Kind != NodeTask {
		return Event{}, fmt.Errorf("%w: inline execution is valid only for task nodes", ErrInvalidCommand)
	}
	if err := requireExactText(command.Title, "node title"); err != nil {
		return Event{}, err
	}
	if err := validatePriority(command.Priority); err != nil {
		return Event{}, err
	}
	if err := validateOptionalStep(command.CreatedStepID, "created step ID"); err != nil {
		return Event{}, err
	}
	if command.InlineExecution && command.CreatedStepID == nil {
		return Event{}, fmt.Errorf("%w: inline task requires its owning queue step", ErrInvalidCommand)
	}
	if err := validateCriteria(command.AcceptanceCriteria); err != nil {
		return Event{}, err
	}
	if err := command.Metadata.Validate(); err != nil {
		return Event{}, fmt.Errorf("%w: invalid node metadata: %v", ErrInvalidCommand, err)
	}
	if err := ledger.validateNodeHierarchy(command.ParentID, command.ObjectiveID, command.Kind); err != nil {
		return Event{}, err
	}
	node := Node{
		ID: command.ID, ParentID: command.ParentID, ObjectiveID: command.ObjectiveID,
		Kind: command.Kind, InlineExecution: command.InlineExecution, Title: command.Title, Status: NodePending,
		Priority: command.Priority, CreatedBy: command.Actor,
		CreatedStepID:      cloneInt64(command.CreatedStepID),
		AcceptanceCriteria: normalizedStrings(command.AcceptanceCriteria),
		Metadata:           cloneJSONObject(command.Metadata),
		VerificationRefs:   make([]Ref, 0),
		CreatedVersion:     ledger.version + 1, UpdatedVersion: ledger.version + 1,
	}
	return Event{Kind: EventNodeAdded, Node: &node, StepID: cloneInt64(command.CreatedStepID)}, nil
}

func (command AddEdgeCommand) decide(ledger *Ledger) (Event, error) {
	if err := requireCode(command.Actor, "create task edges"); err != nil {
		return Event{}, err
	}
	if err := requireExactText(string(command.ID), "edge ID"); err != nil {
		return Event{}, err
	}
	if _, exists := ledger.edges[command.ID]; exists {
		return Event{}, fmt.Errorf("%w: edge %q already exists", ErrInvalidState, command.ID)
	}
	if err := validateEdgeKind(command.Kind); err != nil {
		return Event{}, err
	}
	if command.From == command.To {
		return Event{}, fmt.Errorf("%w: edge cannot connect a node to itself", ErrInvalidCommand)
	}
	from, fromOK := ledger.nodes[command.From]
	to, toOK := ledger.nodes[command.To]
	if !fromOK || !toOK {
		return Event{}, fmt.Errorf("%w: edge endpoints must both exist", ErrNotFound)
	}
	if ledger.nodeSuperseded(command.From) || ledger.nodeSuperseded(command.To) {
		return Event{}, fmt.Errorf("%w: edge endpoints must be current", ErrInvalidState)
	}
	if ledger.semanticEdgeExists(command.Kind, command.From, command.To) {
		return Event{}, fmt.Errorf("%w: semantic edge already exists", ErrInvalidState)
	}
	if err := validateEdgeEndpoints(command.Kind, from, to); err != nil {
		return Event{}, err
	}
	if command.Kind == EdgeDependsOn || command.Kind == EdgeBlocks {
		dependent, prerequisite := executionOrder(command.Kind, command.From, command.To)
		if ledger.executionPathExists(prerequisite, dependent) {
			return Event{}, fmt.Errorf("%w: execution-order edge creates a cycle", ErrInvalidState)
		}
	}
	edge := Edge{ID: command.ID, Kind: command.Kind, From: command.From, To: command.To,
		CreatedVersion: ledger.version + 1}
	return Event{Kind: EventEdgeAdded, Edge: &edge}, nil
}

func (command PromoteReadyNodesCommand) decide(ledger *Ledger) (Event, error) {
	if err := requireCode(command.Actor, "promote runnable nodes"); err != nil {
		return Event{}, err
	}
	ids := ledger.promotableNodeIDs()
	if len(ids) == 0 {
		return Event{}, ErrNoStateChange
	}
	return Event{Kind: EventNodesReadied, NodeIDs: ids}, nil
}

func (command AssignNodeStepCommand) decide(ledger *Ledger) (Event, error) {
	if err := requireCode(command.Actor, "assign node steps"); err != nil {
		return Event{}, err
	}
	if command.StepID <= 0 {
		return Event{}, fmt.Errorf("%w: assigned step ID must be positive", ErrInvalidCommand)
	}
	node, exists := ledger.nodes[command.NodeID]
	if !exists {
		return Event{}, fmt.Errorf("%w: node %q", ErrNotFound, command.NodeID)
	}
	if !executableNode(node.Kind) {
		return Event{}, fmt.Errorf("%w: node kind %q cannot be assigned a step", ErrInvalidState, node.Kind)
	}
	if node.InlineExecution {
		return Event{}, fmt.Errorf("%w: inline task %q cannot be assigned a separate queue step", ErrInvalidState, node.ID)
	}
	if ledger.nodeSuperseded(node.ID) {
		return Event{}, fmt.Errorf("%w: node %q is superseded", ErrInvalidState, node.ID)
	}
	if node.Status != NodePending && node.Status != NodeReady {
		return Event{}, fmt.Errorf("%w: node in status %q cannot be assigned", ErrInvalidState, node.Status)
	}
	if node.AssignedStepID != nil {
		return Event{}, fmt.Errorf("%w: node %q is already assigned", ErrInvalidState, node.ID)
	}
	for _, existing := range ledger.nodes {
		if existing.AssignedStepID != nil && *existing.AssignedStepID == command.StepID {
			return Event{}, fmt.Errorf("%w: step %d is already assigned in this ledger", ErrInvalidState, command.StepID)
		}
	}
	return Event{Kind: EventNodeStepAssigned, NodeID: node.ID, StepID: int64Pointer(command.StepID)}, nil
}

func (command TransitionNodeCommand) decide(ledger *Ledger) (Event, error) {
	if err := requireCode(command.Actor, "transition task nodes"); err != nil {
		return Event{}, err
	}
	node, exists := ledger.nodes[command.NodeID]
	if !exists {
		return Event{}, fmt.Errorf("%w: node %q", ErrNotFound, command.NodeID)
	}
	if ledger.nodeSuperseded(node.ID) {
		return Event{}, fmt.Errorf("%w: node %q is superseded", ErrInvalidState, node.ID)
	}
	if err := validateNodeTransition(node, command); err != nil {
		return Event{}, err
	}
	stepID := cloneInt64(command.CompletedStepID)
	if command.To != NodeDone {
		stepID = cloneInt64(node.AssignedStepID)
	}
	return Event{
		Kind: EventNodeTransitioned, NodeID: node.ID, FromStatus: node.Status,
		ToStatus: command.To, StepID: stepID,
		VerificationRefs: cloneRefs(command.VerificationRefs), Reason: command.Reason,
	}, nil
}

func (command TerminalFailNodeCommand) decide(ledger *Ledger) (Event, error) {
	if err := requireCode(command.Actor, "terminally fail task nodes"); err != nil {
		return Event{}, err
	}
	node, exists := ledger.nodes[command.NodeID]
	if !exists {
		return Event{}, fmt.Errorf("%w: node %q", ErrNotFound, command.NodeID)
	}
	if ledger.nodeSuperseded(node.ID) || terminalNode(node.Status) {
		return Event{}, fmt.Errorf("%w: node %q is not open", ErrInvalidState, node.ID)
	}
	if err := validateTerminalNodeFailure(node, command.Reason, command.Proof); err != nil {
		return Event{}, err
	}
	return Event{
		Kind: EventNodeTransitioned, NodeID: node.ID, FromStatus: node.Status,
		ToStatus: NodeFailed, StepID: cloneInt64(node.AssignedStepID),
		VerificationRefs: []Ref{command.Proof}, Reason: command.Reason,
	}, nil
}

func (ledger *Ledger) validateNodeHierarchy(parentID, objectiveID NodeID, kind NodeKind) error {
	if parentID != "" {
		parent, exists := ledger.nodes[parentID]
		if !exists {
			return fmt.Errorf("%w: parent node %q", ErrNotFound, parentID)
		}
		if ledger.nodeSuperseded(parentID) {
			return fmt.Errorf("%w: parent node %q is superseded", ErrInvalidState, parentID)
		}
		if parent.Kind != NodeGoal && parent.Kind != NodeObjective && parent.Kind != NodeChangeGroup {
			return fmt.Errorf("%w: node kind %q cannot be a parent", ErrInvalidState, parent.Kind)
		}
	}
	if objectiveID != "" {
		objective, exists := ledger.nodes[objectiveID]
		if !exists {
			return fmt.Errorf("%w: objective node %q", ErrNotFound, objectiveID)
		}
		if ledger.nodeSuperseded(objectiveID) {
			return fmt.Errorf("%w: objective node %q is superseded", ErrInvalidState, objectiveID)
		}
		if objective.Kind != NodeObjective {
			return fmt.Errorf("%w: objective ID must reference an objective node", ErrInvalidState)
		}
	}
	if kind == NodeGoal && (parentID != "" || objectiveID != "") {
		return fmt.Errorf("%w: goal nodes cannot have parent or objective IDs", ErrInvalidCommand)
	}
	return nil
}

func validateEdgeEndpoints(kind EdgeKind, from, to Node) error {
	switch kind {
	case EdgeDecomposes:
		if from.Kind != NodeGoal && from.Kind != NodeObjective && from.Kind != NodeChangeGroup {
			return fmt.Errorf("%w: decomposition source must be a goal, objective, or change group", ErrInvalidCommand)
		}
		if to.Kind == NodeGoal {
			return fmt.Errorf("%w: decomposition target cannot be a goal", ErrInvalidCommand)
		}
	case EdgeVerifies:
		if from.Kind != NodeCheckpoint || !executableNode(to.Kind) {
			return fmt.Errorf("%w: verifies must point from a checkpoint to an executable node", ErrInvalidCommand)
		}
	}
	return nil
}

func executionOrder(kind EdgeKind, from, to NodeID) (NodeID, NodeID) {
	if kind == EdgeBlocks {
		return to, from
	}
	return from, to
}

func (ledger *Ledger) semanticEdgeExists(kind EdgeKind, from, to NodeID) bool {
	if kind == EdgeDependsOn || kind == EdgeBlocks {
		dependent, prerequisite := executionOrder(kind, from, to)
		for _, edge := range ledger.edges {
			if edge.Kind != EdgeDependsOn && edge.Kind != EdgeBlocks {
				continue
			}
			if ledger.nodeSuperseded(edge.From) || ledger.nodeSuperseded(edge.To) {
				continue
			}
			existingDependent, existingPrerequisite := executionOrder(edge.Kind, edge.From, edge.To)
			if dependent == existingDependent && prerequisite == existingPrerequisite {
				return true
			}
		}
		return false
	}
	for _, edge := range ledger.edges {
		if edge.Kind == kind && edge.From == from && edge.To == to {
			return true
		}
	}
	return false
}

func (ledger *Ledger) executionPathExists(from, to NodeID) bool {
	visited := make(map[NodeID]bool)
	var visit func(NodeID) bool
	visit = func(current NodeID) bool {
		if current == to {
			return true
		}
		if visited[current] {
			return false
		}
		visited[current] = true
		for _, edge := range ledger.edges {
			if edge.Kind != EdgeDependsOn && edge.Kind != EdgeBlocks {
				continue
			}
			if ledger.nodeSuperseded(edge.From) || ledger.nodeSuperseded(edge.To) {
				continue
			}
			dependent, prerequisite := executionOrder(edge.Kind, edge.From, edge.To)
			if dependent == current && visit(prerequisite) {
				return true
			}
		}
		return false
	}
	return visit(from)
}

func (ledger *Ledger) promotableNodeIDs() []NodeID {
	ids := make([]NodeID, 0)
	for id, node := range ledger.nodes {
		if !ledger.nodeSuperseded(id) && node.Status == NodePending && ledger.dependenciesDone(id) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		left, right := ledger.nodes[ids[i]], ledger.nodes[ids[j]]
		if left.Priority != right.Priority {
			return left.Priority > right.Priority
		}
		if left.CreatedVersion != right.CreatedVersion {
			return left.CreatedVersion < right.CreatedVersion
		}
		return left.ID < right.ID
	})
	return ids
}

// HasPromotableNode lets a code-owned coordinator avoid issuing a ledger
// transition that would make no state change. It exposes no scheduling
// authority beyond the ledger's existing dependency evaluation.
func (ledger *Ledger) HasPromotableNode() bool {
	return ledger != nil && len(ledger.promotableNodeIDs()) > 0
}

func (ledger *Ledger) dependenciesDone(id NodeID) bool {
	for _, edge := range ledger.edges {
		if ledger.nodeSuperseded(edge.From) || ledger.nodeSuperseded(edge.To) {
			continue
		}
		dependent, prerequisite := executionOrder(edge.Kind, edge.From, edge.To)
		if edge.Kind != EdgeDependsOn && edge.Kind != EdgeBlocks {
			continue
		}
		if dependent == id && ledger.nodes[prerequisite].Status != NodeDone {
			return false
		}
	}
	return true
}
