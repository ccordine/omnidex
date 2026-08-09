package taskstate

import (
	"fmt"
	"sort"
)

// RestoreLedger validates and loads normalized current state without replaying
// history. Reconstruct is the separate immutable-history audit boundary.
func RestoreLedger(state MaterializedState) (*Ledger, error) {
	if err := validateMaterializedCapacity(state); err != nil {
		return nil, err
	}
	ledger, err := NewLedger(state.ID, state.Owner)
	if err != nil {
		return nil, err
	}
	if state.Status != LedgerActive && !terminalLedger(state.Status) {
		return nil, fmt.Errorf("%w: ledger status %q is not registered", ErrInvalidState, state.Status)
	}
	if state.Version == 0 && (state.Status != LedgerActive || len(state.Nodes)+len(state.Edges)+len(state.Entries) != 0) {
		return nil, fmt.Errorf("%w: version zero ledger cannot contain materialized records or be terminal", ErrInvalidState)
	}
	assignedSteps := make(map[int64]NodeID)
	for _, supplied := range state.Nodes {
		if supplied.VerificationRefs == nil {
			return nil, fmt.Errorf("%w: node %q verification references must be an explicit array", ErrInvalidState, supplied.ID)
		}
		if supplied.AcceptanceCriteria == nil {
			return nil, fmt.Errorf("%w: node %q acceptance criteria must be an explicit array", ErrInvalidState, supplied.ID)
		}
		node := cloneNode(supplied)
		if err := validateRestoredNode(node, state.Version); err != nil {
			return nil, err
		}
		if _, exists := ledger.nodes[node.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate node %q", ErrInvalidState, node.ID)
		}
		if node.AssignedStepID != nil {
			if existing, exists := assignedSteps[*node.AssignedStepID]; exists {
				return nil, fmt.Errorf("%w: nodes %q and %q share assigned step %d", ErrInvalidState, existing, node.ID, *node.AssignedStepID)
			}
			assignedSteps[*node.AssignedStepID] = node.ID
		}
		ledger.nodes[node.ID] = node
		ledger.nodeRefCount += len(node.VerificationRefs)
	}
	for _, node := range ledger.nodes {
		if err := ledger.validateNodeHierarchy(node.ParentID, node.ObjectiveID, node.Kind); err != nil {
			return nil, err
		}
	}
	if err := ledger.validateRestoredNodeLinks(); err != nil {
		return nil, err
	}
	edges := append([]Edge(nil), state.Edges...)
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].CreatedVersion != edges[j].CreatedVersion {
			return edges[i].CreatedVersion < edges[j].CreatedVersion
		}
		return edges[i].ID < edges[j].ID
	})
	for _, edge := range edges {
		if err := ledger.restoreEdge(edge, state.Version); err != nil {
			return nil, err
		}
	}
	for _, supplied := range state.Entries {
		if supplied.Refs == nil {
			return nil, fmt.Errorf("%w: entry %q references must be an explicit array", ErrInvalidState, supplied.ID)
		}
		entry := cloneEntry(supplied)
		if err := ledger.validateRestoredEntry(entry, state.Version); err != nil {
			return nil, err
		}
		if _, exists := ledger.entries[entry.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate entry %q", ErrInvalidState, entry.ID)
		}
		ledger.entries[entry.ID] = entry
		ledger.entryRefCount += len(entry.Refs)
	}
	if err := ledger.validateRestoredEntryLinks(); err != nil {
		return nil, err
	}
	if state.Status == LedgerClosed {
		for _, node := range ledger.nodes {
			if node.Status != NodeDone {
				return nil, fmt.Errorf("%w: closed ledger contains unfinished node %q", ErrInvalidState, node.ID)
			}
		}
	}
	ledger.version, ledger.status = state.Version, state.Status
	return ledger, nil
}

