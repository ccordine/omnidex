package hostbridge

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestListDirectoryHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("home unavailable")
	}
	result, err := ListDirectory(home, BrowseOptions{Limit: DefaultBrowsePageSize})
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
	_, err := ListDirectory("/etc", BrowseOptions{Limit: DefaultBrowsePageSize})
	if err == nil {
		t.Fatal("expected browse outside roots to fail")
	}
}

func TestListDirectoryUsesBoundedPages(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOST_BROWSE_ROOTS", root)
	for index := 0; index < 7; index++ {
		if err := os.Mkdir(filepath.Join(root, "directory-"+strconv.Itoa(index)), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	first, err := ListDirectory(root, BrowseOptions{Limit: 3, DirectoriesOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	second, err := ListDirectory(root, BrowseOptions{Limit: 3, Offset: 3, DirectoriesOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	last, err := ListDirectory(root, BrowseOptions{Limit: 3, Offset: 6, DirectoriesOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Entries) != 3 || first.HasPrevious || !first.HasMore || first.Offset != 0 || first.NextOffset != 3 {
		t.Fatalf("first page=%+v", first)
	}
	if len(second.Entries) != 3 || !second.HasPrevious || second.PreviousOffset != 0 || !second.HasMore || second.Offset != 3 || second.NextOffset != 6 {
		t.Fatalf("second page=%+v", second)
	}
	if len(last.Entries) != 1 || !last.HasPrevious || last.PreviousOffset != 3 || last.HasMore || last.Offset != 6 || last.NextOffset != 0 {
		t.Fatalf("last page=%+v", last)
	}
	seen := map[string]bool{}
	for _, page := range []*BrowseResult{first, second, last} {
		for _, entry := range page.Entries {
			if seen[entry.Name] {
				t.Fatalf("duplicate entry %q across pages", entry.Name)
			}
			seen[entry.Name] = true
		}
	}
	if len(seen) != 7 {
		t.Fatalf("seen=%d want 7", len(seen))
	}
}

func TestListDirectoryPaginationSkipsHiddenAndNonDirectoriesBeforeOffset(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOST_BROWSE_ROOTS", root)
	for _, name := range []string{".hidden", "a", "b", "c"} {
		if err := os.Mkdir(filepath.Join(root, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "plain-file"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	page, err := ListDirectory(root, BrowseOptions{Limit: 2, Offset: 2, DirectoriesOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Entries) != 1 || !page.Entries[0].IsDir || page.Entries[0].Name == ".hidden" {
		t.Fatalf("page=%+v", page)
	}
}

func TestListDirectoryRejectsInvalidPageBounds(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOST_BROWSE_ROOTS", root)
	for _, opts := range []BrowseOptions{
		{},
		{Limit: MaxBrowsePageSize + 1},
		{Limit: 1, Offset: -1},
	} {
		if _, err := ListDirectory(root, opts); err == nil {
			t.Fatalf("expected bounds %+v to fail", opts)
		}
	}
}
