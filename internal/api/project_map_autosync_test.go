package api

import (
	"encoding/json"
	"testing"
)

func TestJobWorkspaceLocationPrefersProjectDirectory(t *testing.T) {
	meta := json.RawMessage(`{"project_directory":"/repo/app","client_cwd":"/tmp/other"}`)
	got := jobWorkspaceLocation(meta)
	if got != "/repo/app" {
		t.Fatalf("location=%q want /repo/app", got)
	}
}

func TestMetadataProjectID(t *testing.T) {
	meta := json.RawMessage(`{"project_id":42}`)
	if got := metadataProjectID(meta); got != 42 {
		t.Fatalf("project_id=%d want 42", got)
	}
}
