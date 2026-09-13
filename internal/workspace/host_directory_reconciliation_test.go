package workspace

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestReconciliationUsesOrdinaryHostDirectoryAndAcceptsZeroDelta(t *testing.T) {
	for _, fixture := range []struct {
		path    string
		content []byte
		mode    uint32
	}{
		{"nested/settings.json", []byte("{\"enabled\":true}\r\n"), 0o640},
		{"bin/payload", []byte{0, 1, 255, 10}, 0o750},
	} {
		t.Run(fixture.path, func(t *testing.T) {
			root := t.TempDir()
			ctx := context.Background()
			fence, err := AcquireMutationFence(ctx, root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := fence.Release(); err != nil {
					t.Error(err)
				}
			})
			desired := []DesiredFile{{Path: fixture.path, Present: true, Content: fixture.content, Mode: fixture.mode}}
			for iteration := 0; iteration < 2; iteration++ {
				prepared, err := fence.Prepare(ctx, desired, nil)
				if err != nil {
					t.Fatal(err)
				}
				result, err := prepared.ApplyVerified(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				if iteration == 0 && len(result.Changes) != 1 || iteration == 1 && len(result.Changes) != 0 {
					t.Fatalf("iteration=%d changes=%#v", iteration, result.Changes)
				}
				actual, err := os.ReadFile(filepath.Join(root, fixture.path))
				if err != nil || !bytes.Equal(actual, fixture.content) {
					t.Fatalf("actual=%v err=%v", actual, err)
				}
				info, err := os.Stat(filepath.Join(root, fixture.path))
				if err != nil {
					t.Fatal(err)
				}
				if info.Mode().Perm() != os.FileMode(fixture.mode) {
					t.Fatalf("mode=%v", info.Mode())
				}
			}
		})
	}
}

func TestReconciliationRejectsRootReplacementAfterPreparation(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	fence, err := AcquireMutationFence(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := fence.Release(); err != nil {
			t.Error(err)
		}
	})
	prepared, err := fence.Prepare(ctx,
		[]DesiredFile{{Path: "result.txt", Present: true, Content: []byte("value"), Mode: 0o600}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	retained := filepath.Join(parent, "retained")
	if err := os.Rename(root, retained); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o750); err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.ApplyVerified(ctx, nil); err == nil {
		t.Fatal("reconciliation accepted a replaced root")
	}
	for _, path := range []string{root, retained} {
		if _, err := os.Lstat(filepath.Join(path, "result.txt")); !os.IsNotExist(err) {
			t.Fatalf("rejected reconciliation wrote to %q: %v", path, err)
		}
	}
}
