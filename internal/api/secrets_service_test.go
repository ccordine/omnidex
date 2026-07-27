package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
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

func TestSecretUpdateSourceRequiresRestartInsteadOfPartiallyMutatingRuntime(t *testing.T) {
	source, err := os.ReadFile("secrets_service.go")
	if err != nil {
		t.Fatalf("read secret service source: %v", err)
	}
	text := string(source)
	if strings.Contains(text, "applyStoredSecrets") || strings.Contains(text, "replaceProviderConfiguration") {
		t.Fatal("secret update still partially mutates the running provider configuration")
	}
	if !strings.Contains(text, `"restart_required": true`) {
		t.Fatal("secret update does not explicitly report its restart requirement")
	}
}
