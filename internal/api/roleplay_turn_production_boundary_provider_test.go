package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const (
	roleplayBoundaryNarrativeModel  = "qwen3.5:9b"
	roleplayBoundaryNarrativeRunner = "qwen3.5:9b-q4_K_M"
	roleplayBoundarySemanticModel   = "qwen3.5:9b-semantic"
	roleplayBoundarySemanticRunner  = "qwen3.5:9b-semantic-q4_K_M"
	roleplayBoundaryReply           = "Mara ducks beneath the blade, blood streaking her sleeve, and drives the attacker back through the shattered archive door."
	roleplayBoundaryAction          = "Mara is holding the shattered doorway against the remaining attackers."
	roleplayBoundaryFact            = "Mara survived the first assault at the shattered archive doorway."
)

type roleplayBoundaryOllama struct {
	mu              sync.Mutex
	generatedModels []string
	stationSchemas  []string
	unexpected      []string
	terminalCanon   chan struct{}
	release         chan struct{}
	terminalOnce    sync.Once
	releaseOnce     sync.Once
}

func newRoleplayBoundaryOllama() *roleplayBoundaryOllama {
	return &roleplayBoundaryOllama{
		terminalCanon: make(chan struct{}),
		release:       make(chan struct{}),
	}
}

func (provider *roleplayBoundaryOllama) serveHTTP(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch request.URL.Path {
	case "/api/version":
		writeRoleplayBoundaryJSON(w, map[string]any{"version": "0.24.0"})
	case "/api/tags":
		writeRoleplayBoundaryJSON(w, map[string]any{"models": []any{
			roleplayBoundaryModel(roleplayBoundaryNarrativeModel, false),
			roleplayBoundaryModel(roleplayBoundarySemanticModel, false),
		}})
	case "/api/show":
		writeRoleplayBoundaryJSON(w, roleplayBoundaryShowResponse())
	case "/api/ps":
		writeRoleplayBoundaryJSON(w, map[string]any{"models": []any{
			roleplayBoundaryModel(roleplayBoundaryNarrativeRunner, true),
			roleplayBoundaryModel(roleplayBoundarySemanticRunner, true),
		}})
	case "/api/generate":
		provider.generate(w, request)
	default:
		provider.recordUnexpected(request.URL.Path)
		http.Error(w, `{"error":"unexpected endpoint"}`, http.StatusNotFound)
	}
}

func (provider *roleplayBoundaryOllama) generate(w http.ResponseWriter, request *http.Request) {
	var payload map[string]any
	if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
		provider.recordUnexpected("decode generate: " + err.Error())
		http.Error(w, `{"error":"invalid request"}`, http.StatusBadRequest)
		return
	}
	modelName, _ := payload["model"].(string)
	if _, generated := payload["prompt"]; !generated {
		writeRoleplayBoundaryJSON(w, map[string]any{"done": true})
		return
	}
	schema := roleplayBoundarySchema(payload)
	candidate := ""
	switch schema {
	case assemblyline.ContextRelevanceSchemaV1:
		candidate = `{"schema":"omnidex.context-relevance.v1","referenced_candidate_ids":["CTX_3","CTX_4","CTX_5","CTX_6"]}`
	case assemblyline.ContextMinificationSchemaV1:
		candidate = `{"schema":"omnidex.context-minification.v1","minimal_context":"Mara is defending the archive."}`
	case assemblyline.ConversationResponseSchemaV1:
		raw, _ := json.Marshal(assemblyline.ConversationResponseDecision{
			Schema: assemblyline.ConversationResponseSchemaV1, Text: roleplayBoundaryReply,
		})
		candidate = string(raw)
	case assemblyline.RoleplayCanonExtractionSchemaV1:
		candidate = `{"schema":"omnidex.roleplay-canon-extraction.v1","facts":["` + roleplayBoundaryFact + `"]}`
	case assemblyline.RoleplayOngoingStateLeafV1:
		candidate = `{"schema":"omnidex.roleplay-ongoing-action.v1","ongoing_action":"` + roleplayBoundaryAction + `"}`
	default:
		provider.recordUnexpected("station schema " + schema)
		http.Error(w, `{"error":"unexpected station"}`, http.StatusBadRequest)
		return
	}
	provider.mu.Lock()
	provider.generatedModels = append(provider.generatedModels, modelName)
	provider.stationSchemas = append(provider.stationSchemas, schema)
	provider.mu.Unlock()
	if schema == assemblyline.RoleplayCanonExtractionSchemaV1 {
		provider.terminalOnce.Do(func() { close(provider.terminalCanon) })
		<-provider.release
	}
	writeRoleplayBoundaryJSON(w, map[string]any{
		"model": modelName, "created_at": "2026-08-09T22:00:00Z", "response": candidate,
		"done": true, "done_reason": "stop", "total_duration": int64(101),
		"load_duration": int64(11), "prompt_eval_count": 41,
		"prompt_eval_duration": int64(21), "eval_count": 7, "eval_duration": int64(31),
	})
}

