package hostbridge

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListDirectoryRejectsNestedSymlinkEscapeFromRequiredRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "outside-link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err := ListDirectory(link, BrowseOptions{
		Limit: 1, RequiredRoot: root, ExtraRoots: []string{root},
	}); err == nil {
		t.Fatal("nested browse symlink escaped the required project root")
	}
}
