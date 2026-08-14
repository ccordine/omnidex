package queue

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestPostgresScrumTypedCursorAuthority(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repo := New(pool)
	if err := repo.EnsureSchema(ctx, loadMigrationBundleThroughPrefix(t, "089")); err != nil {
		t.Fatal(err)
	}
	project, err := repo.CreateProject(ctx, "typed Scrum", filepath.Join(t.TempDir(), "typed-scrum"), "")
	if err != nil {
		t.Fatal(err)
	}
	const markerContent = "[[agent-stream-len:42]]\n[[context-sync:7]]"
	card, err := repo.CreateScrumCard(ctx, project.ID, "typed-card", "Typed card", "", "assigned", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	appendScrumMessagesForTest(t, repo, project.ID, card.ID, []ScrumCardMessageAppend{{
		ID: "marker-message", Role: "assistant", Content: markerContent,
	}})
	page, err := repo.ScrumChannelPage(ctx, project.ID, card.ID, 1, -1)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 || page.Messages[0].Role != "assistant" || page.Messages[0].Content != markerContent {
		t.Fatalf("marker-like prose changed: %+v", page.Messages)
	}

	if _, err := pool.Exec(ctx, `
		UPDATE scrum_cards
		SET job_id='101', column_name='in_progress', play_state='running'
		WHERE project_id=$1 AND id=$2
	`, project.ID, card.ID); err == nil {
		t.Fatal("active card without typed cursor authority was accepted")
	}
	if _, err := pool.Exec(ctx, `
		UPDATE scrum_cards
		SET job_id='101', sync_job_id='102'
		WHERE project_id=$1 AND id=$2
	`, project.ID, card.ID); err == nil {
		t.Fatal("foreign cursor job authority was accepted")
	}
	if _, err := pool.Exec(ctx, `
		UPDATE scrum_cards
		SET job_id='101', sync_job_id='101', column_name='in_progress', play_state='running',
		    step_context_cursor=7
		WHERE project_id=$1 AND id=$2
	`, project.ID, card.ID); err != nil {
		t.Fatalf("valid typed cursor authority rejected: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		UPDATE projects SET settings='{"scrum_auto_review":{"enabled":true}}'::jsonb WHERE id=$1
	`, project.ID); err == nil {
		t.Fatal("removed auto-review setting was accepted")
	}
}
