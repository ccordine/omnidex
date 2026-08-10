package taskstate

import "fmt"

const (
	MaxLedgerNodes                = 4096
	MaxLedgerNodeVerificationRefs = 16384
	MaxLedgerEdges                = 16384
	MaxLedgerEntries              = 8192
	MaxLedgerEntryRefs            = 32768
	MaxLedgerNodeSupersessions    = MaxLedgerNodes
)

// ValidateMaterializedState validates the complete normalized aggregate without
// granting callers a mutable, command-applying Ledger.
func ValidateMaterializedState(state MaterializedState) error {
	_, err := RestoreLedger(state)
	return err
}

func validateMaterializedCapacity(state MaterializedState) error {
	if err := validateAggregateCounts(
		state.ID, len(state.Nodes), len(state.Edges), len(state.Entries), len(state.NodeSupersessions), 0, 0,
	); err != nil {
		return err
	}
	nodeRefs := 0
	for _, node := range state.Nodes {
		nodeRefs += len(node.VerificationRefs)
		if nodeRefs > MaxLedgerNodeVerificationRefs {
			return aggregateLimitError(state.ID, "node verification references", MaxLedgerNodeVerificationRefs)
		}
	}
	entryRefs := 0
	for _, entry := range state.Entries {
		entryRefs += len(entry.Refs)
		if entryRefs > MaxLedgerEntryRefs {
			return aggregateLimitError(state.ID, "entry references", MaxLedgerEntryRefs)
		}
	}
	return validateAggregateCounts(
		state.ID, len(state.Nodes), len(state.Edges), len(state.Entries), len(state.NodeSupersessions), nodeRefs, entryRefs,
	)
}

func (ledger *Ledger) validateAggregateCapacity() error {
	return validateAggregateCounts(
		ledger.id, len(ledger.nodes), len(ledger.edges), len(ledger.entries), len(ledger.nodeSupersessions),
		ledger.nodeRefCount, ledger.entryRefCount,
	)
}

func (ledger *Ledger) validateProjectedAggregateCapacity(event Event) error {
	nodes, edges, entries := len(ledger.nodes), len(ledger.edges), len(ledger.entries)
	nodeRefs, entryRefs := ledger.nodeRefCount, ledger.entryRefCount
	switch event.Kind {
	case EventNodeAdded:
		nodes++
	case EventEdgeAdded:
		edges++
	case EventEntryAdded, EventDecisionAccepted:
		entries++
		if event.Entry != nil {
			entryRefs += len(event.Entry.Refs)
		}
	case EventEntryResolved:
		entryRefs += len(event.VerificationRefs)
	case EventNodeTransitioned:
		if event.ToStatus == NodeDone {
			nodeRefs += len(event.VerificationRefs)
		}
	}
	supersessions := len(ledger.nodeSupersessions)
	if event.Kind == EventNodeGenerationSuperseded {
		supersessions += len(event.NodeIDs)
	}
	return validateAggregateCounts(ledger.id, nodes, edges, entries, supersessions, nodeRefs, entryRefs)
}

func validateAggregateCounts(
	ledgerID LedgerID,
	nodes, edges, entries, supersessions, nodeRefs, entryRefs int,
) error {
	limits := []struct {
		count   int
		limit   int
		subject string
	}{
		{nodes, MaxLedgerNodes, "nodes"},
		{edges, MaxLedgerEdges, "edges"},
		{entries, MaxLedgerEntries, "entries"},
		{supersessions, MaxLedgerNodeSupersessions, "node supersessions"},
		{nodeRefs, MaxLedgerNodeVerificationRefs, "node verification references"},
		{entryRefs, MaxLedgerEntryRefs, "entry references"},
	}
	for _, bounded := range limits {
		if bounded.count < 0 || bounded.count > bounded.limit {
			return aggregateLimitError(ledgerID, bounded.subject, bounded.limit)
		}
	}
	return nil
}

func aggregateLimitError(ledgerID LedgerID, subject string, limit int) error {
	return fmt.Errorf(
		"%w: task ledger %q exceeds the %d-%s limit",
		ErrInvalidState, ledgerID, limit, subject,
	)
}
