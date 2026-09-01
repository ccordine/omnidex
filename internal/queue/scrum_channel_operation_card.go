package queue

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/model"
	"github.com/jackc/pgx/v5"
)

func updateScrumChannelCardTx(
	ctx context.Context,
	tx pgx.Tx,
	current DBScrumCard,
	request ScrumChannelOperationRequest,
	job model.Job,
	update ScrumChannelCardUpdate,
) (DBScrumCard, error) {
	if err := validateScrumChannelCardUpdate(request, job, &update); err != nil {
		return DBScrumCard{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE scrum_cards
		SET column_name=$3,job_id=$4,play_state=$5,queue_order=$6,
		 updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond')
		WHERE project_id=$1 AND id=$2 AND updated_at=$7
	`, current.ProjectID, current.ID, update.Column, update.JobID, update.PlayState,
		update.QueueOrder, current.UpdatedAt)
	if err != nil {
		return DBScrumCard{}, fmt.Errorf("apply Scrum channel card mutation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return DBScrumCard{}, fmt.Errorf("Scrum channel card changed before exact mutation apply")
	}
	for _, message := range update.Messages {
		if _, err := insertScrumCardMessageTx(ctx, tx, current.ProjectID, current.ID, message); err != nil {
			return DBScrumCard{}, err
		}
	}
	next := current
	next.Column, next.JobID, next.PlayState, next.QueueOrder = update.Column, update.JobID, update.PlayState, update.QueueOrder
	if err := applyScrumCardStateMetricsTx(ctx, tx, current, next, ""); err != nil {
		return DBScrumCard{}, err
	}
	card, err := scanDBScrumCard(tx.QueryRow(ctx, scrumCardSelectSQL, current.ProjectID, current.ID))
	if err != nil {
		return DBScrumCard{}, err
	}
	tag, err = tx.Exec(ctx, `
		UPDATE projects SET last_seen_at=NOW(), updated_at=NOW() WHERE id=$1
	`, current.ProjectID)
	if err != nil {
		return DBScrumCard{}, fmt.Errorf("touch Scrum channel project: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return DBScrumCard{}, fmt.Errorf("Scrum channel project %d was not updated", current.ProjectID)
	}
	return card, nil
}
