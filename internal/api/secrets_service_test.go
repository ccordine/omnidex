package api

import (
	"bytes"
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

func TestHandleAPISecretsGetFailsLoudlyOnRetiredStoredCredential(t *testing.T) {
	store := &secrets.MemoryStore{Values: map[string]string{"cursor_api_key": "retired-secret"}}
	server := &Server{secretsResolver: secrets.NewResolver(store)}
	request := httptest.NewRequest(http.MethodGet, "/v1/settings/secrets", nil)
	response := httptest.NewRecorder()
	server.handleAPISecretsGet(response, request)
	if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "unsupported or retired") {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAPISecretsRequestIsExactBoundedAndRejectsRetiredAgentCredentials(t *testing.T) {
	validKey := ""
	for _, field := range secrets.Fields {
		if field.Key != "" {
			validKey = field.Key
			break
		}
	}
	if validKey == "" {
		t.Fatal("production provider secret catalog is empty")
	}
	validBody := []byte(`{"values":{"` + validKey + `":"secret"},"clear_keys":[]}`)
	request := httptest.NewRequest(http.MethodPut, "/v1/settings/secrets", bytes.NewReader(validBody))
	response := httptest.NewRecorder()
	decoded, err := decodeAPISecretsRequest(response, request)
	if err != nil || decoded.Values[validKey] != "secret" {
		t.Fatalf("decoded=%+v err=%v", decoded, err)
	}

	for _, body := range [][]byte{
		[]byte(`{"values":{},"values":{},"clear_keys":[]}`),
		[]byte(`{"values":{},"clear_keys":[],"agent":"codex"}`),
		[]byte(`{"Values":{},"clear_keys":[]}`),
		[]byte(`{"values":{},"clear_keys":[]} {}`),
		[]byte(`{"values":null,"clear_keys":[]}`),
		[]byte(`{"values":{},"clear_keys":null}`),
		[]byte(`{"values":{"cursor_api_key":"retired"},"clear_keys":[]}`),
		[]byte(`{"values":{"codex_api_key":"retired"},"clear_keys":[]}`),
		[]byte(`{"values":{"` + validKey + `":" one "},"clear_keys":[]}`),
		[]byte(`{"values":{"` + validKey + `":"one"},"clear_keys":["` + validKey + `"]}`),
		[]byte(`{"values":{},"clear_keys":["` + validKey + `","` + validKey + `"]}`),
		bytes.Repeat([]byte(" "), int(maxAPISecretsBodyBytes+1)),
	} {
		request := httptest.NewRequest(http.MethodPut, "/v1/settings/secrets", bytes.NewReader(body))
		response := httptest.NewRecorder()
		if _, err := decodeAPISecretsRequest(response, request); err == nil {
			t.Fatalf("invalid API secrets authority accepted: %q", body)
		}
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
