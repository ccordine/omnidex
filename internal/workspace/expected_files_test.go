package workspace

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReconciliationPreservesChangedExpectedFileAndStopsBeforeNewWrites(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "accepted.txt"), []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	fence, err := AcquireMutationFence(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	defer fence.Release()
	expected := []File{{Entry: Entry{Path: "accepted.txt", Kind: EntryFile, Mode: 0o644, Size: 6}, Content: []byte("before")}}
	prepared, err := fence.Prepare(ctx,
		[]DesiredFile{{Path: "new.txt", Present: true, Content: []byte("new"), Mode: 0o644, CreateOnly: true}}, expected)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "accepted.txt"), []byte("edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := prepared.ApplyVerified(ctx, nil)
	if err == nil || len(result.Changes) != 0 {
		t.Fatalf("changed dependency allowed publication: %#v: %v", result, err)
	}
	if _, err := os.Stat(filepath.Join(root, "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("new file was written: %v", err)
	}
	if content, err := os.ReadFile(filepath.Join(root, "accepted.txt")); err != nil || string(content) != "edited" {
		t.Fatalf("external edit was not retained: %q: %v", content, err)
	}
}
