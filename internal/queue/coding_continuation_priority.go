package queue

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

// Only an incomplete-task handoff against the preceding frozen ledger establishes priority.
// New approved tasks, the interrupted task, and their prerequisites precede
// untouched work; agents do not select or reorder execution.
func codingContinuationPrioritiesTx(ctx context.Context, tx pgx.Tx, ledgerID int64, ledger model.EngineeringLedger) (map[model.LedgerItemID]bool, error) {
	priority := map[model.LedgerItemID]bool{}
	var taskID model.LedgerItemID
	var sourceRevision, priorRevision int64
	err := tx.QueryRow(ctx, `SELECT event.payload_json->>'task_id',
	 COALESCE((event.payload_json->'continuation'->>'ledger_revision')::bigint,0) FROM coding_ledgers ledger
	 JOIN jobs job ON job.id=ledger.job_id JOIN evidence event ON event.job_id=job.id
	 WHERE ledger.id=$1 AND event.kind='coding_agent' AND event.payload_json->>'phase'='task_incomplete'
	   AND (event.payload_json->>'generation')::bigint<job.current_generation
	 ORDER BY event.id DESC LIMIT 1`, ledgerID).Scan(&taskID, &sourceRevision)
	if errors.Is(err, pgx.ErrNoRows) {
		return priority, nil
	}
	if err != nil {
		return nil, err
	}
	if sourceRevision < 1 {
		return nil, fmt.Errorf("continuation priority lost its interrupted frozen revision")
	}
	if err := tx.QueryRow(ctx, `SELECT COALESCE(max(revision),0) FROM coding_ledger_revisions
	 WHERE ledger_id=$1 AND revision<$2 AND state='frozen'`, ledgerID, ledger.Revision).Scan(&priorRevision); err != nil {
		return nil, err
	}
	if sourceRevision != priorRevision {
		return priority, nil // A later approved freeze has already consumed this handoff.
	}
	prior := map[model.LedgerItemID]bool{}
	rows, err := tx.Query(ctx, `SELECT task_item_id FROM coding_agent_tasks WHERE ledger_id=$1 AND frozen_revision=$2`, ledgerID, sourceRevision)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id model.LedgerItemID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		prior[id] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for _, task := range ledger.Tasks {
		if task.Disposition == model.LedgerDispositionActive && (task.ID == taskID || !prior[task.ID]) {
			priority[task.ID] = true
		}
	}
	changed := true
	for changed {
		changed = false
		for _, edge := range ledger.Dependencies {
			if edge.Disposition == model.LedgerDispositionActive && priority[edge.TaskID] && !priority[edge.PrerequisiteID] {
				priority[edge.PrerequisiteID] = true
				changed = true
			}
		}
	}
	return priority, nil
}
