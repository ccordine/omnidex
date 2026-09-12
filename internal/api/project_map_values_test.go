package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProjectMapReportsCurrentFilesWithoutHashedWorkspaceIdentity(t *testing.T) {
	for _, fixture := range []struct{ path, source string }{
		{"main.go", "package sample\nconst Value = 3\n"},
		{"index.ts", "export const label = 'observation';\n"},
	} {
		t.Run(fixture.path, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, fixture.path), []byte(fixture.source), 0o600); err != nil {
				t.Fatal(err)
			}
			payload, err := loadProjectCodebaseMapPayloadLocal(root)
			if err != nil {
				t.Fatal(err)
			}
			if _, exists := payload["workspace_id"]; exists {
				t.Fatal("project map retained a redundant hashed workspace identity")
			}
			if payload["root"] != root || payload["file_count"] != 1 || payload["scan_truncated"] != false {
				t.Fatalf("map lost the inspected workspace: %#v", payload)
			}
			files := payload["files_preview"].([]map[string]any)
			languages := payload["languages"].([]map[string]any)
			if len(files) != 1 || files[0]["path"] != fixture.path || len(languages) != 1 ||
				languages[0]["files"] != 1 || languages[0]["bytes"] != int64(len(fixture.source)) {
				t.Fatalf("map differs from actual file content and size: %#v", payload)
			}
		})
	}
}
