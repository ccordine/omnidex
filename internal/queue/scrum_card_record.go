package queue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type DBScrumCard struct {
	ID                  string          `json:"id"`
	ProjectID           int64           `json:"project_id"`
	Title               string          `json:"title"`
	Description         string          `json:"description"`
	Column              string          `json:"column"`
	Checklist           json.RawMessage `json:"checklist"`
	RefFiles            json.RawMessage `json:"ref_files"`
	CardTicket          string          `json:"card_ticket"`
	CardPrompt          string          `json:"card_prompt"`
	Tags                json.RawMessage `json:"tags"`
	TestCriteria        json.RawMessage `json:"test_criteria"`
	FlowMetrics         json.RawMessage `json:"flow_metrics"`
	JobID               string          `json:"job_id"`
	PlayState           string          `json:"play_state"`
	QueueOrder          int             `json:"queue_order"`
	BoardOrder          int             `json:"board_order"`
	ChannelMessageCount int64           `json:"channel_message_count"`
	ChannelContentBytes int64           `json:"channel_content_bytes"`
	CreatedAt           time.Time       `json:"created_at"`
	UpdatedAt           time.Time       `json:"updated_at"`
}

const scrumCardSelectColumns = `id,project_id,title,description,column_name,checklist,ref_files,
	 card_ticket,card_prompt,tags,test_criteria,flow_metrics,
	 job_id,play_state,queue_order,board_order,
	 channel_message_count,channel_content_bytes,
	 created_at,updated_at`

const scrumCardSelectSQL = `SELECT ` + scrumCardSelectColumns + `
	FROM scrum_cards WHERE project_id=$1 AND id=$2`

func (r *Repository) GetScrumCard(ctx context.Context, projectID int64, cardID string) (DBScrumCard, error) {
	card, err := scanDBScrumCard(r.pool.QueryRow(ctx, scrumCardSelectSQL, projectID, strings.TrimSpace(cardID)))
	if errors.Is(err, pgx.ErrNoRows) {
		return DBScrumCard{}, fmt.Errorf("%w: Scrum card %q was not found in project %d", ErrScrumCardNotFound, cardID, projectID)
	}
	return card, err
}

func (r *Repository) CreateScrumCard(
	ctx context.Context,
	projectID int64,
	cardID, title, description, column string,
	checklist, refFiles json.RawMessage,
) (DBScrumCard, error) {
	title = SanitizeUTF8Text(strings.TrimSpace(title))
	if title == "" {
		return DBScrumCard{}, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(cardID) == "" {
		cardID = fmt.Sprintf("card_%d", time.Now().UnixNano())
	}
	if _, err := ParseScrumCardColumn(column); err != nil {
		return DBScrumCard{}, err
	}
	if len(checklist) == 0 {
		checklist = json.RawMessage(`[]`)
	}
	if len(refFiles) == 0 {
		refFiles = json.RawMessage(`[]`)
	}
	checklist = SanitizeUTF8Bytes(checklist)
	refFiles = SanitizeUTF8Bytes(refFiles)
	if err := validateStoredScrumArray("checklist", checklist); err != nil {
		return DBScrumCard{}, err
	}
	if err := validateStoredScrumArray("ref_files", refFiles); err != nil {
		return DBScrumCard{}, err
	}
	tx, err := r.beginLockedProjectTx(ctx, projectID, "create Scrum card")
	if err != nil {
		return DBScrumCard{}, err
	}
	defer rollbackTx(ctx, tx, "create Scrum card")
	card, err := scanDBScrumCard(tx.QueryRow(ctx, `
		INSERT INTO scrum_cards(id,project_id,title,description,column_name,checklist,ref_files,board_order)
		VALUES($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,
		 COALESCE((SELECT MAX(board_order) FROM scrum_cards WHERE project_id=$2 AND column_name=$5),-1)+1)
		RETURNING `+scrumCardSelectColumns,
		cardID, projectID, title, SanitizeUTF8Text(description), column,
		string(checklist), string(refFiles),
	))
	if err != nil {
		return DBScrumCard{}, err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE projects SET last_seen_at=NOW(),updated_at=NOW() WHERE id=$1
	`, projectID)
	if err != nil {
		return DBScrumCard{}, fmt.Errorf("touch project after Scrum card creation: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return DBScrumCard{}, fmt.Errorf("Scrum card creation project %d disappeared", projectID)
	}
	if err := tx.Commit(ctx); err != nil {
		return DBScrumCard{}, fmt.Errorf("commit Scrum card creation: %w", err)
	}
	return card, nil
}
