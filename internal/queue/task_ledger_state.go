package queue

import (
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5/pgconn"
)

func taskLedgerStateNode(state taskstate.MaterializedState, id taskstate.NodeID) (taskstate.Node, error) {
	for _, node := range state.Nodes {
		if node.ID == id {
			return node, nil
		}
	}
	return taskstate.Node{}, fmt.Errorf("%w: materialized task node %q is missing", taskstate.ErrInvalidState, id)
}

func taskLedgerStateEdge(state taskstate.MaterializedState, id taskstate.EdgeID) (taskstate.Edge, error) {
	for _, edge := range state.Edges {
		if edge.ID == id {
			return edge, nil
		}
	}
	return taskstate.Edge{}, fmt.Errorf("%w: materialized task edge %q is missing", taskstate.ErrInvalidState, id)
}

func taskLedgerStateEntry(state taskstate.MaterializedState, id taskstate.EntryID) (taskstate.Entry, error) {
	for _, entry := range state.Entries {
		if entry.ID == id {
			return entry, nil
		}
	}
	return taskstate.Entry{}, fmt.Errorf("%w: materialized task entry %q is missing", taskstate.ErrInvalidState, id)
}

func nullableTaskText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func requireTaskLedgerRow(result pgconn.CommandTag, err error, action string) error {
	if err != nil {
		return fmt.Errorf("%s: %w", action, err)
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("%w: %s affected %d rows", taskstate.ErrInvalidState, action, result.RowsAffected())
	}
	return nil
}
