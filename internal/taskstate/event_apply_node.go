package taskstate

import "fmt"

func (ledger *Ledger) applyNodeStepAssigned(event Event) error {
	if event.Authority != AuthorityCode || event.StepID == nil || *event.StepID <= 0 {
		return fmt.Errorf("node step assignment requires code authority and a positive step")
	}
	node, exists := ledger.nodes[event.NodeID]
	if !exists || !executableNode(node.Kind) ||
		(node.Status != NodePending && node.Status != NodeReady) || node.AssignedStepID != nil {
		return fmt.Errorf("node %q cannot receive a step assignment", event.NodeID)
	}
	for _, existing := range ledger.nodes {
		if existing.AssignedStepID != nil && *existing.AssignedStepID == *event.StepID {
			return fmt.Errorf("step %d is already assigned", *event.StepID)
		}
	}
	node.AssignedStepID, node.UpdatedVersion = cloneInt64(event.StepID), event.Version
	ledger.nodes[node.ID] = node
	return nil
}

func (ledger *Ledger) applyNodeTransitioned(event Event) error {
	if event.Authority != AuthorityCode {
		return fmt.Errorf("node transition requires code authority")
	}
	node, exists := ledger.nodes[event.NodeID]
	if !exists || node.Status != event.FromStatus {
		return fmt.Errorf("node transition source is missing or stale")
	}
	completedStep := (*int64)(nil)
	if event.ToStatus == NodeDone {
		completedStep = cloneInt64(event.StepID)
	} else if !equalInt64Pointers(event.StepID, node.AssignedStepID) {
		return fmt.Errorf("non-completion transition step does not match assigned step")
	}
	command := TransitionNodeCommand{
		Actor: event.Authority, NodeID: event.NodeID, To: event.ToStatus,
		CompletedStepID:  completedStep,
		VerificationRefs: cloneRefs(event.VerificationRefs), Reason: event.Reason,
	}
	if err := validateNodeTransition(node, command); err != nil {
		return err
	}
	node.Status, node.StatusReason, node.UpdatedVersion = event.ToStatus, event.Reason, event.Version
	if event.ToStatus == NodeDone {
		node.CompletedStepID = cloneInt64(event.StepID)
		node.VerificationRefs = cloneRefs(event.VerificationRefs)
		ledger.nodeRefCount += len(event.VerificationRefs)
	}
	ledger.nodes[node.ID] = node
	return nil
}

func (ledger *Ledger) applyLedgerClosed(event Event) error {
	if event.Authority != AuthorityCode || !terminalLedger(event.LedgerStatus) {
		return fmt.Errorf("ledger terminalization requires code authority and terminal status")
	}
	if err := requireExactText(event.Reason, "ledger close reason"); err != nil {
		return err
	}
	if err := validateOptionalStep(event.StepID, "ledger close step ID"); err != nil {
		return err
	}
	if event.LedgerStatus == LedgerClosed {
		for _, node := range ledger.nodes {
			if node.Status != NodeDone {
				return fmt.Errorf("successful close requires every node to be done")
			}
		}
	}
	ledger.status = event.LedgerStatus
	return nil
}

func (ledger *Ledger) validateNewEntryProjection(event Event, entry Entry) error {
	if _, exists := ledger.entries[entry.ID]; exists {
		return fmt.Errorf("entry %q already exists", entry.ID)
	}
	if err := requireExactText(string(entry.ID), "entry ID"); err != nil {
		return err
	}
	if entry.ScopeNodeID != "" {
		if _, exists := ledger.nodes[entry.ScopeNodeID]; !exists {
			return fmt.Errorf("scope node %q does not exist", entry.ScopeNodeID)
		}
	}
	if entry.Status != EntryActive || entry.Authority != event.Authority || entry.CreatedBy != event.Authority ||
		entry.CreatedVersion != event.Version || entry.UpdatedVersion != event.Version ||
		entry.ContentSHA256 != contentDigest(entry.Content) || entry.SupersedesID != "" || entry.SupersededBy != "" ||
		entry.DispositionReason != "" || entry.DispositionBy != "" {
		return fmt.Errorf("new entry projection has invalid status, authority, content digest, or versions")
	}
	if err := requireExactText(entry.Content, "entry content"); err != nil {
		return err
	}
	if entry.Confidence != nil && (*entry.Confidence < 0 || *entry.Confidence > 1) {
		return fmt.Errorf("confidence must be between zero and one")
	}
	if err := validateOptionalStep(entry.CreatedStepID, "created step ID"); err != nil {
		return err
	}
	if !equalInt64Pointers(entry.CreatedStepID, event.StepID) {
		return fmt.Errorf("entry created step does not match event step")
	}
	if err := entry.Metadata.Validate(); err != nil {
		return err
	}
	if err := validateRefs(entry.Refs); err != nil {
		return err
	}
	if err := validateFeedback(entry.Kind, entry.FeedbackPurpose, event.Authority); err != nil {
		return err
	}
	if entry.Kind == EntryFact && !hasEvidenceRef(entry.Refs) {
		return fmt.Errorf("fact requires evidence")
	}
	return nil
}
