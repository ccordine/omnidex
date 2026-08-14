package queue

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func updateScrumCardFieldsTx(ctx context.Context, tx pgx.Tx, card DBScrumCard) error {
	tag, err := tx.Exec(ctx, `
		UPDATE scrum_cards SET title=$3,description=$4,column_name=$5,
		 checklist=$6::jsonb,ref_files=$7::jsonb,card_ticket=$8,card_prompt=$9,
		 tags=$10::jsonb,test_criteria=$11::jsonb,job_id=$12,
		 play_state=$13,queue_order=$14,board_order=$15,
		 sync_job_id=$16,step_context_cursor=$17,updated_at=NOW()
		WHERE project_id=$1 AND id=$2
	`, card.ProjectID, card.ID, card.Title, card.Description, card.Column,
		string(card.Checklist), string(card.RefFiles), card.CardTicket, card.CardPrompt,
		string(defaultJSON(card.Tags, `[]`)), string(defaultJSON(card.TestCriteria, `[]`)),
		card.JobID, card.PlayState, card.QueueOrder,
		card.BoardOrder, card.SyncJobID, card.StepContextCursor)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("Scrum card mutation did not update exactly one card")
	}
	return nil
}
