package omni

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestTwoModelsCommunicateThroughPostgresMemoryInterface(t *testing.T) {
	ctx := context.Background()
	runner := newFakeMemoryRunner()
	store := NewPGMemoryStore(runner)
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}

	alice, err := store.AddMemory(ctx, "model_a", "profile", "User name is Gryph.", []string{"user", "identity", "gryph"})
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.AddMemory(ctx, "model_b", "profile", "Gryph builds coding agents and manager-worker LLM systems.", []string{"user", "career", "coding-agents"})
	if err != nil {
		t.Fatal(err)
	}
	if alice.ID == bob.ID {
		t.Fatal("memory ids should be distinct")
	}

	tags, err := store.ListTags(ctx, 20)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"career", "coding-agents", "gryph", "identity", "user"} {
		if !containsString(tags, want) {
			t.Fatalf("tags missing %q: %v", want, tags)
		}
	}

	memories, err := store.SearchMemory(ctx, "Gryph", []string{"career", "identity"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) != 2 {
		t.Fatalf("memories = %d, want 2: %#v", len(memories), memories)
	}
	if !memoryRecordsContain(memories, "User name is Gryph") {
		t.Fatalf("missing identity memory: %#v", memories)
	}
	if !memoryRecordsContain(memories, "coding agents") {
		t.Fatalf("missing career memory: %#v", memories)
	}

	payload := "MEMORY_QUERY: Gryph career identity"
	client, closeServer := fakeOllamaClient(t, []string{
		mustRelayJSON(t, "model_a", "memory_manager", payload),
		mustRelayJSON(t, "memory_manager", "model_b", payload),
	})
	defer closeServer()
	relay := NewLLMRelayService(client).WithTimeout(5 * time.Second)
	result, err := relay.TelephoneGame(ctx, []string{"model_a", "memory_manager", "model_b"}, payload)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Delivered {
		t.Fatal("relay did not deliver memory query")
	}

	for _, wantSQL := range []string{
		"INSERT INTO memory_chunks",
		"INSERT INTO tags",
		"SELECT name",
		"FROM memory_chunks",
		"ft.name = ANY",
	} {
		if !runner.SawSQL(wantSQL) {
			t.Fatalf("runner did not execute SQL containing %q\nqueries:\n%s", wantSQL, strings.Join(runner.SQLLog, "\n---\n"))
		}
	}
}

func TestPGMemoryStoreSearchRequiresQueryOrTags(t *testing.T) {
	store := NewPGMemoryStore(newFakeMemoryRunner())
	_, err := store.SearchMemory(context.Background(), "", nil, 10)
	if err == nil {
		t.Fatal("expected empty search to fail")
	}
}

