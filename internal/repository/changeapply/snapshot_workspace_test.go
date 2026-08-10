package changeapply_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/repository/changeapply"
)

func TestSnapshotWorkspaceContainsOnlyExactSnapshotFiles(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t, map[string]fixtureEntry{
		".gitignore": {content: ".env\n.env.local\n*.pem\n*.key\n", mode: 0o600},
		"go.mod":     {content: "module example.com/snapshotworkspace\n\ngo 1.24\n", mode: 0o600},
		"value.go":   {content: "package snapshotworkspace\n\nfunc Value() int { return 1 }\n", mode: 0o600},
	})
	snapshot := fixture.snapshot
	for _, item := range []struct{ name, content string }{
		{name: ".env", content: "CANARY_ENV"},
		{name: ".env.local", content: "CANARY_LOCAL"},
		{name: "ignored.pem", content: "CANARY_PEM"},
		{name: "ignored.key", content: "CANARY_KEY"},
	} {
		if err := os.WriteFile(filepath.Join(snapshot.Root, item.name), []byte(item.content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	workspace, err := changeapply.NewSnapshotWorkspace(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = workspace.Cleanup() })
	for _, file := range snapshot.Files {
		if _, err := os.Lstat(filepath.Join(workspace.Root(), filepath.FromSlash(file.Path))); err != nil {
			t.Fatalf("snapshot file %q absent from projection: %v", file.Path, err)
		}
	}
	for _, forbidden := range []string{
		".git", ".omni", ".env", ".env.local", "ignored.pem", "ignored.key",
	} {
		if _, err := os.Lstat(filepath.Join(workspace.Root(), forbidden)); !os.IsNotExist(err) {
			t.Fatalf("non-snapshot authority %q entered projection: %v", forbidden, err)
		}
	}
	if err := workspace.VerifyExact(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotWorkspaceFailsWhenExactSourceIsAbsent(t *testing.T) {
	t.Parallel()
	fixture := basicFixture(t)
	snapshot := fixture.snapshot
	file := snapshot.Files[0]
	if err := os.Remove(filepath.Join(snapshot.Root, filepath.FromSlash(file.Path))); err != nil {
		t.Fatal(err)
	}
	if workspace, err := changeapply.NewSnapshotWorkspace(context.Background(), snapshot); err == nil || workspace != nil {
		t.Fatalf("absent exact source produced workspace=%+v error=%v", workspace, err)
	}
}
