package queue_test

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/sessionlist"
)

func TestCLISessionsBoundedPagesReachEverySession(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	pool, repo := freshLifecycleRepository(t, databaseURL)
	owner := strings.Repeat("1", 32)
	const total = 205
	for i := 1; i <= total; i++ {
		_, err := pool.Exec(t.Context(), `INSERT INTO ai_channels(id,name,scope,workspace_root,cli_workspace_identity) VALUES($1,'Session','user',$2,$3)`, fmt.Sprintf("cli-chat-%032x", i), fmt.Sprintf("/fixture/%d", i), fmt.Sprintf("client_%s_directory_1_%d", owner, i))
		if err != nil {
			t.Fatal(err)
		}
	}
	var cursor model.ChannelID
	count := 0
	for pageNumber := 0; pageNumber < 3; pageNumber++ {
		page, err := repo.CLISessions(t.Context(), owner, cursor)
		if err != nil {
			t.Fatal(err)
		}
		expected := min(sessionlist.PageSize, total-count)
		if len(page.Sessions) != expected {
			t.Fatalf("page size %d want %d", len(page.Sessions), expected)
		}
		for _, entry := range page.Sessions {
			count++
			if entry.ID != model.ChannelID(fmt.Sprintf("cli-chat-%032x", count)) {
				t.Fatal("unreachable or repeated session", entry.ID)
			}
		}
		cursor = page.NextCursor
		if (cursor == "") != (count == total) {
			t.Fatal("incorrect continuation", cursor)
		}
	}
}
