package api

import (
	"strings"
	"testing"
)

func TestProjectMapAutosyncUsesDurableJobProjectAuthority(t *testing.T) {
	source := readAPISource(t, "project_map_autosync.go")
	if !strings.Contains(source, "s.repo.JobProjectID(ctx, jobID)") {
		t.Fatal("project map autosync must use jobs.project_id")
	}
	for _, forbidden := range []string{
		"context.Background()",
		"resolveJobProjectRef",
		"metadataProjectID",
		"jobWorkspaceLocation",
		"GetProjectByLocation",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("project map autosync contains detached or metadata fallback %q", forbidden)
		}
	}
}
