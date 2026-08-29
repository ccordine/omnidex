package hostbridge

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeResponseJSONRejectsTrailingGarbage(t *testing.T) {
	if _, err := decodeResponseJSON([]byte(`{"path":"/tmp/New-project"}permission denied`)); err == nil {
		t.Fatal("trailing host bridge bytes were accepted")
	}
}

func TestDecodeResponseJSONRejectsDuplicateKeys(t *testing.T) {
	if _, err := decodeResponseJSON([]byte(`{"path":"/first","path":"/second"}`)); err == nil {
		t.Fatal("duplicate host bridge response field was accepted")
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

func TestClientMkdirRejectsTrailingGarbage(t *testing.T) {
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
	if _, err := client.Mkdir(t.Context(), "/tmp", "New-project"); err == nil {
		t.Fatal("Mkdir accepted trailing host bridge bytes")
	}
}

func TestReadResponseBodyRejectsOversizedPayload(t *testing.T) {
	if _, err := readResponseBody(strings.NewReader(strings.Repeat("x", maxResponseBodyBytes+1))); err == nil {
		t.Fatal("oversized host bridge response was accepted")
	}
}
