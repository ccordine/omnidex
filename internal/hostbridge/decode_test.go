package hostbridge

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestDecodeResponseBodyReportsPlainTextHTTPError(t *testing.T) {
	_, err := decodeResponseBody([]byte("404 page not found\n"), http.StatusNotFound)
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), "host bridge HTTP 404: 404 page not found"; got != want {
		t.Fatalf("error=%q want %q", got, want)
	}
}

func TestDecodeResponseBodyStillReportsInvalidSuccessJSON(t *testing.T) {
	_, err := decodeResponseBody([]byte("404 page not found\n"), http.StatusOK)
	if err == nil {
		t.Fatal("expected error")
	}
	if got, want := err.Error(), "invalid host bridge JSON"; !strings.Contains(got, want) {
		t.Fatalf("error=%q should contain %q", got, want)
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
