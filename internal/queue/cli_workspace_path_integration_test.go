package queue_test

import (
	"context"
	"os"
	"testing"
)

func TestCLIWorkspaceBindingsPersistNativePathsAndSeparateClients(t *testing.T) {
	databaseURL := os.Getenv("OMNI_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("OMNI_TEST_DATABASE_URL is required")
	}
	pool, repository := freshLifecycleRepository(t, databaseURL)
	ctx := context.Background()
	for _, root := range []string{`C:\`, `C:\work\source`, `\\server\share`, `\\server\share\source`, "/work/source"} {
		first, err := repository.EnsureCLIChatSessionChannel(ctx, root,
			"client_11111111111111111111111111111111_directory_1_2")
		if err != nil {
			t.Fatalf("store native path %q: %v", root, err)
		}
		second, err := repository.EnsureCLIChatSessionChannel(ctx, root,
			"client_22222222222222222222222222222222_directory_1_2")
		if err != nil {
			t.Fatal(err)
		}
		if first.ID == second.ID {
			t.Fatalf("different installations share channel for %q", root)
		}
	}
	for _, root := range []string{`C:source`, `C:\work\..\source`, `\\server`, "/work/../source"} {
		var valid bool
		if err := pool.QueryRow(ctx, `SELECT channel_workspace_root_is_exact($1)`, root).Scan(&valid); err != nil {
			t.Fatal(err)
		}
		if valid {
			t.Fatalf("database accepted invalid workspace path %q", root)
		}
	}
}
