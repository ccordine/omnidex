package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeBrowseMkdirRequestAcceptsOnlyExactBoundedDTO(t *testing.T) {
	recorder := httptest.NewRecorder()
	request, err := decodeBrowseMkdirRequest(recorder, httptest.NewRequest(
		http.MethodPost, "/v1/browse/mkdir", strings.NewReader(`{"parent":"/srv/projects","name":"new-project"}`),
	))
	if err != nil || request.Parent != "/srv/projects" || request.Name != "new-project" {
		t.Fatalf("request=%+v error=%v", request, err)
	}
	for name, raw := range map[string]string{
		"unknown":        `{"parent":"/srv/projects","name":"new-project","agent":"forbidden"}`,
		"duplicate":      `{"parent":"/srv/projects","name":"first","name":"second"}`,
		"trailing":       `{"parent":"/srv/projects","name":"new-project"}{}`,
		"trimmed parent": `{"parent":" /srv/projects","name":"new-project"}`,
		"trimmed name":   `{"parent":"/srv/projects","name":" new-project "}`,
		"path name":      `{"parent":"/srv/projects","name":"nested/project"}`,
		"NUL":            `{"parent":"/srv/projects","name":"bad\u0000name"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeBrowseMkdirRequest(httptest.NewRecorder(), httptest.NewRequest(
				http.MethodPost, "/v1/browse/mkdir", strings.NewReader(raw),
			)); err == nil {
				t.Fatal("invalid directory creation request was accepted")
			}
		})
	}
}

func TestDecodeBrowseMkdirRequestRejectsOversizedTransport(t *testing.T) {
	raw := `{"parent":"/srv/projects","name":"` + strings.Repeat("x", int(maxBrowseMkdirBodyBytes)) + `"}`
	recorder := httptest.NewRecorder()
	_, err := decodeBrowseMkdirRequest(recorder, httptest.NewRequest(http.MethodPost, "/v1/browse/mkdir", strings.NewReader(raw)))
	if err == nil || browseMkdirRequestStatus(err) != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d error=%v", browseMkdirRequestStatus(err), err)
	}
}