func TestPGMemoryStoreUpdatesAndDeprioritizesStaleMemory(t *testing.T) {
	ctx := context.Background()
	runner := newFakeMemoryRunner()
	store := NewPGMemoryStore(runner)
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}

	oldRecord, err := store.AddMemory(ctx, "memory_specialist", "project", "Old API endpoint is /v1/legacy.", []string{"api", "project"})
	if err != nil {
		t.Fatal(err)
	}
	newRecord, err := store.AddMemory(ctx, "research_specialist", "project", "Current API endpoint is /v2/current.", []string{"api", "project"})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.UpdateMemoryContent(ctx, oldRecord.ID, "correction_specialist", "Old API endpoint /v1/legacy is stale; use /v2/current.", []string{"stale"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.AgentID != "correction_specialist" || !strings.Contains(updated.Content, "stale") {
		t.Fatalf("updated memory = %#v", updated)
	}
	stale, err := store.DeprioritizeMemory(ctx, oldRecord.ID, newRecord.ID, "superseded by current API research")
	if err != nil {
		t.Fatal(err)
	}
	if stale.Priority >= newRecord.Priority {
		t.Fatalf("stale priority = %d, new priority = %d", stale.Priority, newRecord.Priority)
	}
	if stale.SupersededBy != newRecord.ID || stale.StalenessNote == "" {
		t.Fatalf("stale memory missing supersession metadata: %#v", stale)
	}
	memories, err := store.SearchMemory(ctx, "API endpoint", []string{"api"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) < 2 {
		t.Fatalf("memories = %#v, want both current and stale records", memories)
	}
	if memories[0].ID != newRecord.ID {
		t.Fatalf("current memory should rank ahead of stale memory: %#v", memories)
	}
	for _, wantSQL := range []string{"ALTER TABLE memory_chunks ADD COLUMN IF NOT EXISTS priority", "UPDATE memory_chunks SET content", "UPDATE memory_chunks SET priority"} {
		if !runner.SawSQL(wantSQL) {
			t.Fatalf("runner did not execute SQL containing %q\nqueries:\n%s", wantSQL, strings.Join(runner.SQLLog, "\n---\n"))
		}
	}
}

func TestLivePGMemoryStoreTwoModelMemory(t *testing.T) {
	if os.Getenv("OMNI_LIVE_PG_MEMORY") != "1" {
		t.Skip("set OMNI_LIVE_PG_MEMORY=1 to run live Postgres memory test")
	}
	databaseURL := strings.TrimSpace(os.Getenv("OMNI_TEST_DATABASE_URL"))
	if databaseURL == "" {
		databaseURL = "postgres://agent:agent@172.20.0.2:5432/agent?sslmode=disable"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		t.Skipf("live Postgres unavailable: %v", err)
	}

	store := NewPGMemoryStore(NewPgxMemoryRunner(pool))
	if err := store.EnsureSchema(ctx); err != nil {
		t.Fatal(err)
	}

	tag := fmt.Sprintf("omni-live-memory-%d", time.Now().UnixNano())
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `
			DELETE FROM memory_chunks mc
			USING memory_chunk_tags mct, tags t
			WHERE mc.id = mct.memory_chunk_id
			  AND mct.tag_id = t.id
			  AND t.name = $1
		`, tag)
		_, _ = pool.Exec(context.Background(), `DELETE FROM tags WHERE name = $1`, tag)
	})

	if _, err := store.AddMemory(ctx, "model_a", "profile", "Live memory: user name is Gryph.", []string{tag, "identity"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddMemory(ctx, "model_b", "profile", "Live memory: Gryph builds coding-agent systems.", []string{tag, "career"}); err != nil {
		t.Fatal(err)
	}
	memories, err := store.SearchMemory(ctx, "Gryph", []string{tag}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(memories) < 2 {
		t.Fatalf("live memories = %d, want >=2: %#v", len(memories), memories)
	}
	if !memoryRecordsContain(memories, "user name is Gryph") || !memoryRecordsContain(memories, "coding-agent systems") {
		t.Fatalf("live memory retrieval missed expected facts: %#v", memories)
	}
}

type fakeMemoryRunner struct {
	nextMemoryID int64
	nextTagID    int64
	memories     []MemoryRecord
	tagIDs       map[string]int64
	SQLLog       []string
}

func newFakeMemoryRunner() *fakeMemoryRunner {
	return &fakeMemoryRunner{
		nextMemoryID: 1,
		nextTagID:    1,
		tagIDs:       map[string]int64{},
	}
}

func (r *fakeMemoryRunner) Exec(ctx context.Context, sql string, args ...any) error {
	r.SQLLog = append(r.SQLLog, normalizeSQLForTest(sql))
	clean := strings.ToUpper(sql)
	if strings.Contains(clean, "INSERT INTO MEMORY_CHUNK_TAGS") && len(args) >= 2 {
		memoryID := int64FromAny(args[0])
		tagID := int64FromAny(args[1])
		tagName := ""
		for name, id := range r.tagIDs {
			if id == tagID {
				tagName = name
				break
			}
		}
		for i := range r.memories {
			if r.memories[i].ID == memoryID && tagName != "" && !containsString(r.memories[i].Tags, tagName) {
				r.memories[i].Tags = append(r.memories[i].Tags, tagName)
			}
		}
	}
	return nil
}

func (r *fakeMemoryRunner) Query(ctx context.Context, sql string, args ...any) ([]MemorySQLRow, error) {
	r.SQLLog = append(r.SQLLog, normalizeSQLForTest(sql))
	clean := strings.ToUpper(sql)

	switch {
	case strings.Contains(clean, "INSERT INTO MEMORY_CHUNKS"):
		record := MemoryRecord{
			ID:        r.nextMemoryID,
			AgentID:   stringFromAny(args[0]),
			Source:    stringFromAny(args[1]),
			Kind:      stringFromAny(args[2]),
			Content:   stringFromAny(args[3]),
			Priority:  100,
			CreatedAt: time.Now().UTC(),
		}
		r.nextMemoryID++
		r.memories = append(r.memories, record)
		return []MemorySQLRow{rowFromMemoryRecord(record)}, nil

	case strings.Contains(clean, "UPDATE MEMORY_CHUNKS") && strings.Contains(clean, "SET CONTENT"):
		id := int64FromAny(args[0])
		content := stringFromAny(args[1])
		agentID := stringFromAny(args[2])
		for i := range r.memories {
			if r.memories[i].ID == id {
				r.memories[i].Content = content
				r.memories[i].AgentID = agentID
				r.memories[i].Priority = 100
				r.memories[i].SupersededAt = time.Time{}
				r.memories[i].SupersededBy = 0
				r.memories[i].StalenessNote = ""
				return []MemorySQLRow{rowFromMemoryRecord(r.memories[i])}, nil
			}
		}
		return nil, nil

	case strings.Contains(clean, "UPDATE MEMORY_CHUNKS") && strings.Contains(clean, "SET PRIORITY"):
		id := int64FromAny(args[0])
		supersededBy := int64FromAny(args[1])
		note := stringFromAny(args[2])
		for i := range r.memories {
			if r.memories[i].ID == id {
				r.memories[i].Priority = 10
				r.memories[i].SupersededAt = time.Now().UTC()
				r.memories[i].SupersededBy = supersededBy
				r.memories[i].StalenessNote = note
				return []MemorySQLRow{rowFromMemoryRecord(r.memories[i])}, nil
			}
		}
		return nil, nil

	case strings.Contains(clean, "INSERT INTO TAGS"):
		tag := stringFromAny(args[0])
		id, ok := r.tagIDs[tag]
		if !ok {
			id = r.nextTagID
			r.nextTagID++
			r.tagIDs[tag] = id
		}
		return []MemorySQLRow{{"id": id}}, nil

	case strings.Contains(clean, "SELECT NAME") && strings.Contains(clean, "FROM TAGS"):
		tags := make([]string, 0, len(r.tagIDs))
		for tag := range r.tagIDs {
			tags = append(tags, tag)
		}
		tags = cleanMemoryTags(tags)
		limit := int(int64FromAny(args[0]))
		if limit > 0 && len(tags) > limit {
			tags = tags[:limit]
		}
		rows := make([]MemorySQLRow, 0, len(tags))
		for _, tag := range tags {
			rows = append(rows, MemorySQLRow{"name": tag})
		}
		return rows, nil

	case strings.Contains(clean, "FROM MEMORY_CHUNKS"):
		query := strings.ToLower(stringFromAny(args[0]))
		filterTags := stringSliceFromAny(args[1])
		limit := int(int64FromAny(args[2]))
		rows := []MemorySQLRow{}
		for _, memory := range r.memories {
			if query != "" && !strings.Contains(strings.ToLower(memory.Content), query) {
				continue
			}
			if len(filterTags) > 0 && !anyTagMatches(memory.Tags, filterTags) {
				continue
			}
			rows = append(rows, rowFromMemoryRecord(memory))
		}
		if limit > 0 && len(rows) > limit {
			rows = rows[:limit]
		}
		sortMemoryRowsByPriority(rows)
		return rows, nil
	}
	return nil, nil
}

func (r *fakeMemoryRunner) SawSQL(fragment string) bool {
	for _, sql := range r.SQLLog {
		if strings.Contains(sql, fragment) {
			return true
		}
	}
	return false
}

func normalizeSQLForTest(sql string) string {
	return strings.Join(strings.Fields(sql), " ")
}

func rowFromMemoryRecord(record MemoryRecord) MemorySQLRow {
	return MemorySQLRow{
		"id":             record.ID,
		"agent_id":       record.AgentID,
		"source":         record.Source,
		"kind":           record.Kind,
		"content":        record.Content,
		"priority":       record.Priority,
		"superseded_at":  record.SupersededAt,
		"superseded_by":  record.SupersededBy,
		"staleness_note": record.StalenessNote,
		"created_at":     record.CreatedAt,
		"tags":           append([]string(nil), record.Tags...),
	}
}

func sortMemoryRowsByPriority(rows []MemorySQLRow) {
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if int64FromAny(rows[j]["priority"]) > int64FromAny(rows[i]["priority"]) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
}

func memoryRecordsContain(records []MemoryRecord, needle string) bool {
	for _, record := range records {
		if strings.Contains(strings.ToLower(record.Content), strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func anyTagMatches(left, right []string) bool {
	for _, l := range left {
		for _, r := range right {
			if l == r {
				return true
			}
		}
	}
	return false
}