func validateRestoredNode(node Node, ledgerVersion uint64) error {
	if err := requireExactText(string(node.ID), "node ID"); err != nil {
		return err
	}
	if err := validateNodeKind(node.Kind); err != nil {
		return err
	}
	if err := requireExactText(node.Title, "node title"); err != nil {
		return err
	}
	if err := validatePriority(node.Priority); err != nil {
		return err
	}
	if node.CreatedBy != AuthorityCode {
		return fmt.Errorf("%w: restored nodes require code creation authority", ErrInvalidState)
	}
	if err := validateNodeStatus(node.Status); err != nil {
		return err
	}
	if node.CreatedVersion == 0 || node.CreatedVersion > node.UpdatedVersion || node.UpdatedVersion > ledgerVersion {
		return fmt.Errorf("%w: node %q has invalid materialization versions", ErrInvalidState, node.ID)
	}
	if err := validateOptionalStep(node.CreatedStepID, "created step ID"); err != nil {
		return err
	}
	if err := validateOptionalStep(node.AssignedStepID, "assigned step ID"); err != nil {
		return err
	}
	if err := validateOptionalStep(node.CompletedStepID, "completed step ID"); err != nil {
		return err
	}
	if node.Status != NodeDone && node.CompletedStepID != nil {
		return fmt.Errorf("%w: non-done node %q cannot carry a completed step", ErrInvalidState, node.ID)
	}
	if node.VerificationRefs == nil {
		return fmt.Errorf("%w: node %q verification references must be an explicit array", ErrInvalidState, node.ID)
	}
	if err := validateRefs(node.VerificationRefs); err != nil {
		return fmt.Errorf("%w: node %q verification references are invalid: %v", ErrInvalidState, node.ID, err)
	}
	if node.Status == NodeDone {
		if !hasEvidenceRef(node.VerificationRefs) {
			return fmt.Errorf("%w: %w: completed node %q lacks verification evidence", ErrInvalidState, ErrEvidenceRequired, node.ID)
		}
	} else if len(node.VerificationRefs) != 0 {
		return fmt.Errorf("%w: non-done node %q cannot carry verification references", ErrInvalidState, node.ID)
	}
	if executableNode(node.Kind) {
		if (node.Status == NodeActive || node.Status == NodeBlocked || node.Status == NodeFailed || node.Status == NodeDone) &&
			node.AssignedStepID == nil {
			return fmt.Errorf("%w: executable node %q in status %q is unassigned", ErrInvalidState, node.ID, node.Status)
		}
		if node.Status == NodeDone && (node.AssignedStepID == nil || node.CompletedStepID == nil || *node.AssignedStepID != *node.CompletedStepID) {
			return fmt.Errorf("%w: completed executable node %q has inconsistent step authority", ErrInvalidState, node.ID)
		}
	} else if node.AssignedStepID != nil {
		return fmt.Errorf("%w: aggregate node %q cannot have an assigned step", ErrInvalidState, node.ID)
	} else if node.Status == NodeDone && node.CompletedStepID == nil {
		return fmt.Errorf("%w: completed aggregate node %q requires a verifier step", ErrInvalidState, node.ID)
	}
	if (node.Status == NodeBlocked || node.Status == NodeFailed || node.Status == NodeCanceled) && node.StatusReason == "" {
		return fmt.Errorf("%w: node %q status %q requires a reason", ErrInvalidState, node.ID, node.Status)
	}
	if (node.Status == NodePending || node.Status == NodeActive || node.Status == NodeDone) && node.StatusReason != "" {
		return fmt.Errorf("%w: node %q status %q cannot carry a reason", ErrInvalidState, node.ID, node.Status)
	}
	if node.StatusReason != "" {
		if err := requireExactText(node.StatusReason, "node status reason"); err != nil {
			return fmt.Errorf("%w: node %q status reason is invalid: %v", ErrInvalidState, node.ID, err)
		}
	}
	if err := validateCriteria(node.AcceptanceCriteria); err != nil {
		return err
	}
	if err := node.Metadata.Validate(); err != nil {
		return err
	}
	return nil
}

func validateNodeStatus(status NodeStatus) error {
	switch status {
	case NodePending, NodeReady, NodeActive, NodeBlocked, NodeDone, NodeFailed, NodeCanceled:
		return nil
	default:
		return fmt.Errorf("%w: node status %q is not registered", ErrInvalidState, status)
	}
}
