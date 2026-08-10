package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/cognition"
	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func (r *Repository) UnresolvedCognitionAction(
	ctx context.Context,
	authority model.StepAttemptAuthority,
	episodeID cognition.EpisodeID,
) (*CognitionActionRecord, error) {
	if ctx == nil || r == nil || r.pool == nil {
		return nil, fmt.Errorf("unresolved cognition action read requires PostgreSQL and context")
	}
	if err := cognitionEpisodeIdentityValid(episodeID); err != nil {
		return nil, err
	}
	if err := validateStepAttemptAuthority(authority); err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	if _, status, _, err := requireActiveStepAttemptTx(ctx, tx, authority); err != nil {
		return nil, err
	} else if status != model.StepStatusRunning {
		return nil, staleStepAttemptError(authority, "cognition recovery step is not running", nil)
	}
	episode, found, err := loadCognitionEpisodeTx(ctx, tx, episodeID, true)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("%w: %s", ErrCognitionEpisodeNotFound, episodeID)
	}
	if err := cognitionAuthorityMatches(authority, episode); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT action_id FROM cognition_actions
		WHERE episode_id=$1 AND status IN ('prepared','dispatched')
		ORDER BY created_at,action_id LIMIT 2
	`, episodeID)
	if err != nil {
		return nil, err
	}
	ids := make([]cognition.ActionID, 0, 2)
	for rows.Next() {
		var id cognition.ActionID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(ids) > 1 {
		return nil, fmt.Errorf("%w: episode %q has multiple unresolved actions", ErrCognitionConflict, episodeID)
	}
	if len(ids) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, nil
	}
	record, found, err := loadCognitionActionTx(ctx, tx, ids[0], false)
	if err != nil {
		return nil, err
	}
	if !found || (record.Status != CognitionActionPrepared && record.Status != CognitionActionDispatched) {
		return nil, fmt.Errorf("%w: unresolved cognition action projection changed", ErrCognitionConflict)
	}
	reauthorized, err := record.ActionFor(authority)
	if err != nil {
		return nil, err
	}
	record.Action = reauthorized
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &record, nil
}
