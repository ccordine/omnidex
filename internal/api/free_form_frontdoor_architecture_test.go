package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestFreeFormProductionFrontDoorHasNoDirectModelOrMemoryFallback(t *testing.T) {
	t.Parallel()
	for file, forbidden := range map[string][]string{
		"server.go": {
			`/v1/instruct`, `/v1/roleplay`, `/v1/narrate`, `/v1/reasoning`,
			`/v1/research/ingest`, `newInMemoryChannelStore`, `instructIntegration`,
		},
		"channels.go": {
			`runPersona(`, `resolvePersonaLLM(`, `channelMessagesToPersonaHistory`,
			`persistChannelMemory`, `newInMemoryChannelStore`,
		},
		"web/src/controllers/chat_controller.ts": {
			`submitDirect`, `/v1/instruct`,
		},
	} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(source), token) {
				t.Errorf("%s retains forbidden free-form fallback %q", file, token)
			}
		}
	}
}

func TestRemovedDirectPersonaRoutesReturnNotFound(t *testing.T) {
	t.Parallel()
	server := NewServer(nil, nil)
	for _, path := range []string{
		"/v1/instruct", "/v1/roleplay", "/v1/narrate", "/v1/reasoning", "/v1/research/ingest",
	} {
		request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"prompt":"ignored"}`))
		response := httptest.NewRecorder()
		server.Handler().ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Errorf("%s status=%d body=%s", path, response.Code, response.Body.String())
		}
	}
}
