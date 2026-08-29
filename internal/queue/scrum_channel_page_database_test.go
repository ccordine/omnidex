package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestScrumChannelPageProjectsOnlyOneBoundedJSONWindow(t *testing.T) {
	repository, _, ctx := scrumChannelPageTestRepository(t)
	var projectID int64
	if err := repository.pool.QueryRow(ctx, `
		INSERT INTO projects(location,name) VALUES('/tmp/scrum-channel-page','Channel page') RETURNING id
	`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO scrum_cards(id,project_id,title,column_name,play_state)
		VALUES('channel-card',$1,'Channel','assigned','')
	`, projectID); err != nil {
		t.Fatal(err)
	}
	appends := make([]ScrumCardMessageAppend, 205)
	for index := range appends {
		appends[index] = ScrumCardMessageAppend{
			ID: fmt.Sprintf("message_%03d", index), Role: "assistant",
			Content: fmt.Sprintf("content %03d", index),
		}
	}
	appendScrumMessagesForTest(t, repository, projectID, "channel-card", appends)

	latest, err := repository.ScrumChannelPage(ctx, projectID, "channel-card", 50, -1)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Total != 205 || latest.Start != 155 || !latest.HasMore || latest.PlayState != "" {
		t.Fatalf("latest page authority=%+v", latest)
	}
	if len(latest.Messages) != 50 || latest.Messages[0].ID != "message_155" || latest.Messages[49].ID != "message_204" {
		t.Fatalf("latest messages=%v..%v count=%d", latest.Messages[0], latest.Messages[len(latest.Messages)-1], len(latest.Messages))
	}

	earlier, err := repository.ScrumChannelPage(ctx, projectID, "channel-card", 50, latest.Start)
	if err != nil {
		t.Fatal(err)
	}
	if earlier.Start != 105 || !earlier.HasMore || earlier.Total != 205 {
		t.Fatalf("earlier page authority=%+v", earlier)
	}
	if _, err := repository.ScrumChannelPage(ctx, projectID, "channel-card", 50, 206); err == nil {
		t.Fatal("stale channel cursor was accepted")
	}
	if _, err := repository.ScrumChannelPage(ctx, projectID, "channel-card", 50, 0); err == nil {
		t.Fatal("zero channel cursor was accepted instead of omitted or positive")
	}
	if _, err := repository.ScrumChannelPage(ctx, projectID, " channel-card", 50, -1); err == nil {
		t.Fatal("noncanonical card identity was accepted")
	}

	card, err := repository.GetScrumCard(ctx, projectID, "channel-card")
	if err != nil {
		t.Fatal(err)
	}
	if card.ChannelMessageCount != 205 {
		t.Fatalf("lean card counter=%d", card.ChannelMessageCount)
	}
}

func TestScrumChannelPageEnforcesExactContentByteBudgetAndStableCursor(t *testing.T) {
	repository, _, ctx := scrumChannelPageTestRepository(t)
	var projectID int64
	if err := repository.pool.QueryRow(ctx, `
		INSERT INTO projects(location,name) VALUES('/tmp/scrum-channel-byte-page','Byte page') RETURNING id
	`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.pool.Exec(ctx, `
		INSERT INTO scrum_cards(id,project_id,title,column_name,play_state)
		VALUES('byte-page-card',$1,'Byte page','assigned','')
	`, projectID); err != nil {
		t.Fatal(err)
	}
	appends := make([]ScrumCardMessageAppend, 6)
	for index := range appends {
		appends[index] = ScrumCardMessageAppend{
			ID: fmt.Sprintf("byte-message-%d", index+1), Role: "assistant",
			Content: strings.Repeat(string(rune('a'+index)), MaxScrumChannelPageBytes/4),
		}
	}
	appendScrumMessagesForTest(t, repository, projectID, "byte-page-card", appends)
	latest, err := repository.ScrumChannelPage(ctx, projectID, "byte-page-card", 100, -1)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Total != 6 || latest.Start != 2 || len(latest.Messages) != 4 || !latest.HasMore {
		t.Fatalf("latest byte page=%+v", latest)
	}
	bytes := 0
	for _, message := range latest.Messages {
		bytes += len(message.Content)
	}
	if bytes != MaxScrumChannelPageBytes {
		t.Fatalf("latest content bytes=%d want=%d", bytes, MaxScrumChannelPageBytes)
	}
	earlier, err := repository.ScrumChannelPage(ctx, projectID, "byte-page-card", 100, latest.Start)
	if err != nil {
		t.Fatal(err)
	}
	if earlier.Start != 0 || earlier.HasMore || len(earlier.Messages) != 2 ||
		earlier.Messages[0].ID != "byte-message-1" || earlier.Messages[1].ID != "byte-message-2" {
		t.Fatalf("earlier byte page=%+v", earlier)
	}
}

func TestScrumChannelEncodedMessageCeilingCoversWorstCaseJSONEscaping(t *testing.T) {
	t.Parallel()
	content := strings.Repeat("\x01", MaxScrumChannelPageBytes)
	encoded, err := json.Marshal([]ScrumCardMessage{{
		Ordinal: 1, ID: strings.Repeat("m", 256), Role: "assistant", Content: content,
		CreatedAt:       time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC),
		SourceCreatedAt: "2026-08-13T12:00:00Z", TimestampOrigin: "runtime",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(encoded) <= MaxScrumChannelPageBytes*5 || len(encoded) > MaxScrumChannelEncodedMessagesBytes {
		t.Fatalf("escaped message bytes=%d ceiling=%d", len(encoded), MaxScrumChannelEncodedMessagesBytes)
	}
}

func TestScrumChannelPageUsesAnIndexBoundedTailAtLargeHistory(t *testing.T) {
	repository, pool, ctx := scrumChannelPageTestRepository(t)
	var projectID int64
	if err := pool.QueryRow(ctx, `
		INSERT INTO projects(location,name) VALUES('/tmp/scrum-channel-large-page','Large page') RETURNING id
	`).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO scrum_cards(id,project_id,title,column_name,play_state)
		VALUES('large-page-card',$1,'Large page','assigned','')
	`, projectID); err != nil {
		t.Fatal(err)
	}
	const historySize = 5000
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for value := 1; value <= historySize; value++ {
		if _, err := tx.Exec(ctx, `
			INSERT INTO scrum_card_messages(project_id,card_id,message_id,role,content,status)
			VALUES($1,'large-page-card',$2,'assistant',$3,'')
		`, projectID, fmt.Sprintf("large_%06d", value), fmt.Sprintf("bounded content %d", value)); err != nil {
			t.Fatalf("append large-history message %d: %v", value, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ANALYZE scrum_card_messages`); err != nil {
		t.Fatal(err)
	}
	var counterPlan []byte
	if err := pool.QueryRow(ctx, `
		EXPLAIN (ANALYZE, FORMAT JSON)
		SELECT content_bytes FROM scrum_card_messages
		WHERE project_id=$1 AND card_id='large-page-card' AND ordinal=$2
	`, projectID, historySize).Scan(&counterPlan); err != nil {
		t.Fatal(err)
	}
	assertScrumMessageTailPlanIsBounded(t, counterPlan, 1)

	var rawPlan []byte
	if err := pool.QueryRow(ctx,
		"EXPLAIN (ANALYZE, FORMAT JSON) "+scrumCardMessageTailQuery,
		projectID, "large-page-card", historySize, 50, MaxScrumChannelPageBytes,
	).Scan(&rawPlan); err != nil {
		t.Fatal(err)
	}
	assertScrumMessageTailPlanIsBounded(t, rawPlan, 50)

	page, err := repository.ScrumChannelPage(ctx, projectID, "large-page-card", 50, -1)
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != historySize || page.Start != historySize-50 || len(page.Messages) != 50 ||
		page.Messages[0].ID != "large_004951" || page.Messages[49].ID != "large_005000" {
		t.Fatalf("large-history page=%+v", page)
	}
}

func assertScrumMessageTailPlanIsBounded(t *testing.T, raw []byte, limit float64) {
	t.Helper()
	var plan any
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatalf("decode Scrum message tail plan: %v", err)
	}
	seenBoundedIndex := false
	var walk func(any)
	walk = func(value any) {
		switch typed := value.(type) {
		case []any:
			for _, child := range typed {
				walk(child)
			}
		case map[string]any:
			if typed["Relation Name"] == "scrum_card_messages" {
				node, _ := typed["Node Type"].(string)
				if strings.Contains(node, "Seq Scan") {
					t.Fatalf("unbounded Scrum message relation plan node=%v", typed)
				}
				if strings.Contains(node, "Index") {
					rows, ok := typed["Actual Rows"].(float64)
					if !ok || rows > limit {
						t.Fatalf("Scrum message index read rows=%v, limit=%v", typed["Actual Rows"], limit)
					}
					seenBoundedIndex = true
				}
			}
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(plan)
	if !seenBoundedIndex {
		t.Fatalf("Scrum message tail plan lacks one bounded index read: %s", raw)
	}
}

func scrumChannelPageTestRepository(t *testing.T) (*Repository, *pgxpool.Pool, context.Context) {
	t.Helper()
	pool := openIsolatedDatabasePool(t)
	repository := New(pool)
	if err := repository.ResetDatabase(t.Context(), loadCurrentDatabaseSetup(t)); err != nil {
		t.Fatal(err)
	}
	return repository, pool, t.Context()
}
