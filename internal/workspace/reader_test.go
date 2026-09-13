package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceReaderReturnsExactBytesAndBoundedOrderedPages(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{"z.txt": "last", "a.bin": "\x00first\xff", "middle.txt": "middle"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o640); err != nil {
			t.Fatal(err)
		}
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	ctx := context.Background()
	file, err := ReadFile(ctx, opened, "a.bin")
	if err != nil || string(file.Content) != "\x00first\xff" || file.Entry.Mode != 0o640 {
		t.Fatalf("read exact binary file = %#v: %v", file, err)
	}
	page, err := ReadDirectory(ctx, opened, ".", "", 2)
	if err != nil || len(page.Entries) != 2 || !page.HasMore || page.Entries[0].Path != "a.bin" || page.Entries[1].Path != "middle.txt" {
		t.Fatalf("first directory page = %#v: %v", page, err)
	}
	page, err = ReadDirectory(ctx, opened, ".", "middle.txt", 2)
	if err != nil || len(page.Entries) != 1 || page.HasMore || page.Entries[0].Path != "z.txt" {
		t.Fatalf("last directory page = %#v: %v", page, err)
	}
	if _, err := ReadFile(ctx, opened, "missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing file error = %v", err)
	}
}

func TestWorkspaceReaderRejectsEscapesLinksAndOversizedContents(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "outside.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "linked")); err != nil {
		t.Fatal(err)
	}
	large, err := os.Create(filepath.Join(root, "large.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if err := large.Truncate(MaxReconciliationFileBytes + 1); err != nil {
		t.Fatal(err)
	}
	if err := large.Close(); err != nil {
		t.Fatal(err)
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	for _, name := range []string{"../outside.txt", "linked/outside.txt", "large.bin"} {
		if _, err := ReadFile(context.Background(), opened, name); err == nil {
			t.Errorf("accepted invalid read %q", name)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadDirectory(ctx, opened, ".", "", 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled listing error = %v", err)
	}
}

func TestWorkspaceReaderDeterminesEmptyContentWithoutReadPermission(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "empty"), nil, 0); err != nil {
		t.Fatal(err)
	}
	opened, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	file, err := ReadFile(context.Background(), opened, "empty")
	if err != nil || len(file.Content) != 0 || file.Entry.Mode != 0 {
		t.Fatalf("empty file: %#v %v", file, err)
	}
}
