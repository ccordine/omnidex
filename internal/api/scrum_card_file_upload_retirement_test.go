package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScrumCardFileUploadIsRetiredBeforeWorkspaceMutation(t *testing.T) {
	workspace := t.TempDir()
	server := &Server{}
	request := httptest.NewRequest(
		http.MethodPost,
		"/v1/scrum/cards/card-1/files?project_id=1&project_directory="+workspace,
		strings.NewReader("untrusted upload bytes"),
	)
	response := httptest.NewRecorder()

	server.handleScrumCardFiles(response, request, "card-1")

	if response.Code != http.StatusGone {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	entries, err := os.ReadDir(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("retired upload mutated workspace: %#v", entries)
	}
}

func TestScrumCardFileUploadClientAndFilesystemWriterAreAbsent(t *testing.T) {
	root := filepath.Join("web", "src")
	for _, target := range []struct {
		path      string
		forbidden []string
	}{
		{path: "scrum_card_file_upload_handler.go", forbidden: []string{"ParseMultipartForm", "os.OpenFile", "io.Copy", "MkdirAll"}},
		{path: filepath.Join(root, "lib", "scrum_api.ts"), forbidden: []string{"uploadScrumCardFiles", "body.append(\"files\""}},
		{path: filepath.Join(root, "react", "card-modal", "FilesTab.tsx"), forbidden: []string{"Upload Files", "uploadScrumCardFiles", "setUploadFiles"}},
	} {
		source, err := os.ReadFile(target.path)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range target.forbidden {
			if strings.Contains(string(source), forbidden) {
				t.Errorf("%s retains retired upload authority %q", target.path, forbidden)
			}
		}
	}
}
