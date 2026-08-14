package api

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeScrumCardActionProjectIDIsEndpointExact(t *testing.T) {
	t.Parallel()
	request := httptest.NewRequest("POST", "/play?project_id=14", nil)
	if projectID, err := decodeScrumCardActionProjectID(request, "Scrum card play"); err != nil || projectID != 14 {
		t.Fatalf("projectID=%d error=%v", projectID, err)
	}
	for _, target := range []string{
		"/play", "/play?project_id=014", "/play?project_id=14&project_id=15",
		"/play?project_id=14&before=scrumchat_v1_a",
		"/play?project_id=14&tool=" + strings.Repeat("x", maxScrumCardActionQueryBytes),
	} {
		if _, err := decodeScrumCardActionProjectID(
			httptest.NewRequest("POST", target, nil), "Scrum card play",
		); err == nil {
			t.Fatalf("target %q unexpectedly accepted", target)
		}
	}
}
