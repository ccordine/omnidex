package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func loadTaskLedgerNodeVerificationRefs(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID taskstate.LedgerID,
	nodes []taskstate.Node,
) error {
	index := make(map[taskstate.NodeID]int, len(nodes))
	for nodeIndex := range nodes {
		index[nodes[nodeIndex].ID] = nodeIndex
		nodes[nodeIndex].VerificationRefs = make([]taskstate.Ref, 0)
	}
	rows, err := tx.Query(ctx, `
		SELECT node_id, uri, version, content_sha256, relation, position, created_version
		FROM task_node_verification_refs
		WHERE ledger_id=$1
		ORDER BY node_id, position
		LIMIT $2
	`, ledgerID, maxTaskLedgerNodeRefs+1)
	if err != nil {
		return fmt.Errorf("read task ledger %q node verification references: %w", ledgerID, err)
	}
	defer rows.Close()
	refCount := 0
	for rows.Next() {
		refCount++
		if refCount > maxTaskLedgerNodeRefs {
			return fmt.Errorf(
				"%w: task ledger %q exceeds the %d-node-reference limit",
				taskstate.ErrInvalidState, ledgerID, maxTaskLedgerNodeRefs,
			)
		}
		var nodeID taskstate.NodeID
		var ref taskstate.Ref
		var relation string
		var position int
		var createdVersion int64
		if err := rows.Scan(
			&nodeID, &ref.URI, &ref.Version, &ref.Hash, &relation, &position, &createdVersion,
		); err != nil {
			return fmt.Errorf("scan task ledger %q node verification reference: %w", ledgerID, err)
		}
		nodeIndex, exists := index[nodeID]
		if !exists {
			return fmt.Errorf("%w: task ledger %q reference targets missing node %q", taskstate.ErrInvalidState, ledgerID, nodeID)
		}
		if createdVersion <= 0 || uint64(createdVersion) != nodes[nodeIndex].UpdatedVersion {
			return fmt.Errorf("%w: task ledger %q node %q reference version is inconsistent", taskstate.ErrInvalidState, ledgerID, nodeID)
		}
		if position != len(nodes[nodeIndex].VerificationRefs) {
			return fmt.Errorf("%w: task ledger %q node %q reference positions are not contiguous", taskstate.ErrInvalidState, ledgerID, nodeID)
		}
		ref.Relation = taskstate.RefRelation(relation)
		nodes[nodeIndex].VerificationRefs = append(nodes[nodeIndex].VerificationRefs, ref)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate task ledger %q node verification references: %w", ledgerID, err)
	}
	return nil
}

func insertTaskLedgerNodeVerificationRefs(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID taskstate.LedgerID,
	jobID int64,
	node taskstate.Node,
) error {
	for position, ref := range node.VerificationRefs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO task_node_verification_refs (
				ledger_id, job_id, node_id, uri, version, content_sha256, relation,
				position, created_version
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		`, ledgerID, jobID, node.ID, ref.URI, ref.Version, ref.Hash, ref.Relation,
			position, int64(node.UpdatedVersion)); err != nil {
			return fmt.Errorf("insert task ledger %q node %q verification reference: %w", ledgerID, node.ID, err)
		}
	}
	return nil
}
