package taskstate

import "fmt"

func (ledger *Ledger) restoreNodeSupersessions(
	values []NodeGenerationSupersession,
	ledgerVersion uint64,
) error {
	previous := NodeID("")
	for index, value := range values {
		if err := requireExactText(string(value.NodeID), "superseded node ID"); err != nil {
			return err
		}
		if index > 0 && value.NodeID <= previous {
			return fmt.Errorf("%w: node supersessions must be unique and sorted", ErrInvalidState)
		}
		previous = value.NodeID
		node, exists := ledger.nodes[value.NodeID]
		if !exists || node.Kind == NodeGoal {
			return fmt.Errorf("%w: superseded node %q is missing or is a goal", ErrInvalidState, value.NodeID)
		}
		if err := validateGenerationSuccessor(value.RetiringGeneration, value.SupersededAtGeneration); err != nil {
			return fmt.Errorf("%w: node %q generation: %v", ErrInvalidState, value.NodeID, err)
		}
		if err := requireExactText(value.Reason, "node-generation supersession reason"); err != nil {
			return err
		}
		if value.CreatedVersion <= node.CreatedVersion || value.CreatedVersion > ledgerVersion {
			return fmt.Errorf("%w: node %q supersession version is invalid", ErrInvalidState, value.NodeID)
		}
		if !terminalNode(node.Status) {
			return fmt.Errorf("%w: superseded node %q is not terminal", ErrInvalidState, value.NodeID)
		}
		if node.Status == NodeCanceled && node.UpdatedVersion == value.CreatedVersion && node.StatusReason != value.Reason {
			return fmt.Errorf("%w: node %q cancellation does not bind supersession", ErrInvalidState, value.NodeID)
		}
		ledger.nodeSupersessions[value.NodeID] = value
	}
	for id, node := range ledger.nodes {
		if ledger.nodeSuperseded(id) {
			continue
		}
		for _, related := range []NodeID{node.ParentID, node.ObjectiveID} {
			if related != "" && ledger.nodeSuperseded(related) {
				return fmt.Errorf("%w: current node %q references superseded node %q", ErrInvalidState, id, related)
			}
		}
	}
	for _, edge := range ledger.edges {
		if edge.Kind != EdgeDependsOn && edge.Kind != EdgeBlocks {
			continue
		}
		if ledger.nodeSuperseded(edge.From) != ledger.nodeSuperseded(edge.To) {
			return fmt.Errorf("%w: execution edge %q crosses current and superseded nodes", ErrInvalidState, edge.ID)
		}
	}
	return nil
}
