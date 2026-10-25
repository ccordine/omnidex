package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// Restore uses the accepted snapshot, not model-reproduced source text. The
// unapproved edits and the owner's restoration remain in revision history.
func acceptedCodingLedgerRestoresTx(ctx context.Context, tx pgx.Tx, ledgerID, baseRevision int64, ledger model.EngineeringLedger, operations []model.LedgerMutationOperation) (map[model.LedgerItemID]model.LedgerTask, error) {
	restored := make(map[model.LedgerItemID]model.LedgerTask)
	for _, operation := range operations {
		if operation.Kind != model.LedgerMutationRestore {
			continue
		}
		disposition, found := codingLedgerItemDisposition(ledger, operation.TargetID)
		if !found || disposition != model.LedgerDispositionActive {
			continue // Ordinary tombstone restoration keeps its existing behavior.
		}
		var payload []byte
		err := tx.QueryRow(ctx, `SELECT version.payload
		 FROM coding_agent_tasks task
		 JOIN coding_ledger_revision_items projection
		   ON projection.ledger_id=task.ledger_id AND projection.revision=task.frozen_revision
		  AND projection.item_id=task.task_item_id
		 JOIN coding_ledger_item_versions version
		   ON (version.ledger_id,version.item_id,version.revision)=
		      (projection.ledger_id,projection.item_id,projection.item_revision)
		 WHERE task.ledger_id=$1 AND task.task_item_id=$2 AND task.state='accepted'
		 AND task.frozen_revision=(SELECT max(revision) FROM coding_ledger_revisions
		   WHERE ledger_id=$1 AND revision<=$3 AND state='frozen')`, ledgerID, operation.TargetID, baseRevision).Scan(&payload)
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("task %s has no accepted contract in the last frozen ledger", operation.TargetID)
		}
		if err != nil {
			return nil, err
		}
		var task model.LedgerTask
		if err := json.Unmarshal(payload, &task); err != nil {
			return nil, err
		}
		// Restoring changes the draft's content. Its normal proposal approval
		// still applies; execution acceptance is carried by the exact contract.
		task.ReviewState = model.LedgerReviewProposed
		// Ordering is independent from the accepted task contract.
		for _, current := range ledger.Tasks {
			if current.ID == task.ID {
				task.Rank = current.Rank
				break
			}
		}
		restored[operation.TargetID] = task
	}
	return restored, nil
}