func (provider *roleplayBoundaryOllama) waitForTerminalCanon(t *testing.T) {
	t.Helper()
	select {
	case <-provider.terminalCanon:
	case <-time.After(15 * time.Second):
		t.Fatal("roleplay workflow did not reach the terminal canon boundary")
	}
}

func (provider *roleplayBoundaryOllama) releaseCompletion() {
	provider.releaseOnce.Do(func() { close(provider.release) })
}

func (provider *roleplayBoundaryOllama) recordUnexpected(value string) {
	provider.mu.Lock()
	defer provider.mu.Unlock()
	provider.unexpected = append(provider.unexpected, value)
}

func (provider *roleplayBoundaryOllama) assertCompleted(t *testing.T) {
	t.Helper()
	provider.mu.Lock()
	defer provider.mu.Unlock()
	if len(provider.unexpected) != 0 {
		t.Fatalf("unexpected provider traffic: %v", provider.unexpected)
	}
	wantSchemas := []string{
		assemblyline.ContextRelevanceSchemaV1,
		assemblyline.ContextMinificationSchemaV1,
		assemblyline.ConversationResponseSchemaV1,
		assemblyline.RoleplayOngoingStateLeafV1,
		assemblyline.RoleplayCanonExtractionSchemaV1,
	}
	if strings.Join(provider.stationSchemas, ",") != strings.Join(wantSchemas, ",") {
		t.Fatalf("station calls=%v want=%v", provider.stationSchemas, wantSchemas)
	}
	for index, modelName := range provider.generatedModels {
		wantModel := roleplayBoundarySemanticModel
		if provider.stationSchemas[index] == assemblyline.ConversationResponseSchemaV1 {
			wantModel = roleplayBoundaryNarrativeModel
		}
		if modelName != wantModel {
			t.Fatalf(
				"station %s used model %q instead of %q",
				provider.stationSchemas[index], modelName, wantModel,
			)
		}
	}
}

func roleplayBoundarySchema(payload map[string]any) string {
	format, _ := payload["format"].(map[string]any)
	properties, _ := format["properties"].(map[string]any)
	schema, _ := properties["schema"].(map[string]any)
	value, _ := schema["const"].(string)
	return value
}

func roleplayBoundaryShowResponse() map[string]any {
	return map[string]any{
		"capabilities": []string{"completion", "vision", "tools", "thinking"},
		"model_info": map[string]any{
			"general.architecture": "qwen35", "qwen35.context_length": 8192,
			"tokenizer.ggml.model": "gpt2", "tokenizer.ggml.pre": "qwen35",
			"tokenizer.ggml.add_eos_token": false, "tokenizer.ggml.add_padding_token": false,
			"tokenizer.ggml.tokens": nil, "tokenizer.ggml.token_type": nil, "tokenizer.ggml.merges": nil,
		},
		"template": "{{ .Prompt }}",
		"parameters": "temperature                    1\n" +
			"top_k                          20\n" +
			"top_p                          0.95\n" +
			"presence_penalty               1.5",
	}
}

func roleplayBoundaryModel(name string, running bool) map[string]any {
	digestRune := "a"
	if strings.Contains(name, "semantic") {
		digestRune = "b"
	}
	model := map[string]any{
		"name": name, "model": name, "digest": strings.Repeat(digestRune, 64),
		"size": int64(6_594_474_711), "modified_at": "2026-08-08T00:35:46-04:00",
		"details": map[string]any{
			"parent_model": "", "format": "gguf", "family": "qwen35",
			"families": []string{"qwen35"}, "parameter_size": "9.7B", "quantization_level": "Q4_K_M",
		},
	}
	if running {
		model["context_length"] = 8192
		model["size"] = int64(14_524_483_488)
		model["size_vram"] = int64(7_165_128_704)
		model["expires_at"] = "2026-08-09T17:22:58-04:00"
		delete(model, "modified_at")
	}
	return model
}

func writeRoleplayBoundaryJSON(w http.ResponseWriter, value any) {
	_ = json.NewEncoder(w).Encode(value)
}
