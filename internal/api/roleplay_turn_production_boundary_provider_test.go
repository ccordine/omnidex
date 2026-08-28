package api

import (
	"encoding/json"
	"fmt"
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
	roleplayBoundarySearchTerm      = "the current fictional ambush"
)

type roleplayBoundaryOllama struct {
	mu              sync.Mutex
	generatedModels []string
	stationKinds    []assemblyline.WorkKind
	responses       []string
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
	prompt, ok := payload["prompt"].(string)
	if !ok || strings.TrimSpace(prompt) == "" {
		provider.recordUnexpected("generate prompt is not a non-empty string")
		http.Error(w, `{"error":"invalid raw station prompt"}`, http.StatusBadRequest)
		return
	}
	if raw, _ := payload["raw"].(bool); !raw {
		provider.recordUnexpected("station request omitted raw=true")
		http.Error(w, `{"error":"non-raw station request"}`, http.StatusBadRequest)
		return
	}
	if _, structured := payload["format"]; structured {
		provider.recordUnexpected("station request included structured format")
		http.Error(w, `{"error":"structured station request"}`, http.StatusBadRequest)
		return
	}
	candidate, kind, terminalCanon, err := roleplayBoundaryRawResponse(prompt)
	if err != nil {
		provider.recordUnexpected(err.Error())
		http.Error(w, `{"error":"unexpected station"}`, http.StatusBadRequest)
		return
	}
	provider.mu.Lock()
	provider.generatedModels = append(provider.generatedModels, modelName)
	provider.stationKinds = append(provider.stationKinds, kind)
	provider.responses = append(provider.responses, candidate)
	provider.mu.Unlock()
	if terminalCanon {
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
	wantKinds := []assemblyline.WorkKind{
		assemblyline.WorkContextRelevanceSelection,
		assemblyline.WorkContextRelevanceSelection,
		assemblyline.WorkContextRelevanceSelection,
		assemblyline.WorkContextRelevanceSelection,
		assemblyline.WorkContextMinification,
		assemblyline.WorkConversationResponse,
		assemblyline.WorkRoleplayOngoingAction,
		assemblyline.WorkRoleplayCanonFactCoverage,
		assemblyline.WorkRoleplayCanonFact,
		assemblyline.WorkRoleplayCanonFactCoverage,
	}
	if len(provider.stationKinds) != len(wantKinds) {
		t.Fatalf("station calls=%v want=%v", provider.stationKinds, wantKinds)
	}
	for index, wantKind := range wantKinds {
		if provider.stationKinds[index] != wantKind {
			t.Fatalf("station calls=%v want=%v", provider.stationKinds, wantKinds)
		}
		if json.Valid([]byte(provider.responses[index])) {
			t.Fatalf(
				"station %s returned retired JSON response %q",
				provider.stationKinds[index], provider.responses[index],
			)
		}
		wantModel := roleplayBoundarySemanticModel
		if provider.stationKinds[index] == assemblyline.WorkConversationResponse {
			wantModel = roleplayBoundaryNarrativeModel
		}
		modelName := provider.generatedModels[index]
		if modelName != wantModel {
			t.Fatalf(
				"station %s used model %q instead of %q",
				provider.stationKinds[index], modelName, wantModel,
			)
		}
	}
}

func roleplayBoundaryRawResponse(
	prompt string,
) (string, assemblyline.WorkKind, bool, error) {
	switch {
	case strings.Contains(prompt, "Answer one semantic coverage relation: does the exact current instruction"):
		if strings.Contains(prompt, "ACCEPTED RETRIEVAL CONCEPTS:\n(none)") {
			return assemblyline.ContextTermRemains,
				assemblyline.WorkContextSearchTermCoverage, false, nil
		}
		return assemblyline.ContextNoUncoveredTerm,
			assemblyline.WorkContextSearchTermCoverage, false, nil
	case strings.Contains(prompt, "Return exactly one concise retrieval concept"):
		return roleplayBoundarySearchTerm, assemblyline.WorkContextSearchTerm, false, nil
	case strings.Contains(prompt, "CONTEXT_RELEVANCE_AUTHORITY:\n"):
		candidateID, err := roleplayBoundaryRelevantCandidate(prompt)
		return candidateID, assemblyline.WorkContextRelevanceSelection, false, err
	case strings.Contains(prompt, "CONTEXT_MINIFICATION_JSON:\n"):
		return "Mara is defending the archive.",
			assemblyline.WorkContextMinification, false, nil
	case strings.Contains(prompt, "ROLEPLAY_IDENTITY_JSON:\n"):
		return roleplayBoundaryReply, assemblyline.WorkConversationResponse, false, nil
	case strings.Contains(prompt, "ROLEPLAY_ONGOING_ACTION_JSON:\n"):
		return roleplayBoundaryAction, assemblyline.WorkRoleplayOngoingAction, false, nil
	case strings.Contains(prompt, "Answer one semantic coverage relation: does the exact current contribution"):
		if strings.Contains(prompt, "ACCEPTED CURRENT-CONTRIBUTION FACTS:\n(none)") {
			return assemblyline.RoleplayCanonFactRemains,
				assemblyline.WorkRoleplayCanonFactCoverage, false, nil
		}
		return assemblyline.RoleplayNoUncoveredCanonFact,
			assemblyline.WorkRoleplayCanonFactCoverage, true, nil
	case strings.Contains(prompt, "Return exactly one durable fictional fact established by the exact current contribution"):
		return roleplayBoundaryFact, assemblyline.WorkRoleplayCanonFact, false, nil
	default:
		return "", "", false, fmt.Errorf("unexpected raw roleplay station envelope")
	}
}

func roleplayBoundaryRelevantCandidate(prompt string) (string, error) {
	const marker = "CONTEXT_RELEVANCE_AUTHORITY:\n"
	index := strings.Index(prompt, marker)
	if index < 0 {
		return "", fmt.Errorf("raw context relevance prompt omitted authority")
	}
	var authority struct {
		Candidates []struct {
			CandidateID string `json:"candidate_id"`
		} `json:"candidates"`
		AcceptedCandidateIDs []string `json:"accepted_candidate_ids"`
	}
	if err := json.NewDecoder(strings.NewReader(prompt[index+len(marker):])).Decode(&authority); err != nil {
		return "", fmt.Errorf("decode raw context relevance authority: %w", err)
	}
	available := make(map[string]struct{}, len(authority.Candidates))
	for _, candidate := range authority.Candidates {
		available[candidate.CandidateID] = struct{}{}
	}
	accepted := make(map[string]struct{}, len(authority.AcceptedCandidateIDs))
	for _, candidateID := range authority.AcceptedCandidateIDs {
		accepted[candidateID] = struct{}{}
	}
	for _, candidateID := range []string{"CTX_3", "CTX_4", "CTX_5", "CTX_6"} {
		_, exists := available[candidateID]
		_, retained := accepted[candidateID]
		if exists && !retained {
			return candidateID, nil
		}
	}
	return assemblyline.ContextRelevanceNoCandidate, nil
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
