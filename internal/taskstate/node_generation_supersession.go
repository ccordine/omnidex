package taskstate

import (
	"fmt"
	"math"
	"sort"
)

func (command SupersedeNodeGenerationCommand) decide(ledger *Ledger) (Event, error) {
	if err := requireCode(command.Actor, "supersede a task-node generation"); err != nil {
		return Event{}, err
	}
	if err := validateGenerationSuccessor(command.RetiringGeneration, command.SupersededAtGeneration); err != nil {
		return Event{}, err
	}
	if err := requireExactText(command.Reason, "node-generation supersession reason"); err != nil {
		return Event{}, err
	}
	if len(command.NodeIDs) == 0 || len(command.NodeIDs) > MaxLedgerNodes {
		return Event{}, fmt.Errorf("%w: node-generation supersession requires between 1 and %d nodes", ErrInvalidCommand, MaxLedgerNodes)
	}
	previous := NodeID("")
	for index, id := range command.NodeIDs {
		if err := requireExactText(string(id), "superseded node ID"); err != nil {
			return Event{}, err
		}
		if index > 0 && id <= previous {
			return Event{}, fmt.Errorf("%w: superseded node IDs must be unique and sorted", ErrInvalidCommand)
		}
		previous = id
		node, exists := ledger.nodes[id]
		if !exists {
			return Event{}, fmt.Errorf("%w: node %q", ErrNotFound, id)
		}
		if node.Kind == NodeGoal {
			return Event{}, fmt.Errorf("%w: goal node %q cannot be generation-scoped", ErrInvalidState, id)
		}
		if _, exists := ledger.nodeSupersessions[id]; exists {
			return Event{}, fmt.Errorf("%w: node %q is already superseded", ErrInvalidState, id)
		}
	}
	return Event{
		Kind: EventNodeGenerationSuperseded, NodeIDs: cloneNodeIDs(command.NodeIDs),
		RetiringGeneration:     command.RetiringGeneration,
		SupersededAtGeneration: command.SupersededAtGeneration,
		Reason:                 command.Reason,
	}, nil
}

func (ledger *Ledger) applyNodeGenerationSuperseded(event Event) error {
	if event.Authority != AuthorityCode {
		return fmt.Errorf("node-generation supersession requires code authority")
	}
	if err := validateGenerationSuccessor(event.RetiringGeneration, event.SupersededAtGeneration); err != nil {
		return err
	}
	if err := requireExactText(event.Reason, "node-generation supersession reason"); err != nil {
		return err
	}
	if len(event.NodeIDs) == 0 || len(event.NodeIDs) > MaxLedgerNodes || !sort.SliceIsSorted(event.NodeIDs, func(i, j int) bool {
		return event.NodeIDs[i] < event.NodeIDs[j]
	}) {
		return fmt.Errorf("node-generation supersession node set is invalid")
	}
	previous := NodeID("")
	for index, id := range event.NodeIDs {
		if index > 0 && id == previous {
			return fmt.Errorf("node-generation supersession node set is duplicated")
		}
		previous = id
		node, exists := ledger.nodes[id]
		if !exists || node.Kind == NodeGoal {
			return fmt.Errorf("superseded node %q is missing or is the root goal", id)
		}
		if _, exists := ledger.nodeSupersessions[id]; exists {
			return fmt.Errorf("node %q is already superseded", id)
		}
	}
	for _, id := range event.NodeIDs {
		node := ledger.nodes[id]
		if !terminalNode(node.Status) {
			node.Status = NodeCanceled
			node.StatusReason = event.Reason
			node.UpdatedVersion = event.Version
			ledger.nodes[id] = node
		}
		ledger.nodeSupersessions[id] = NodeGenerationSupersession{
			NodeID: id, RetiringGeneration: event.RetiringGeneration,
			SupersededAtGeneration: event.SupersededAtGeneration,
			Reason:                 event.Reason, CreatedVersion: event.Version,
		}
	}
	return nil
}

func validateGenerationSuccessor(retiring, successor int64) error {
	if retiring <= 0 || retiring == math.MaxInt64 || successor != retiring+1 {
		return fmt.Errorf("%w: superseded generation must be the exact positive successor", ErrInvalidCommand)
	}
	return nil
}

func (ledger *Ledger) nodeSuperseded(id NodeID) bool {
	_, exists := ledger.nodeSupersessions[id]
	return exists
}
