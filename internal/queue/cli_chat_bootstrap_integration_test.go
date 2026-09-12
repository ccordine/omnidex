package queue_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/projectroot"
	"github.com/gryph/omnidex/internal/queue"
)

func TestCLIChatBootstrapStoresExactWorkspaceBindingAndReusesOneChannel(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for isolated PostgreSQL CLI bootstrap coverage")
	}
	var longPath strings.Builder
	for index := range 70 {
		fmt.Fprintf(&longPath, "/segment-%04d-%040d", index, index)
	}
	for _, root := range []string{"/tmp/plain workspace/文本", longPath.String()} {
		t.Run(fmt.Sprintf("root-bytes-%d", len(root)), func(t *testing.T) {
			pool, repository := freshLifecycleRepository(t, databaseURL)
			ctx := context.Background()
			identity := "directory_1_101"
			type result struct {
				channel model.Channel
				err     error
			}
			results := make(chan result, 8)
			for range cap(results) {
				go func() {
					channel, err := repository.EnsureCLIChatSessionChannel(ctx, root, identity)
					results <- result{channel, err}
				}()
			}
			var retained model.Channel
			for index := range cap(results) {
				out := <-results
				if out.err != nil {
					t.Fatalf("bootstrap %d: %v", index, out.err)
				}
				if index == 0 {
					retained = out.channel
				} else if out.channel.ID != retained.ID ||
					!out.channel.CreatedAt.Equal(retained.CreatedAt) ||
					!out.channel.UpdatedAt.Equal(retained.UpdatedAt) {
					t.Fatalf("repeated bootstrap changed retained channel: %#v / %#v", retained, out.channel)
				}
			}
			if !projectroot.IsCLIChatChannelID(retained.ID) || retained.WorkspaceRoot != root {
				t.Fatalf("invalid server-issued channel: %#v", retained)
			}
			var count int
			if err := pool.QueryRow(ctx, `
				SELECT count(*) FROM ai_channels WHERE workspace_root=$1 AND cli_workspace_identity=$2
			`, root, identity).Scan(&count); err != nil || count != 1 {
				t.Fatalf("stored exact binding count = %d, error = %v", count, err)
			}
			moved, err := repository.EnsureCLIChatSessionChannel(ctx, root+"/moved", identity)
			if err != nil {
				t.Fatal(err)
			}
			replaced, err := repository.EnsureCLIChatSessionChannel(ctx, root, "directory_1_102")
			if err != nil {
				t.Fatal(err)
			}
			if moved.ID == retained.ID || replaced.ID == retained.ID || moved.ID == replaced.ID {
				t.Fatal("changed workspace values reused a different binding")
			}
			if _, err := repository.CreateChannel(ctx, retained); err == nil ||
				!strings.Contains(err.Error(), "server bootstrap boundary") {
				t.Fatalf("generic creation accepted a reserved CLI identity: %v", err)
			}
		})
	}
}

func TestCLIChatBootstrapRejectsContradictoryDuplicateBinding(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required for isolated PostgreSQL CLI bootstrap coverage")
	}
	pool, repository := freshLifecycleRepository(t, databaseURL)
	ctx := context.Background()
	root, identity := "/tmp/duplicate-cli-binding", "directory_1_101"
	retained, err := repository.EnsureCLIChatSessionChannel(ctx, root, identity)
	if err != nil {
		t.Fatal(err)
	}
	duplicateID, err := projectroot.NewCLIChatChannelID()
	if err != nil {
		t.Fatal(err)
	}
	// Deliberately bypass the sole bootstrap writer to prove corrupt database
	// state is rejected instead of selecting one of two conflicting rows.
	if _, err := pool.Exec(ctx, `
		INSERT INTO ai_channels (id,name,tags,scope,workspace_root,cli_workspace_identity)
		SELECT $1,$2,tags,scope,workspace_root,cli_workspace_identity FROM ai_channels WHERE id=$3
	`, duplicateID, "CLI chat "+string(duplicateID), retained.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := repository.EnsureCLIChatSessionChannel(ctx, root, identity); !errors.Is(err, queue.ErrCLIChatSessionConflict) {
		t.Fatalf("duplicate workspace binding error = %v", err)
	}
}
