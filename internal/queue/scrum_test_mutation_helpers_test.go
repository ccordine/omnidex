package queue

import (
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func appendScrumMessagesForTest(
	t *testing.T,
	repository *Repository,
	projectID int64,
	cardID string,
	messages []ScrumCardMessageAppend,
) DBScrumCard {
	t.Helper()
	ctx := t.Context()
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := lockScrumCardTx(ctx, tx, projectID, cardID); err != nil {
		t.Fatal(err)
	}
	for index, message := range messages {
		if _, err := insertScrumCardMessageTx(ctx, tx, projectID, cardID, message); err != nil {
			t.Fatalf("append test Scrum message %d: %v", index+1, err)
		}
	}
	if err := refreshScrumFlowMetricsTx(ctx, tx, projectID, cardID); err != nil {
		t.Fatal(err)
	}
	card, err := scanDBScrumCard(tx.QueryRow(ctx, scrumCardSelectSQL, projectID, cardID))
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return card
}

func setScrumCardRunningForTest(
	t *testing.T,
	pool *pgxpool.Pool,
	projectID int64,
	cardID string,
	jobID int64,
) DBScrumCard {
	t.Helper()
	if _, err := pool.Exec(t.Context(), `
		UPDATE scrum_cards SET column_name='in_progress',play_state='running',
		 job_id=$3::text,sync_job_id=$3::text,
		 updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond')
		WHERE project_id=$1 AND id=$2
	`, projectID, cardID, strconv.FormatInt(jobID, 10)); err != nil {
		t.Fatal(err)
	}
	card, err := scanDBScrumCard(pool.QueryRow(t.Context(), scrumCardSelectSQL, projectID, cardID))
	if err != nil {
		t.Fatal(err)
	}
	return card
}
