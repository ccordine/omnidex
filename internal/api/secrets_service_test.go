package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gryph/omnidex/internal/secrets"
)

func TestHandleAPISecretsGetMasksValues(t *testing.T) {
	store := &secrets.MemoryStore{Values: map[string]string{"openai_api_key": "sk-test-9876"}}
	server := &Server{secretsResolver: secrets.NewResolver(store)}

	req := httptest.NewRequest(http.MethodGet, "/v1/settings/secrets", nil)
	rec := httptest.NewRecorder()
	server.handleAPISecretsGet(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var payload struct {
		Fields []map[string]any `json:"fields"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var openai map[string]any
	for _, field := range payload.Fields {
		if field["key"] == "openai_api_key" {
			openai = field
			break
		}
	}
	if openai == nil {
		t.Fatal("openai field missing")
	}
	if openai["hint"] != "••••9876" {
		t.Fatalf("expected masked hint, got %#v", openai["hint"])
	}
}

func TestApplyStoredSecretsUsesDatabase(t *testing.T) {
	store := &secrets.MemoryStore{Values: map[string]string{"openai_api_key": "sk-db-key"}}
	server := &Server{secretsResolver: secrets.NewResolver(store)}
	server.applyStoredSecrets(context.Background())
	if server.openAIAPIKey != "sk-db-key" {
		t.Fatalf("expected server key from database, got %q", server.openAIAPIKey)
	}
}
