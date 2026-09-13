package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestServerConstructionDefersUnusedHostDirectoryValidation(t *testing.T) {
	for _, configured := range []string{"", "relative", filepath.Join(t.TempDir(), "missing")} {
		server, err := NewServer(&queue.Repository{}, nil, ServerOptions{
			LifecycleContext: context.Background(), HostDirectoryAccessRoot: configured,
		})
		if err != nil {
			t.Fatalf("unused host directory %q blocked server construction: %v", configured, err)
		}
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodPatch, "/v1/jobs", nil))
		if response.Code != http.StatusMethodNotAllowed {
			t.Fatalf("routing without filesystem access: status=%d", response.Code)
		}
		if err := server.hostDirectoryAccess.ValidateWorkspaceRoot(t.TempDir()); err == nil {
			t.Fatalf("filesystem consumer accepted invalid host directory %q", configured)
		}
	}
}
