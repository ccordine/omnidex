package taskstate

import (
	"fmt"
	"reflect"
)

func (ledger *Ledger) applyEvent(event Event) error {
	if event.Version != ledger.version+1 {
		return VersionConflictError{Expected: ledger.version + 1, Actual: event.Version}
	}
	if ledger.status != LedgerActive {
		return fmt.Errorf("ledger is terminal with status %q", ledger.status)
	}
	if err := ledger.validateProjectedAggregateCapacity(event); err != nil {
		return err
	}
	var err error
	switch event.Kind {
	case EventNodeAdded:
		err = ledger.applyNodeAdded(event)
	case EventEdgeAdded:
		err = ledger.applyEdgeAdded(event)
	case EventEntryAdded:
		err = ledger.applyEntryAdded(event)
	case EventEntryRejected:
		err = ledger.applyEntryStatus(event, EntryRejected)
	case EventEntryResolved:
		err = ledger.applyEntryStatus(event, EntryResolved)
	case EventEntrySuperseded:
		err = ledger.applyEntrySuperseded(event)
	case EventNodesReadied:
		err = ledger.applyNodesReadied(event)
	case EventNodeStepAssigned:
		err = ledger.applyNodeStepAssigned(event)
	case EventNodeTransitioned:
		err = ledger.applyNodeTransitioned(event)
	case EventNodeGenerationSuperseded:
		err = ledger.applyNodeGenerationSuperseded(event)
	case EventLedgerClosed:
		err = ledger.applyLedgerClosed(event)
	default:
		err = fmt.Errorf("event kind %q is not registered", event.Kind)
	}
	if err != nil {
		return err
	}
	ledger.version = event.Version
	if err := ledger.validateAggregateCapacity(); err != nil {
		return fmt.Errorf("post-event aggregate capacity invariant failed: %w", err)
	}
	return nil
}

func (ledger *Ledger) applyNodeAdded(event Event) error {
	if event.Authority != AuthorityCode || event.Node == nil {
		return fmt.Errorf("node-added event requires code authority and a node")
	}
	node := cloneNode(*event.Node)
	if _, exists := ledger.nodes[node.ID]; exists {
		return fmt.Errorf("node %q already exists", node.ID)
	}
	if err := requireExactText(string(node.ID), "node ID"); err != nil {
		return err
	}
	if err := validateNodeKind(node.Kind); err != nil {
		return err
	}
	if node.Status != NodePending || node.CreatedBy != event.Authority ||
		node.CreatedVersion != event.Version || node.UpdatedVersion != event.Version {
		return fmt.Errorf("new node projection has invalid authority, status, or versions")
	}
	if node.VerificationRefs == nil || len(node.VerificationRefs) != 0 {
		return fmt.Errorf("new node verification references must be an empty array")
	}
	if err := requireExactText(node.Title, "node title"); err != nil {
		return err
	}
	if err := validatePriority(node.Priority); err != nil {
		return err
	}
	if err := validateOptionalStep(node.CreatedStepID, "created step ID"); err != nil {
		return err
	}
	if !equalInt64Pointers(node.CreatedStepID, event.StepID) {
		return fmt.Errorf("node created step does not match event step")
	}
	if err := validateCriteria(node.AcceptanceCriteria); err != nil {
		return err
	}
	if err := node.Metadata.Validate(); err != nil {
		return err
	}
	if err := ledger.validateNodeHierarchy(node.ParentID, node.ObjectiveID, node.Kind); err != nil {
		return err
	}
	ledger.nodes[node.ID] = node
	return nil
}

func (ledger *Ledger) applyEdgeAdded(event Event) error {
	if event.Authority != AuthorityCode || event.Edge == nil {
		return fmt.Errorf("edge-added event requires code authority and an edge")
	}
	edge := *event.Edge
	if _, exists := ledger.edges[edge.ID]; exists {
		return fmt.Errorf("edge %q already exists", edge.ID)
	}
	if err := requireExactText(string(edge.ID), "edge ID"); err != nil {
		return err
	}
	if err := validateEdgeKind(edge.Kind); err != nil {
		return err
	}
	from, fromOK := ledger.nodes[edge.From]
	to, toOK := ledger.nodes[edge.To]
	if !fromOK || !toOK || edge.From == edge.To || edge.CreatedVersion != event.Version {
		return fmt.Errorf("edge projection has invalid endpoints or version")
	}
	if ledger.nodeSuperseded(edge.From) || ledger.nodeSuperseded(edge.To) {
		return fmt.Errorf("edge endpoints must be current")
	}
	if ledger.semanticEdgeExists(edge.Kind, edge.From, edge.To) {
		return fmt.Errorf("semantic edge already exists")
	}
	if err := validateEdgeEndpoints(edge.Kind, from, to); err != nil {
		return err
	}
	if edge.Kind == EdgeDependsOn || edge.Kind == EdgeBlocks {
		dependent, prerequisite := executionOrder(edge.Kind, edge.From, edge.To)
		if ledger.executionPathExists(prerequisite, dependent) {
			return fmt.Errorf("execution-order edge creates a cycle")
		}
	}
	ledger.edges[edge.ID] = edge
	return nil
}

