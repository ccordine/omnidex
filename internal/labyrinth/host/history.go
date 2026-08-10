package host

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/jackc/pgx/v5"
)

func loadSuccessfulHistory(
	ctx context.Context,
	tx pgx.Tx,
	schema string,
	episode cognition.EpisodeRef,
) ([]storedAction, error) {
	rows, err := tx.Query(ctx, `
		SELECT action_id,request_sha256,expected_number,expected_sha256,
		       action_json,action_json_sha256,
		       actor_job_id,actor_generation,actor_step_id,actor_attempt,actor_worker_id,
		       outcome,result_number,result_sha256,
		       transition_json,transition_json_sha256,failure_json,failure_json_sha256
		FROM `+qualifiedHostRelation(schema, "action_receipts")+`
		WHERE episode_id=$1 AND outcome='transition'
		ORDER BY result_number
	`, episode.ID)
	if err != nil {
		return nil, fmt.Errorf("load labyrinth successful history: %w", err)
	}
	defer rows.Close()
	history := make([]storedAction, 0)
	for rows.Next() {
		receipt, found, err := scanActionRow(rows, episode)
		if err != nil {
			return nil, err
		}
		if !found || receipt.ResultNumber == nil {
			return nil, fmt.Errorf("%w: successful history row disappeared", ErrReceiptCorrupt)
		}
		history = append(history, receipt)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate labyrinth successful history: %w", err)
	}
	return history, nil
}
