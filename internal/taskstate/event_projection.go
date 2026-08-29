package taskstate

import "fmt"

type eventProjectionField uint32

const (
	eventFieldStep eventProjectionField = 1 << iota
	eventFieldNode
	eventFieldEdge
	eventFieldEntry
	eventFieldNodeID
	eventFieldNodeIDs
	eventFieldEntryID
	eventFieldReplacementID
	eventFieldStatuses
	eventFieldVerificationRefs
	eventFieldLedgerStatus
	eventFieldReason
	eventFieldGenerations
)

// ValidateEventProjection validates the complete stateless event contract shared by
// in-memory replay, PostgreSQL persistence, and paginated audit reads. Relationships
// that require current ledger state remain the responsibility of Ledger.applyEvent.
func ValidateEventProjection(event Event) error {
	if !ledgerIDPattern.MatchString(string(event.LedgerID)) {
		return invalidEvent("ledger ID must match ledger_ plus 64 lowercase hex characters")
	}
	if event.Version == 0 {
		return invalidEvent("ledger version must be positive")
	}
	if !commandIDPattern.MatchString(string(event.CommandID)) || !validDigest(event.CommandSHA256) {
		return invalidEvent("command identity is invalid")
	}
	if err := validateAuthority(event.Authority); err != nil {
		return invalidEvent("actor is invalid: %v", err)
	}
	if err := validateCommandEventKinds(event.CommandKind, event.Kind); err != nil {
		return invalidEvent("%v", err)
	}
	if err := validateOptionalStep(event.StepID, "event step ID"); err != nil {
		return invalidEvent("%v", err)
	}
	if event.Reason != "" {
		if err := requireExactText(event.Reason, "event reason"); err != nil {
			return invalidEvent("%v", err)
		}
	}

	allowed, err := validateEventPayload(event)
	if err != nil {
		return err
	}
	return rejectUnexpectedEventProjection(event, allowed)
}

func validateEventPayload(event Event) (eventProjectionField, error) {
	switch event.Kind {
	case EventNodeAdded:
		return eventFieldNode | eventFieldStep, validateProjectedNodeAdded(event)
	case EventEdgeAdded:
		return eventFieldEdge, validateProjectedEdgeAdded(event)
	case EventEntryAdded:
		return eventFieldEntry | eventFieldStep, validateProjectedEntryAdded(event)
	case EventEntryRejected:
		if event.Authority != AuthorityCode && event.Authority != AuthorityUser {
			return 0, invalidEvent("entry rejection requires code or user authority")
		}
		if err := requireEventEntryAndReason(event); err != nil {
			return 0, err
		}
		if err := validateRefs(event.VerificationRefs); err != nil {
			return 0, invalidEvent("entry rejection evidence is invalid: %v", err)
		}
		return eventFieldEntryID | eventFieldVerificationRefs | eventFieldReason, nil
	case EventEntryResolved:
		if event.Authority != AuthorityCode {
			return 0, invalidEvent("entry resolution requires code authority")
		}
		if err := requireEventEntryAndReason(event); err != nil {
			return 0, err
		}
		if err := validateRefs(event.VerificationRefs); err != nil || !hasEvidenceRef(event.VerificationRefs) {
			return 0, invalidEvent("entry resolution requires valid evidence references: %v", err)
		}
		return eventFieldEntryID | eventFieldVerificationRefs | eventFieldReason, nil
	case EventEntrySuperseded:
		if event.Authority != AuthorityCode && event.Authority != AuthorityUser {
			return 0, invalidEvent("entry supersession requires code or user authority")
		}
		if err := requireEventEntryAndReplacement(event); err != nil {
			return 0, err
		}
		if event.Reason == "" {
			return 0, invalidEvent("entry supersession requires a reason")
		}
		return eventFieldEntryID | eventFieldReplacementID | eventFieldReason, nil
	case EventNodesReadied:
		return eventFieldNodeIDs, validateProjectedNodesReadied(event)
	case EventNodeStepAssigned:
		if event.Authority != AuthorityCode || event.StepID == nil {
			return 0, invalidEvent("node assignment requires code authority, a node, and a step")
		}
		if err := requireExactText(string(event.NodeID), "assigned node ID"); err != nil {
			return 0, invalidEvent("%v", err)
		}
		return eventFieldNodeID | eventFieldStep, nil
	case EventNodeTransitioned:
		return eventFieldNodeID | eventFieldStatuses | eventFieldStep |
			eventFieldVerificationRefs | eventFieldReason, validateProjectedNodeTransition(event)
	case EventNodeGenerationSuperseded:
		return eventFieldNodeIDs | eventFieldGenerations | eventFieldReason,
			validateProjectedNodeGenerationSupersession(event)
	case EventLedgerClosed:
		return eventFieldLedgerStatus | eventFieldStep | eventFieldReason, validateProjectedLedgerClose(event)
	default:
		return 0, invalidEvent("event kind %q is not registered", event.Kind)
	}
}

func validateProjectedNodeGenerationSupersession(event Event) error {
	if event.Authority != AuthorityCode || len(event.NodeIDs) == 0 {
		return invalidEvent("node-generation supersession requires code authority and nodes")
	}
	if err := validateGenerationSuccessor(event.RetiringGeneration, event.SupersededAtGeneration); err != nil {
		return invalidEvent("%v", err)
	}
	if event.Reason == "" {
		return invalidEvent("node-generation supersession requires a reason")
	}
	seen := make(map[NodeID]struct{}, len(event.NodeIDs))
	previous := NodeID("")
	for index, id := range event.NodeIDs {
		if err := requireExactText(string(id), "superseded node ID"); err != nil {
			return invalidEvent("%v", err)
		}
		if _, duplicate := seen[id]; duplicate || (index > 0 && id <= previous) {
			return invalidEvent("superseded node IDs must be unique and sorted")
		}
		seen[id], previous = struct{}{}, id
	}
	return nil
}

func invalidEvent(format string, values ...any) error {
	return fmt.Errorf("%w: "+format, append([]any{ErrInvalidState}, values...)...)
}
