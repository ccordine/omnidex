package queue

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionHasNoRoleplayOnlyPortableResultReuseAPI(t *testing.T) {
	t.Parallel()
	forbidden := []string{
		"RoleplayPortableResultReuseReceiptSchemaV1",
		"RoleplayPortableResultReuseRequest",
		"RoleplayPortableResultReuseReceipt",
		"RoleplayPortableResultReuse",
		"ReuseRoleplayPortableResult",
		"reuseRoleplayResult",
		"roleplay_portable_result_reuses",
		"omnidex.roleplay-portable-result-reuse.v1",
	}
	root := filepath.Clean(filepath.Join("..", ".."))
	files := make([]string, 0)
	if err := filepath.WalkDir(filepath.Join(root, "internal"), func(
		path string,
		entry fs.DirEntry,
		err error,
	) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		files = append(files, path)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	files = append(files, filepath.Join(root, "database", "setup.sql"))
	for _, path := range files {
		source, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for _, retired := range forbidden {
			if strings.Contains(string(source), retired) {
				t.Fatalf("production source %s retains roleplay-only reuse API %q", path, retired)
			}
		}
	}
}
