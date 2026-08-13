package queue

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"
)

func TestPostgresScrumTypedCursorAuthority(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	pool := openIsolatedMigrationPool(t)
	repo := New(pool)
	if err := repo.EnsureSchema(ctx, loadCheckedMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	project, err := repo.CreateProject(ctx, "typed Scrum", filepath.Join(t.TempDir(), "typed-scrum"), "", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	const markerContent = "[[agent-stream-len:42]]\n[[context-sync:7]]"
	markerChat := json.RawMessage(`[{"role":"assistant","content":"[[agent-stream-len:42]]\n[[context-sync:7]]","created_at":""}]`)
	card, err := repo.CreateScrumCard(ctx, project.ID, "typed-card", "Typed card", "", "assigned", nil, nil, markerChat)
	if err != nil {
		t.Fatal(err)
	}
	var persisted []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(card.Chat, &persisted); err != nil {
		t.Fatalf("decode persisted chat: %v", err)
	}
	if len(persisted) != 1 || persisted[0].Role != "assistant" || persisted[0].Content != markerContent {
		t.Fatalf("marker-like prose changed: %s", card.Chat)
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
		    agent_stream_chat_cursor=9, agent_stream_console_cursor=8, step_context_cursor=7
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
