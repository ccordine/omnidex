package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestResolveProjectIDRejectsAliasesAndDuplicateAuthority(t *testing.T) {
	t.Parallel()
	server := &Server{repo: &queue.Repository{}}
	accepted := httptest.NewRequest(http.MethodGet, "/?project_id=14&tab=files", nil)
	if id, err := server.resolveProjectID(accepted); err != nil || id != 14 {
		t.Fatalf("id=%d error=%v", id, err)
	}
	for _, test := range []struct {
		target, message string
	}{
		{target: "/", message: "required"},
		{target: "/?project_id=014", message: "canonical"},
		{target: "/?project_id=%2B14", message: "canonical"},
		{target: "/?project_id=%2014", message: "canonical"},
		{target: "/?project_id=14&project_id=15", message: "exactly once"},
		{target: "/?project_id=14&tab=%zz", message: "decode project query"},
	} {
		request := httptest.NewRequest(http.MethodGet, test.target, nil)
		_, err := server.resolveProjectID(request)
		if err == nil || !strings.Contains(err.Error(), test.message) {
			t.Fatalf("target=%q error=%v want containing %q", test.target, err, test.message)
		}
	}
}
