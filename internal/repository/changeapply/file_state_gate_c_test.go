package changeapply_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeletionRejectsTargetContentDriftImmediatelyBeforeApply(t *testing.T) {
	t.Parallel()
	fixture := newFixture(t, map[string]fixtureEntry{
		"go.mod":      {content: "module example.com/delete-drift\n\ngo 1.24\n", mode: 0o644},
		"retained.go": {content: "package deletedrift\n\nfunc Retained() int { return 1 }\n", mode: 0o644},
		"obsolete.go": {content: "package deletedrift\n\nfunc Obsolete() int { return 2 }\n", mode: 0o644},
	})
	stage, err := planFixtureDeletion(t, fixture, "obsolete.go")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stage.Cleanup() })

	drifted := "package deletedrift\n\nfunc Obsolete() int { return 7 }\n"
	if err := os.WriteFile(filepath.Join(fixture.root, "obsolete.go"), []byte(drifted), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := stage.ApplyVerified(context.Background()); err == nil ||
		(!strings.Contains(err.Error(), "changed") && !strings.Contains(err.Error(), "stale")) {
		t.Fatalf("deletion content drift error=%v", err)
	}
	assertFile(t, filepath.Join(fixture.root, "obsolete.go"), drifted, 0o644)
	assertFile(
		t, filepath.Join(fixture.root, "retained.go"),
		"package deletedrift\n\nfunc Retained() int { return 1 }\n", 0o644,
	)
}
