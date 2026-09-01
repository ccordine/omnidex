package workspace

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestAuthoritativeDigestRejectsCheckedFileSwappedToSymlinkOrFIFO(t *testing.T) {
	for _, fixture := range []struct {
		name    string
		replace func(string) error
	}{
		{
			name: "symlink",
			replace: func(target string) error {
				outside := filepath.Join(filepath.Dir(target), "outside.txt")
				if err := os.WriteFile(outside, []byte("outside authority"), 0o600); err != nil {
					return fmt.Errorf("write outside fixture: %w", err)
				}
				if err := os.Symlink(outside, target); err != nil {
					return fmt.Errorf("replace checked file with symlink: %w", err)
				}
				return nil
			},
		},
		{
			name: "fifo",
			replace: func(target string) error {
				if err := unix.Mkfifo(target, 0o600); err != nil {
					return fmt.Errorf("replace checked file with FIFO: %w", err)
				}
				return nil
			},
		},
	} {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			rootPath := t.TempDir()
			target := filepath.Join(rootPath, "checked.txt")
			if err := os.WriteFile(target, []byte("checked"), 0o600); err != nil {
				t.Fatalf("write checked fixture: %v", err)
			}
			fence, err := AcquireMutationFence(context.Background(), rootPath)
			if err != nil {
				t.Fatalf("acquire digest fence: %v", err)
			}
			defer func() {
				if err := fence.Release(); err != nil {
					t.Errorf("release digest fence: %v", err)
				}
			}()
			fence.mu.Lock()
			root, err := fence.authoritativeRootLocked()
			if err != nil {
				fence.mu.Unlock()
				t.Fatalf("attest digest root: %v", err)
			}
			expected, err := root.Lstat("checked.txt")
			if err != nil {
				fence.mu.Unlock()
				t.Fatalf("lstat checked fixture: %v", err)
			}
			if err := os.Remove(target); err != nil {
				fence.mu.Unlock()
				t.Fatalf("remove checked fixture: %v", err)
			}
			if err := fixture.replace(target); err != nil {
				fence.mu.Unlock()
				t.Fatal(err)
			}
			digester := authoritativeWorkspaceDigester{
				root: root, digest: sha256.New(),
				maximum: WorkspaceDigestOptions{MaxPaths: 8, MaxBytes: 1024},
			}
			started := time.Now()
			err = digester.digestRegularFile("checked.txt", expected)
			fence.mu.Unlock()
			if err == nil {
				t.Fatal("swapped checked file unexpectedly entered authoritative digest")
			}
			if elapsed := time.Since(started); elapsed > time.Second {
				t.Fatalf("swapped checked file failure was not bounded: %s", elapsed)
			}
		})
	}
}

