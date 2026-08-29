package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestRetiredBrowserInferenceEndpointsAreAbsent(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	for _, path := range []string{
		"/v1/browser-inference/context-relevance",
		"/v1/browser-inference/context-relevance/ws",
	} {
		request := httptest.NewRequest(http.MethodGet, path, nil)
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Errorf("retired endpoint %s returned status %d", path, response.Code)
		}
	}
}

func TestRetiredBrowserInferenceClientFilesAreAbsent(t *testing.T) {
	for _, path := range []string{
		"web/src/controllers/browser_inference_controller.ts",
		"web/src/lib/browser_context_relevance_protocol.ts",
		"web/src/lib/browser_context_relevance_runtime.ts",
		"web/src/workers/browser_inference_worker.ts",
	} {
		if _, err := os.Stat(filepath.FromSlash(path)); !os.IsNotExist(err) {
			t.Errorf("retired browser inference client %s still exists: %v", path, err)
		}
	}
}
