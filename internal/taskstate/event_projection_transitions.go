package taskstate

func validateProjectedNodesReadied(event Event) error {
	if event.Authority != AuthorityCode || len(event.NodeIDs) == 0 {
		return invalidEvent("node readiness requires code authority and at least one node")
	}
	seen := make(map[NodeID]struct{}, len(event.NodeIDs))
	for _, id := range event.NodeIDs {
		if err := requireExactText(string(id), "readied node ID"); err != nil {
			return invalidEvent("%v", err)
		}
		if _, duplicate := seen[id]; duplicate {
			return invalidEvent("readied node identities must be unique")
		}
		seen[id] = struct{}{}
	}
	return nil
}

func validateProjectedNodeTransition(event Event) error {
	if event.Authority != AuthorityCode {
		return invalidEvent("node transition requires code authority")
	}
	if err := requireExactText(string(event.NodeID), "transition node ID"); err != nil {
		return invalidEvent("%v", err)
	}
	if err := validateNodeStatus(event.FromStatus); err != nil {
		return invalidEvent("transition source status is invalid: %v", err)
	}
	if err := validateNodeStatus(event.ToStatus); err != nil {
		return invalidEvent("transition target status is invalid: %v", err)
	}
	if event.FromStatus == event.ToStatus || terminalNode(event.FromStatus) {
		return invalidEvent("node transition source and target are invalid")
	}
	terminalFailure := event.ToStatus == NodeFailed && len(event.VerificationRefs) == 1
	if event.ToStatus != NodeDone && len(event.VerificationRefs) != 0 && !terminalFailure {
		return invalidEvent("non-completion transition cannot carry verification references")
	}
	switch {
	case terminalFailure:
		if event.Reason == "" || event.VerificationRefs[0].Relation != RefVerifies {
			return invalidEvent("terminal failure requires an exact reason and verifying proof")
		}
		if err := validateRefs(event.VerificationRefs); err != nil {
			return invalidEvent("terminal failure proof is invalid: %v", err)
		}
	case event.FromStatus == NodeReady && event.ToStatus == NodeActive:
		if event.Reason != "" {
			return invalidEvent("activation cannot carry a reason")
		}
	case event.FromStatus == NodeActive && event.ToStatus == NodeDone:
		if event.StepID == nil || event.Reason != "" {
			return invalidEvent("successful completion requires a step and no failure reason")
		}
		if err := validateRefs(event.VerificationRefs); err != nil || !hasEvidenceRef(event.VerificationRefs) {
			return invalidEvent("successful completion requires valid evidence references: %v", err)
		}
	case event.FromStatus == NodeActive && (event.ToStatus == NodeBlocked || event.ToStatus == NodeFailed):
		if event.Reason == "" {
			return invalidEvent("blocked or failed transition requires a reason")
		}
	case event.FromStatus == NodeBlocked && event.ToStatus == NodeReady:
		if event.Reason == "" {
			return invalidEvent("block resolution requires a reason")
		}
	case event.ToStatus == NodeCanceled:
		if event.Reason == "" {
			return invalidEvent("cancellation requires a reason")
		}
	default:
		return invalidEvent("node transition %q to %q is not registered", event.FromStatus, event.ToStatus)
	}
	return nil
}

func validateProjectedLedgerClose(event Event) error {
	if event.Authority != AuthorityCode || !terminalLedger(event.LedgerStatus) {
		return invalidEvent("ledger closure requires code authority and a terminal status")
	}
	if event.Reason == "" {
		return invalidEvent("ledger closure requires a reason")
	}
	return nil
}

func rejectUnexpectedEventProjection(event Event, allowed eventProjectionField) error {
	checks := []struct {
		field   eventProjectionField
		present bool
		name    string
	}{
		{eventFieldStep, event.StepID != nil, "step_id"},
		{eventFieldNode, event.Node != nil, "node"},
		{eventFieldEdge, event.Edge != nil, "edge"},
		{eventFieldEntry, event.Entry != nil, "entry"},
		{eventFieldNodeID, event.NodeID != "", "node_id"},
		{eventFieldNodeIDs, len(event.NodeIDs) != 0, "node_ids"},
		{eventFieldEntryID, event.EntryID != "", "entry_id"},
		{eventFieldReplacementID, event.ReplacementID != "", "replacement_id"},
		{eventFieldStatuses, event.FromStatus != "" || event.ToStatus != "", "node_status"},
		{eventFieldVerificationRefs, len(event.VerificationRefs) != 0, "verification_refs"},
		{eventFieldLedgerStatus, event.LedgerStatus != "", "ledger_status"},
		{eventFieldReason, event.Reason != "", "reason"},
		{eventFieldGenerations, event.RetiringGeneration != 0 || event.SupersededAtGeneration != 0, "generation"},
	}
	for _, check := range checks {
		if check.present && allowed&check.field == 0 {
			return invalidEvent("event %q contains forbidden projection %s", event.Kind, check.name)
		}
	}
	return nil
}