func TestMutationFenceSameRootExclusionAndCanceledAcquisitionAreBounded(t *testing.T) {
	root := t.TempDir()
	first, err := AcquireMutationFence(context.Background(), root)
	if err != nil {
		t.Fatalf("acquire first fence: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Millisecond)
	defer cancel()
	started := time.Now()
	if second, err := AcquireMutationFence(ctx, root); err == nil || second != nil {
		t.Fatalf("same-root second fence unexpectedly acquired: fence=%v error=%v", second, err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("canceled same-root acquisition was not bounded: %s", elapsed)
	}
	if err := first.Release(); err != nil {
		t.Fatalf("release first fence: %v", err)
	}
	third, err := AcquireMutationFence(context.Background(), root)
	if err != nil {
		t.Fatalf("reacquire released root: %v", err)
	}
	if err := third.Release(); err != nil {
		t.Fatalf("release reacquired fence: %v", err)
	}
}

func TestFenceCommandDirectoryCannotBeRedirectedByRootPathSwap(t *testing.T) {
	if os.Getenv("OMNIDEX_FENCE_CWD_HELPER") == "1" {
		content, err := os.ReadFile("marker.txt")
		if err != nil {
			t.Fatal(err)
		}
		fmt.Print(string(content))
		return
	}
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "marker.txt"), []byte("anchored"), 0o600); err != nil {
		t.Fatalf("write anchored marker: %v", err)
	}
	fence, err := AcquireMutationFence(context.Background(), root)
	if err != nil {
		t.Fatalf("acquire command fence: %v", err)
	}
	released := false
	moved := root + "-moved"
	t.Cleanup(func() {
		if !released {
			_ = fence.Release()
		}
		_ = os.RemoveAll(moved)
	})
	cwd, err := fence.CommandWorkingDirectory(root)
	if err != nil {
		t.Fatalf("resolve fd-rooted command cwd: %v", err)
	}
	if err := os.Rename(root, moved); err != nil {
		t.Fatalf("move literal root: %v", err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("create replacement literal root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "marker.txt"), []byte("replacement"), 0o600); err != nil {
		t.Fatalf("write replacement marker: %v", err)
	}
	command := exec.Command(os.Args[0], "-test.run=^TestFenceCommandDirectoryCannotBeRedirectedByRootPathSwap$")
	command.Dir = cwd
	command.Env = append(os.Environ(), "OMNIDEX_FENCE_CWD_HELPER=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("run fd-rooted helper: %v\n%s", err, output)
	}
	if !strings.Contains(string(output), "anchored") || strings.Contains(string(output), "replacement") {
		t.Fatalf("fd-rooted helper used replacement cwd: %q", output)
	}
	if err := fence.Reattest(root); err == nil {
		t.Fatal("swapped literal root unexpectedly retained fence authority")
	}
	if err := fence.Release(); err != nil {
		t.Fatalf("release command fence: %v", err)
	}
	released = true
	if err := os.RemoveAll(moved); err != nil {
		t.Fatalf("remove moved command root: %v", err)
	}
}

func TestFencedReconciliationRejectsLiteralRootReplacement(t *testing.T) {
	for _, phase := range []string{"prepare", "apply"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			fence, err := AcquireMutationFence(context.Background(), root)
			if err != nil {
				t.Fatalf("acquire reconciliation fence: %v", err)
			}
			released := false
			moved := root + "-moved"
			t.Cleanup(func() {
				if !released {
					_ = fence.Release()
				}
				_ = os.RemoveAll(moved)
			})
			desired := []DesiredFile{{
				Path: "created.txt", Present: true, Content: []byte("fenced"), Mode: 0o600,
			}}
			var prepared *PreparedReconciliation
			if phase == "apply" {
				prepared = &PreparedReconciliation{
					fence: fence, root: root, rootInfo: fence.rootInfo, desired: desired,
				}
			}
			if err := os.Rename(root, moved); err != nil {
				t.Fatalf("move fenced root: %v", err)
			}
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatalf("create replacement root: %v", err)
			}
			if phase == "prepare" {
				prepared, err = fence.PrepareReconciliation(
					context.Background(), HostDirectoryAccess{}, root, desired,
				)
			} else {
				_, err = prepared.ApplyVerified(context.Background(), nil)
			}
			if err == nil {
				t.Fatalf("literal root replacement unexpectedly survived %s", phase)
			}
			if _, statErr := os.Lstat(filepath.Join(root, "created.txt")); !os.IsNotExist(statErr) {
				t.Fatalf("replacement root was mutated: %v", statErr)
			}
			if _, statErr := os.Lstat(filepath.Join(moved, "created.txt")); !os.IsNotExist(statErr) {
				t.Fatalf("moved fenced root was mutated: %v", statErr)
			}
			if err := fence.Release(); err != nil {
				t.Fatalf("release reconciliation fence: %v", err)
			}
			released = true
			if err := os.RemoveAll(moved); err != nil {
				t.Fatalf("remove moved reconciliation root: %v", err)
			}
		})
	}
}
