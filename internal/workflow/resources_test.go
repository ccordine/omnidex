package workflow

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixtureScope(t *testing.T) Scope {
	t.Helper()
	scope, err := Open(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	scope.Job = 17
	scope.Task = "current"
	return scope
}

func TestReuseAndTemporaryCleanup(t *testing.T) {
	scope := fixtureScope(t)
	first, err := scope.Environment([]string{"PATH=" + os.Getenv("PATH"), "TMPDIR=/tmp", "GOTMPDIR=/compiler-temp", "PRESERVED=value"}, "/existing/omnidex")
	if err != nil {
		t.Fatal(err)
	}
	second, err := scope.Environment([]string{"PATH=" + os.Getenv("PATH"), "TMPDIR=/tmp", "GOTMPDIR=/compiler-temp", "PRESERVED=value"}, "/existing/omnidex")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(first, "\n") != strings.Join(second, "\n") {
		t.Fatal("retry allocated another environment")
	}
	var temporary string
	for _, value := range first {
		if strings.HasPrefix(value, ScopeVariable+"=") {
			temporary = filepath.Dir(strings.TrimPrefix(value, ScopeVariable+"="))
		}
	}
	if temporary == "" {
		t.Fatal("missing owned temporary directory")
	}
	if !strings.Contains(strings.Join(first, "\n"), "TMPDIR=/tmp\n") {
		t.Fatal("application temporary directory was changed")
	}
	if !strings.Contains(strings.Join(first, "\n"), "GOTMPDIR=/compiler-temp\n") {
		t.Fatal("Go temporary directory was changed")
	}
	if err := os.WriteFile(filepath.Join(temporary, "compiler-output"), []byte("temporary"), 0600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(scope.Workspace, "source.go")
	if err := os.WriteFile(source, []byte("preserve"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := scope.CleanTemporary(scope.Task); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(temporary); !os.IsNotExist(err) {
		t.Fatal("owned temporary output remains")
	}
	if data, err := os.ReadFile(source); err != nil || string(data) != "preserve" {
		t.Fatal("source was changed")
	}
	if err := scope.CleanTemporary(scope.Task); err != nil {
		t.Fatal("recovery cleanup is not idempotent", err)
	}
}

func TestEnvironmentPreservesTemporaryDefaults(t *testing.T) {
	scope := fixtureScope(t)
	environment, err := scope.Environment([]string{"PATH=" + os.Getenv("PATH")}, "/existing/omnidex")
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range environment {
		if strings.HasPrefix(value, "TMPDIR=") || strings.HasPrefix(value, "GOTMPDIR=") {
			t.Fatal("host temporary defaults were replaced")
		}
	}
}

func TestCleanupRetainsCurrentRollbackAndUnfinishedWork(t *testing.T) {
	scope := fixtureScope(t)
	var names []string
	for i := 0; i < 5; i++ {
		name := filepath.Join(scope.Workspace, fmt.Sprint(i))
		names = append(names, name)
		if err := os.WriteFile(name, []byte(fmt.Sprint(i)), 0600); err != nil {
			t.Fatal(err)
		}
		scope.Task = fmt.Sprint(i)
		if err := scope.TrackFile(name, "compiler", false, ""); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Millisecond)
	}
	if err := scope.Complete([]string{"0", "1", "2", "3"}); err != nil {
		t.Fatal(err)
	}
	if err := scope.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
	for i, name := range names {
		_, err := os.Stat(name)
		if i < 2 && !os.IsNotExist(err) {
			t.Fatalf("obsolete output %d retained", i)
		}
		if i >= 2 && err != nil {
			t.Fatalf("current/rollback/unfinished output %d removed", i)
		}
	}
	if err := scope.Cleanup(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestCleanupPreservesExistingModifiedOutsideAndOtherJobFiles(t *testing.T) {
	scope := fixtureScope(t)
	for _, name := range []string{"existing", "modified", "other-job", "unfinished", "old", "recent", "current"} {
		file := filepath.Join(scope.Workspace, name)
		if err := os.WriteFile(file, []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
		scope.Task = name
		if name == "other-job" {
			scope.Job = 99
		} else {
			scope.Job = 17
		}
		if err := scope.TrackFile(file, "same-family", name == "existing", ""); err != nil {
			t.Fatal(err)
		}
		if name == "modified" {
			if err := os.WriteFile(file, []byte("owner changed this output"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		time.Sleep(time.Millisecond)
	}
	scope.Job = 17
	outside := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(outside, []byte("unique"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := scope.TrackFile(outside, "same-family", false, ""); err != nil {
		t.Fatal(err)
	}
	if err := scope.Complete([]string{"existing", "modified", "other-job", "old", "recent", "current"}); err != nil {
		t.Fatal(err)
	}
	if err := scope.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"existing", "modified", "other-job", "unfinished", "recent", "current"} {
		if _, err := os.Stat(filepath.Join(scope.Workspace, name)); err != nil {
			t.Fatalf("preserved file %s removed", name)
		}
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("outside resource changed")
	}
	if _, err := os.Stat(filepath.Join(scope.Workspace, "old")); !os.IsNotExist(err) {
		t.Fatal("owned obsolete output remains")
	}
}

func TestCleanupDoesNotFollowChangedParentSymlink(t *testing.T) {
	scope := fixtureScope(t)
	parent := filepath.Join(scope.Workspace, "output")
	if err := os.Mkdir(parent, 0700); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(parent, "binary")
	if err := os.WriteFile(file, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(file)
	record := Resource{Path: file, Size: info.Size(), Modified: info.ModTime()}
	retained := filepath.Join(scope.Workspace, "retained")
	if err := os.Rename(parent, retained); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(retained, parent); err != nil {
		t.Fatal(err)
	}
	if err := scope.RemoveFile(record); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(retained, "binary")); err != nil {
		t.Fatal("followed changed symlink")
	}
}
