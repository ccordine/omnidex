package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/taskstate"
	"github.com/jackc/pgx/v5"
)

func loadTaskLedgerNodeSupersessions(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID taskstate.LedgerID,
) ([]taskstate.NodeGenerationSupersession, error) {
	rows, err := tx.Query(ctx, `
		SELECT node_id, retiring_generation, superseded_at_generation, reason, created_version
		FROM task_node_generation_supersessions
		WHERE ledger_id=$1
		ORDER BY node_id ASC
		LIMIT $2
	`, ledgerID, maxTaskLedgerNodeSupersessions+1)
	if err != nil {
		return nil, fmt.Errorf("read task ledger %q node supersessions: %w", ledgerID, err)
	}
	defer rows.Close()
	values := make([]taskstate.NodeGenerationSupersession, 0)
	for rows.Next() {
		var value taskstate.NodeGenerationSupersession
		var version int64
		if err := rows.Scan(
			&value.NodeID, &value.RetiringGeneration, &value.SupersededAtGeneration,
			&value.Reason, &version,
		); err != nil {
			return nil, fmt.Errorf("scan task ledger %q node supersession: %w", ledgerID, err)
		}
		if value.CreatedVersion, err = taskLedgerVersion(version); err != nil {
			return nil, fmt.Errorf("task ledger %q node %q supersession version: %w", ledgerID, value.NodeID, err)
		}
		values = append(values, value)
		if len(values) > maxTaskLedgerNodeSupersessions {
			return nil, fmt.Errorf(
				"%w: task ledger %q exceeds the %d-node-supersession limit",
				taskstate.ErrInvalidState, ledgerID, maxTaskLedgerNodeSupersessions,
			)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate task ledger %q node supersessions: %w", ledgerID, err)
	}
	return values, nil
}

func insertTaskLedgerNodeSupersession(
	ctx context.Context,
	tx pgx.Tx,
	ledgerID taskstate.LedgerID,
	jobID int64,
	jobGeneration int64,
	value taskstate.NodeGenerationSupersession,
) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO task_node_generation_supersessions (
			ledger_id, job_id, job_generation, node_id, retiring_generation,
			superseded_at_generation, reason, created_version
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
	`, ledgerID, jobID, jobGeneration, value.NodeID, value.RetiringGeneration,
		value.SupersededAtGeneration, value.Reason, int64(value.CreatedVersion))
	if err != nil {
		return fmt.Errorf("insert task ledger %q node %q supersession: %w", ledgerID, value.NodeID, err)
	}
	return nil
}
