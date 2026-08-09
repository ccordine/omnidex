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
	card, err := scanDBScrumCard(tx.QueryRow(ctx, `
		UPDATE scrum_cards
		SET chat=$3::jsonb, column_name=$4, job_id=$5, console_log=$6,
		    play_state=$7, queue_order=$8, updated_at=NOW()
		WHERE project_id=$1 AND id=$2 AND updated_at=$9
		RETURNING id, project_id, title, description, column_name, checklist, ref_files, chat,
		          model_config, agent_config, card_ticket, card_prompt, recipe_id, recipe,
		          tags, planning_chat, coach_config, test_criteria, flow_metrics,
		          job_id, tags_job_id, ticket_job_id, console_log, play_state, queue_order,
		          board_order, created_at, updated_at
	`, current.ProjectID, current.ID, string(update.Chat), update.Column, update.JobID,
		update.ConsoleLog, update.PlayState, update.QueueOrder, current.UpdatedAt))
	if err != nil {
		return DBScrumCard{}, fmt.Errorf("apply Scrum channel card mutation: %w", err)
	}
	tag, err := tx.Exec(ctx, `
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