func (ledger *Ledger) applyEntryAdded(event Event) error {
	if event.Entry == nil {
		return fmt.Errorf("entry-added event requires an entry")
	}
	entry := cloneEntry(*event.Entry)
	if err := validateNewEntryAuthority(event.Authority, entry.Kind); err != nil {
		return err
	}
	if err := ledger.validateNewEntryProjection(event, entry); err != nil {
		return err
	}
	ledger.entries[entry.ID] = entry
	ledger.entryRefCount += len(entry.Refs)
	return nil
}

func (ledger *Ledger) applyEntryStatus(event Event, target EntryStatus) error {
	entry, exists := ledger.entries[event.EntryID]
	if !exists || entry.Status != EntryActive {
		return fmt.Errorf("entry %q is missing or inactive", event.EntryID)
	}
	if target == EntryRejected {
		if event.Authority != AuthorityCode && event.Authority != AuthorityUser {
			return fmt.Errorf("entry rejection authority is invalid")
		}
		if entry.Authority == AuthorityUser && event.Authority != AuthorityUser {
			return fmt.Errorf("user-authority state requires user rejection authority")
		}
		if err := validateRefs(event.VerificationRefs); err != nil {
			return err
		}
		if entry.Kind == EntryHypothesis && event.Authority == AuthorityCode &&
			!hasContradictionRef(event.VerificationRefs) {
			return fmt.Errorf("code rejection of a hypothesis requires contradiction evidence")
		}
		combined := append(cloneRefs(entry.Refs), event.VerificationRefs...)
		if err := validateRefs(combined); err != nil {
			return fmt.Errorf("rejected entry reference set is invalid: %w", err)
		}
		entry.Refs = combined
		ledger.entryRefCount += len(event.VerificationRefs)
	} else {
		if event.Authority != AuthorityCode {
			return fmt.Errorf("entry resolution requires code authority")
		}
		switch entry.Kind {
		case EntryQuestion, EntryFailure, EntryObservation, EntryNote, EntryFeedback:
		default:
			return fmt.Errorf("entry kind %q cannot resolve", entry.Kind)
		}
		if err := validateRefs(event.VerificationRefs); err != nil {
			return err
		}
		if !hasEvidenceRef(event.VerificationRefs) {
			return fmt.Errorf("entry resolution requires evidence")
		}
		combined := append(cloneRefs(entry.Refs), event.VerificationRefs...)
		if err := validateRefs(combined); err != nil {
			return fmt.Errorf("resolved entry reference set is invalid: %w", err)
		}
		entry.Refs = combined
		ledger.entryRefCount += len(event.VerificationRefs)
	}
	if err := requireExactText(event.Reason, "entry disposition reason"); err != nil {
		return err
	}
	entry.Status = target
	entry.DispositionReason = event.Reason
	entry.DispositionBy = event.Authority
	entry.UpdatedVersion = event.Version
	ledger.entries[entry.ID] = entry
	return nil
}

func (ledger *Ledger) applyEntrySuperseded(event Event) error {
	if event.Authority != AuthorityCode && event.Authority != AuthorityUser {
		return fmt.Errorf("entry supersession authority is invalid")
	}
	entry, exists := ledger.entries[event.EntryID]
	replacement, replacementExists := ledger.entries[event.ReplacementID]
	if !exists || !replacementExists || entry.ID == replacement.ID ||
		entry.Status != EntryActive || replacement.Status != EntryActive || replacement.SupersedesID != "" {
		return fmt.Errorf("supersession requires two different active entries")
	}
	if entry.Kind != replacement.Kind || authorityRank(replacement.Authority) < authorityRank(entry.Authority) {
		return fmt.Errorf("replacement kind or authority is invalid")
	}
	if entry.Authority == AuthorityUser && (event.Authority != AuthorityUser || replacement.Authority != AuthorityUser) {
		return fmt.Errorf("user-authority state requires a user-authority replacement")
	}
	if err := requireExactText(event.Reason, "supersession reason"); err != nil {
		return err
	}
	entry.Status, entry.SupersededBy = EntrySuperseded, replacement.ID
	entry.DispositionReason, entry.UpdatedVersion = event.Reason, event.Version
	entry.DispositionBy = event.Authority
	replacement.SupersedesID, replacement.UpdatedVersion = entry.ID, event.Version
	ledger.entries[entry.ID], ledger.entries[replacement.ID] = entry, replacement
	return nil
}

func (ledger *Ledger) applyNodesReadied(event Event) error {
	if event.Authority != AuthorityCode {
		return fmt.Errorf("node promotion requires code authority")
	}
	want := ledger.promotableNodeIDs()
	if len(want) == 0 || !reflect.DeepEqual(want, event.NodeIDs) {
		return fmt.Errorf("readied node set is not the exact deterministic promotion set")
	}
	for _, id := range want {
		node := ledger.nodes[id]
		node.Status, node.UpdatedVersion = NodeReady, event.Version
		ledger.nodes[id] = node
	}
	return nil
}
