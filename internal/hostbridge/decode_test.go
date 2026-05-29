package hostbridge

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDecodeResponseJSONIgnoresTrailingGarbage(t *testing.T) {
	payload, err := decodeResponseJSON([]byte(`{"path":"/tmp/New-project"}permission denied`))
	if err != nil {
		t.Fatalf("decodeResponseJSON() error=%v", err)
	}
	if got := stringField(payload, "path"); got != "/tmp/New-project" {
		t.Fatalf("path=%q", got)
	}
}

func TestClientMkdirIgnoresTrailingGarbage(t *testing.T) {
	agent := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/mkdir" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"path":"/tmp/New-project"}permission denied`)
	}))
	defer agent.Close()

	client := NewClient(agent.URL, "", 0)
	path, err := client.Mkdir(t.Context(), "/tmp", "New-project")
	if err != nil {
		t.Fatalf("Mkdir() error=%v", err)
	}
	if path != "/tmp/New-project" {
		t.Fatalf("path=%q", path)
	}
}
