package hostbridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListDirectoryHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("home unavailable")
	}
	result, err := ListDirectory(home, BrowseOptions{})
	if err != nil {
		t.Fatalf("ListDirectory() error=%v", err)
	}
	if result.Path != filepath.Clean(home) {
		t.Fatalf("path=%q want %q", result.Path, home)
	}
}

func TestNonEmptyEntries(t *testing.T) {
	if got := NonEmptyEntries(nil); got == nil || len(got) != 0 {
		t.Fatalf("NonEmptyEntries(nil)=%#v want empty slice", got)
	}
	items := []Entry{{Name: "a", Path: "/a", IsDir: true}}
	if got := NonEmptyEntries(items); len(got) != 1 {
		t.Fatalf("NonEmptyEntries(items)=%#v", got)
	}
}

func TestListDirectoryRejectsOutsideRoots(t *testing.T) {
	_, err := ListDirectory("/etc", BrowseOptions{})
	if err == nil {
		t.Fatal("expected browse outside roots to fail")
	}
}
