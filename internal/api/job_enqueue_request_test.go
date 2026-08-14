package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

func TestDecodeGenericCodingEnqueuePreservesExactInstruction(t *testing.T) {
	exact := "  preserve exact coding authority\nwith trailing tab\t "
	request := httptest.NewRequest(http.MethodPost, "/v1/jobs", strings.NewReader(
		`{"instruction":"  preserve exact coding authority\nwith trailing tab\t ","pipeline":"coding","metadata":{"client_cwd":"/srv/work","host_env_cwd":"/srv/work","session_id":"session-1"}}`,
	))
	response := httptest.NewRecorder()

	decoded, err := decodeGenericCodingEnqueue(response, request)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Instruction != exact || decoded.Pipeline != "coding" || decoded.Metadata == nil ||
		decoded.Metadata.ClientCWD != "/srv/work" || decoded.Metadata.SessionID != "session-1" {
		t.Fatalf("decoded=%+v", decoded)
	}
}

func TestGenericCodingRuntimeMetadataAddsOnlyCodeOwnedModelSnapshot(t *testing.T) {
	envPath := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(envPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OMNI_ENV_FILE", envPath)
	t.Setenv("OMNI_CONVERSATION_RESPONSE_MODEL", "runtime-owned-model")
	raw, err := (&Server{}).genericCodingRuntimeMetadata(genericCodingMetadata{
		ClientCWD: "/srv/work", HostEnvCWD: "/srv/work", SessionID: "session-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded genericCodingRuntimeMetadata
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.ClientCWD != "/srv/work" || decoded.HostEnvCWD != "/srv/work" || decoded.SessionID != "session-1" {
		t.Fatalf("runtime metadata changed typed transport authority: %+v", decoded)
	}
	if decoded.ModelConfig.Get("conversation_response_model") != "runtime-owned-model" {
		t.Fatalf("runtime model snapshot is not code-owned: %+v", decoded.ModelConfig)
	}
}

func TestDecodeGenericCodingEnqueueRejectsLooseOrUnboundedAuthority(t *testing.T) {
	invalidUTF8 := append([]byte(`{"instruction":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, []byte(`","pipeline":"coding","metadata":{}}`)...)
	tests := []struct {
		name string
		body []byte
	}{
		{name: "duplicate top-level", body: []byte(`{"instruction":"one","instruction":"two","pipeline":"coding","metadata":{}}`)},
		{name: "duplicate metadata", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"key":1,"key":2}}`)},
		{name: "unknown", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{},"mode":"agent"}`)},
		{name: "inexact case", body: []byte(`{"Instruction":"one","pipeline":"coding","metadata":{}}`)},
		{name: "trailing", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{}} {}`)},
		{name: "missing metadata", body: []byte(`{"instruction":"one","pipeline":"coding"}`)},
		{name: "null metadata", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":null}`)},
		{name: "array metadata", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":[]}`)},
		{name: "unknown metadata", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"client_cwd":"/srv/work","host_env_cwd":"/srv/work","key":"value"}}`)},
		{name: "client model config", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"client_cwd":"/srv/work","host_env_cwd":"/srv/work","model_config":{"conversation_response_model":"client"}}}`)},
		{name: "missing workspace", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"client_cwd":"","host_env_cwd":""}}`)},
		{name: "relative workspace", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"client_cwd":"work","host_env_cwd":"work"}}`)},
		{name: "unclean workspace", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"client_cwd":"/srv/../work","host_env_cwd":"/srv/../work"}}`)},
		{name: "mismatched workspace", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"client_cwd":"/srv/one","host_env_cwd":"/srv/two"}}`)},
		{name: "padded session", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"client_cwd":"/srv/work","host_env_cwd":"/srv/work","session_id":" session "}}`)},
		{name: "agent selector", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"agent_config":{"agent_system":"codex"}}}`)},
		{name: "instance agent selector", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"instance_agent_config":{"agent_system":"cursor"}}}`)},
		{name: "agent telemetry claim", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"external_agents_used":["codex"]}}`)},
		{name: "legacy execution agent", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"execution_agent":"codex"}}`)},
		{name: "legacy agent strict", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"agent_strict":true}}`)},
		{name: "recipe ID", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"recipe_id":"frontend"}}`)},
		{name: "retired raw Scrum route", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"scrum_raw_play":true}}`)},
		{name: "retired no-delegate route", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"omnidex_no_delegate":true}}`)},
		{name: "recipe payload", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"recipe":{}}}`)},
		{name: "Scrum pipeline", body: []byte(`{"instruction":"one","pipeline":"scrum","metadata":{"source":"omni-scrum"}}`)},
		{name: "chat pipeline", body: []byte(`{"instruction":"one","pipeline":"chat","metadata":{}}`)},
		{name: "padded coding", body: []byte(`{"instruction":"one","pipeline":" coding","metadata":{}}`)},
		{name: "blank instruction", body: []byte(`{"instruction":" \n\t ","pipeline":"coding","metadata":{}}`)},
		{name: "invalid UTF-8", body: invalidUTF8},
		{name: "oversized", body: []byte(`{"instruction":"one","pipeline":"coding","metadata":{"client_cwd":"/srv/work","host_env_cwd":"/srv/work","session_id":"` + strings.Repeat("x", int(maxGenericCodingEnqueueBodyBytes)) + `"}}`)},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/v1/jobs", bytes.NewReader(testCase.body))
			response := httptest.NewRecorder()
			if _, err := decodeGenericCodingEnqueue(response, request); err == nil {
				t.Fatalf("invalid generic enqueue accepted: %q", testCase.body)
			}
		})
	}
}

func TestPublicGenericJobSourceCannotSelectScrum(t *testing.T) {
	source := readAPISource(t, "job_collection.go")
	for _, forbidden := range []string{
		"model.PipelineScrum",
		"expected coding or scrum",
		"json.NewDecoder(r.Body)",
		"enrichJobMetadata",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("public generic job route retains broad authority %q", forbidden)
		}
	}
}

func TestGenericJobCollectionQueryIsExactAndBounded(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/v1/jobs?status=running&limit=20&offset=40", nil)
	query, err := decodeJobCollectionQuery(request)
	if err != nil {
		t.Fatal(err)
	}
	if query.Status != "running" || query.Limit != 20 || query.Offset != 40 {
		t.Fatalf("query=%+v", query)
	}
	for _, raw := range []string{
		"status=RUNNING", "status=running%20", "status=running&status=pending", "status=unknown",
		"limit=0", "limit=101", "limit=01", "offset=-1", "offset=1000001", "unknown=1",
	} {
		if _, err := decodeJobCollectionQuery(httptest.NewRequest(http.MethodGet, "/v1/jobs?"+raw, nil)); err == nil {
			t.Fatalf("inexact job query was accepted: %q", raw)
		}
	}
}

func TestGenericJobEnqueueRejectsQueryAuthorityBeforeRepositoryAccess(t *testing.T) {
	server := NewServer(nil, nil)
	server.repo = &queue.Repository{}
	server.mux = http.NewServeMux()
	server.routes()
	request := httptest.NewRequest(http.MethodPost, "/v1/jobs?pipeline=scrum", strings.NewReader(
		`{"instruction":"exact","pipeline":"coding","metadata":{"client_cwd":"/srv/work","host_env_cwd":"/srv/work"}}`,
	))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
