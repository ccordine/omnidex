package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProjectsRequireDatabase(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/projects", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want %d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

func TestBrowseDefaultsToHome(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/browse", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestRecipesList(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/recipes", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestSplitProjectPath(t *testing.T) {
	id, action := splitProjectPath("/v1/projects/42/activate")
	if id != 42 || action != "activate" {
		t.Fatalf("id=%d action=%q", id, action)
	}
	id, action = splitProjectPath("/v1/projects/7/map/scan")
	if id != 7 || action != "map/scan" {
		t.Fatalf("id=%d action=%q", id, action)
	}
}
