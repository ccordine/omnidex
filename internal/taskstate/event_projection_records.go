package taskstate

func validateProjectedNodeAdded(event Event) error {
	if event.Authority != AuthorityCode || event.Node == nil {
		return invalidEvent("node-added event requires code authority and one node")
	}
	node := *event.Node
	if err := requireExactText(string(node.ID), "node ID"); err != nil {
		return invalidEvent("%v", err)
	}
	for _, identity := range []struct {
		value NodeID
		field string
	}{{node.ParentID, "parent ID"}, {node.ObjectiveID, "objective ID"}} {
		if identity.value != "" {
			if err := requireExactText(string(identity.value), identity.field); err != nil {
				return invalidEvent("%v", err)
			}
			if identity.value == node.ID {
				return invalidEvent("node cannot reference itself as %s", identity.field)
			}
		}
	}
	if err := validateNodeKind(node.Kind); err != nil {
		return invalidEvent("%v", err)
	}
	if err := requireExactText(node.Title, "node title"); err != nil {
		return invalidEvent("%v", err)
	}
	if err := validatePriority(node.Priority); err != nil {
		return invalidEvent("%v", err)
	}
	if node.Status != NodePending || node.CreatedBy != AuthorityCode ||
		node.AssignedStepID != nil || node.CompletedStepID != nil || node.StatusReason != "" ||
		node.CreatedVersion != event.Version || node.UpdatedVersion != event.Version {
		return invalidEvent("new node projection has invalid authority, status, steps, reason, or versions")
	}
	if node.VerificationRefs == nil || len(node.VerificationRefs) != 0 {
		return invalidEvent("new node verification references must be an empty array")
	}
	if node.Kind == NodeGoal && (node.ParentID != "" || node.ObjectiveID != "") {
		return invalidEvent("goal node cannot have parent or objective identity")
	}
	if !equalInt64Pointers(node.CreatedStepID, event.StepID) {
		return invalidEvent("node created step does not match event step")
	}
	if node.AcceptanceCriteria == nil {
		return invalidEvent("node acceptance criteria must be an array")
	}
	if err := validateCriteria(node.AcceptanceCriteria); err != nil {
		return invalidEvent("%v", err)
	}
	if err := node.Metadata.Validate(); err != nil {
		return invalidEvent("node metadata is invalid: %v", err)
	}
	return nil
}

func validateProjectedEdgeAdded(event Event) error {
	if event.Authority != AuthorityCode || event.Edge == nil {
		return invalidEvent("edge-added event requires code authority and one edge")
	}
	edge := *event.Edge
	for _, identity := range []struct {
		value string
		field string
	}{{string(edge.ID), "edge ID"}, {string(edge.From), "edge source"}, {string(edge.To), "edge target"}} {
		if err := requireExactText(identity.value, identity.field); err != nil {
			return invalidEvent("%v", err)
		}
	}
	if edge.From == edge.To || edge.CreatedVersion != event.Version {
		return invalidEvent("new edge projection has invalid endpoints or version")
	}
	if err := validateEdgeKind(edge.Kind); err != nil {
		return invalidEvent("%v", err)
	}
	return nil
}

func validateProjectedEntryAdded(event Event) error {
	if event.Entry == nil {
		return invalidEvent("entry-added event requires one entry")
	}
	entry := *event.Entry
	if err := validateNewEntryAuthority(event.Authority, entry.Kind); err != nil {
		return invalidEvent("entry authority is invalid: %v", err)
	}
	if entry.Authority != event.Authority {
		return invalidEvent("ordinary entry authority does not match the event actor")
	}
	if err := validateProjectedEntryCore(entry, event); err != nil {
		return err
	}
	if entry.Provenance != (EntryProvenance{}) || entry.SupersedesID != "" || entry.SupersededBy != "" {
		return invalidEvent("new ordinary entry contains accepted-decision or supersession state")
	}
	return nil
}

func validateProjectedEntryCore(entry Entry, event Event) error {
	if err := requireExactText(string(entry.ID), "entry ID"); err != nil {
		return invalidEvent("%v", err)
	}
	if entry.ScopeNodeID != "" {
		if err := requireExactText(string(entry.ScopeNodeID), "entry scope node ID"); err != nil {
			return invalidEvent("%v", err)
		}
	}
	if entry.Status != EntryActive || entry.CreatedBy != event.Authority ||
		entry.ContentSHA256 != contentDigest(entry.Content) || entry.DispositionReason != "" ||
		entry.DispositionBy != "" ||
		entry.CreatedVersion != event.Version || entry.UpdatedVersion != event.Version {
		return invalidEvent("new entry projection has invalid status, creator, content, disposition, or versions")
	}
	if err := requireExactText(entry.Content, "entry content"); err != nil {
		return invalidEvent("%v", err)
	}
	if entry.Confidence != nil && (*entry.Confidence < 0 || *entry.Confidence > 1) {
		return invalidEvent("entry confidence is outside zero to one")
	}
	if !equalInt64Pointers(entry.CreatedStepID, event.StepID) {
		return invalidEvent("entry created step does not match event step")
	}
	if entry.Refs == nil {
		return invalidEvent("entry references must be an array")
	}
	if err := entry.Metadata.Validate(); err != nil {
		return invalidEvent("entry metadata is invalid: %v", err)
	}
	if err := validateRefs(entry.Refs); err != nil {
		return invalidEvent("entry references are invalid: %v", err)
	}
	if err := validateFeedback(entry.Kind, entry.FeedbackPurpose, entry.CreatedBy); err != nil {
		return invalidEvent("entry feedback contract is invalid: %v", err)
	}
	if entry.Kind == EntryFact && !hasEvidenceRef(entry.Refs) {
		return invalidEvent("fact entry requires evidence")
	}
	return nil
}

func validateProjectedDecisionAccepted(event Event) error {
	if event.Authority != AuthorityCode && event.Authority != AuthorityUser || event.Entry == nil {
		return invalidEvent("decision acceptance requires code or user authority and one accepted entry")
	}
	if err := requireEventEntryAndReplacement(event); err != nil {
		return err
	}
	entry := *event.Entry
	if entry.ID != event.ReplacementID || entry.Kind != EntryAcceptedDecision ||
		entry.Authority != AuthorityAcceptedModelDecision || entry.SupersedesID != "" ||
		entry.SupersededBy != "" || entry.Provenance.SourceEntryID != event.EntryID ||
		entry.Provenance.AcceptancePolicy != event.Reason || entry.Provenance.AcceptedBy != event.Authority {
		return invalidEvent("accepted decision identity or provenance is inconsistent")
	}
	if event.Reason == "" {
		return invalidEvent("decision acceptance requires a policy")
	}
	if !hasEvidenceRef(entry.Refs) {
		return invalidEvent("accepted decision requires acceptance evidence")
	}
	return validateProjectedEntryCore(entry, event)
}

func requireEventEntryAndReason(event Event) error {
	if err := requireExactText(string(event.EntryID), "event entry ID"); err != nil {
		return invalidEvent("%v", err)
	}
	if event.Reason == "" {
		return invalidEvent("entry event requires an entry ID and reason")
	}
	return nil
}

func requireEventEntryAndReplacement(event Event) error {
	if err := requireExactText(string(event.EntryID), "event entry ID"); err != nil {
		return invalidEvent("%v", err)
	}
	if err := requireExactText(string(event.ReplacementID), "event replacement ID"); err != nil {
		return invalidEvent("%v", err)
	}
	if event.EntryID == event.ReplacementID {
		return invalidEvent("entry supersession requires two different entry identities")
	}
	return nil
}
