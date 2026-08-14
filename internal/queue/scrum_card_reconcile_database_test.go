package queue

import (
	"encoding/json"
	"testing"
)

func TestPostgresScrumCardReconcileCommitsRowsOutcomeAndMetricsAtomically(t *testing.T) {
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Reconcile", "/tmp/scrum-reconcile", "")
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(ctx, project.ID, "reconcile-card", "Reconcile", "", "in_progress", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE scrum_cards SET play_state='running',job_id='17',sync_job_id='17',
		 updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond')
		WHERE project_id=$1 AND id=$2
	`, project.ID, card.ID); err != nil {
		t.Fatal(err)
	}
	card, err = repository.GetScrumCard(ctx, project.ID, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := repository.ReconcileScrumCard(ctx, ScrumCardReconcileCommand{
		ProjectID: project.ID, CardID: card.ID, ExpectedUpdatedAt: card.UpdatedAt,
		ExpectedJobID: "17", Kind: ScrumReconcileJobTerminal, Column: ScrumCardReview,
		PlayState: "", JobID: "17", SyncJobID: "17", Outcome: "success",
		Messages: []ScrumCardMessageAppend{{
			ID: "chatmsg_terminal", Role: "system", Content: "Job completed",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Column != "review" || updated.PlayState != "" || updated.ChannelMessageCount != 1 {
		t.Fatalf("updated=%+v", updated)
	}
	var metrics ScrumFlowMetrics
	if err := json.Unmarshal(updated.FlowMetrics, &metrics); err != nil {
		t.Fatal(err)
	}
	if metrics.LastPlayOutcome != "success" || metrics.ChannelMessages != 1 || metrics.ConversationChars != int64(len("Job completed")) {
		t.Fatalf("metrics=%+v", metrics)
	}
}

func TestPostgresScrumCardReconcileFailureRollsBackEveryAuthorityRow(t *testing.T) {
	repository, pool, ctx := scrumChannelOperationTestRepository(t)
	project, err := repository.CreateProject(ctx, "Rollback", "/tmp/scrum-reconcile-rollback", "")
	if err != nil {
		t.Fatal(err)
	}
	card, err := repository.CreateScrumCard(ctx, project.ID, "rollback-card", "Rollback", "", "in_progress", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE scrum_cards SET play_state='running',job_id='19',sync_job_id='19',
		 updated_at=GREATEST(clock_timestamp(),updated_at+interval '1 microsecond')
		WHERE project_id=$1 AND id=$2
	`, project.ID, card.ID); err != nil {
		t.Fatal(err)
	}
	card, err = repository.GetScrumCard(ctx, project.ID, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repository.ReconcileScrumCard(ctx, ScrumCardReconcileCommand{
		ProjectID: project.ID, CardID: card.ID, ExpectedUpdatedAt: card.UpdatedAt,
		ExpectedJobID: "19", Kind: ScrumReconcileJobTerminal, Column: ScrumCardReview,
		JobID: "19", SyncJobID: "19", Outcome: "success",
		Messages: []ScrumCardMessageAppend{{
			ID: "chatmsg_invalid", Role: "system", Content: "",
		}},
	}); err == nil {
		t.Fatal("invalid reconciliation was accepted")
	}
	after, err := repository.GetScrumCard(ctx, project.ID, card.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Column != card.Column || after.PlayState != card.PlayState || after.ChannelMessageCount != 0 || !after.UpdatedAt.Equal(card.UpdatedAt) {
		t.Fatalf("failed reconcile mutated card before=%+v after=%+v", card, after)
	}
}
