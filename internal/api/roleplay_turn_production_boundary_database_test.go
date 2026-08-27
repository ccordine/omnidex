package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/ollama"
	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/roleplay"
	"github.com/gryph/omnidex/internal/station"
	"github.com/gryph/omnidex/internal/worker"
)

const (
	roleplayBoundaryAlias     = "qwen3.5:9b"
	roleplayBoundaryCanonical = "qwen3.5:9b-q4_K_M"
	roleplayBoundaryReply     = "Mara looks up. \u201cHey, Gryph.\u201d"
)

func TestTypedRoleplayTurnCompletesAcrossHTTPQueueWorkerAndCanonicalRunner(t *testing.T) {
	pool := openIsolatedAPIMigrationPool(t)
	repository := queue.New(pool)
	if err := repository.EnsureSchema(t.Context(), loadAPITestMigrationBundle(t)); err != nil {
		t.Fatal(err)
	}
	roleplayStore, err := roleplay.NewStore(pool)
	if err != nil {
		t.Fatal(err)
	}

	channel, err := repository.CreateRoleplayChannel(t.Context(), model.Channel{
		ID:            "roleplay-production-boundary",
		Scope:         model.ChannelScopeUser,
		Name:          "Roleplay production boundary",
		WorkspaceRoot: "/srv/workspaces/roleplay-production-boundary",
		Mode:          model.ChannelModeRoleplay,
	}, "Boundary World", "Mara")
	if err != nil {
		t.Fatal(err)
	}
	world, found, err := roleplayStore.FindWorldByChannel(t.Context(), string(channel.ID))
	if err != nil || !found {
		t.Fatalf("find roleplay world: found=%t error=%v", found, err)
	}
	gryph, err := roleplayStore.CreateCharacter(t.Context(), world.ID, "Gryph")
	if err != nil {
		t.Fatal(err)
	}
	maraID := string(channel.RoleplayViewpointCharacterID)
	writeBoundaryPersona(t, roleplayStore, maraID, "An attentive archivist.", "Direct and warm.")
	writeBoundaryPersona(t, roleplayStore, gryph.ID, "A visiting artificer.", "Plainspoken.")
	if _, err := roleplayStore.WriteCharacterGeneration(t.Context(), roleplay.CharacterGenerationWriteRequest{
		WorldID: world.ID, CharacterID: maraID, ExpectedRevision: 1,
		NarrativeModel: roleplayBoundaryAlias,
	}); err != nil {
		t.Fatal(err)
	}
	sceneID, err := roleplay.NewSceneIdentity()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := roleplayStore.CreateCurrentScene(t.Context(), roleplay.SceneSetup{
		ID: sceneID, WorldID: world.ID, Title: "Quiet Archive",
		Description:    "Mara waits beside a reading table.",
		ParticipantIDs: []string{maraID, gryph.ID},
	}); err != nil {
		t.Fatal(err)
	}

	provider := newRoleplayBoundaryOllama()
	fakeOllama := httptest.NewServer(http.HandlerFunc(provider.serveHTTP))
	t.Cleanup(fakeOllama.Close)
	client := ollama.New(fakeOllama.URL, roleplayBoundaryAlias, "nomic-embed-text", 5*time.Second, 8192)
	routes := make(map[station.ID]string, len(station.All()))
	for _, stationID := range station.All() {
		routes[stationID] = roleplayBoundaryAlias
	}
	service, err := worker.New(repository, client, client, nil, worker.Options{
		WorkerCount: 1, FragmentConcurrency: 1, PollInterval: 5 * time.Millisecond,
		InferenceContextTokens: 8192, InferenceProvider: "ollama",
		EmbeddingProvider: "ollama", EmbeddingModel: "nomic-embed-text",
		Models: worker.ModelRouting{Stations: routes}, Logger: log.New(io.Discard, "", 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	workerCtx, stopWorker := context.WithCancel(context.Background())
	workerDone := make(chan error, 1)
	go func() { workerDone <- service.Start(workerCtx) }()
	t.Cleanup(func() {
		stopWorker()
		select {
		case err := <-workerDone:
			if err != nil {
				t.Errorf("stop worker: %v", err)
			}
		case <-time.After(2 * time.Second):
			t.Error("worker did not stop")
		}
	})

	server := NewServer(nil, nil)
	server.repo = repository
	server.channelStore = repository
	server.roleplaySimulation = roleplayStore
	server.enqueueRoleplayChannelTurn = repository.EnqueueRoleplayChannelTurn
	server.mux = http.NewServeMux()
	server.routes()
	body, err := json.Marshal(map[string]any{
		"prompt": "[Message]\nHey",
		"roleplay_turn": roleplay.UserTurnRequest{
			PersonaKind: roleplay.UserPersonaCharacter, CharacterID: gryph.ID,
			ContributionKind: roleplay.UserContributionDialogue,
			Parts:            []roleplay.UserTurnPart{{Kind: roleplay.UserTurnPartMessage, Text: "Hey"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/v1/channels/"+string(channel.ID)+"/messages", bytes.NewReader(body))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("submit status=%d body=%s", response.Code, response.Body.String())
	}
	var submitted channelMessageResponse
	if err := json.Unmarshal(response.Body.Bytes(), &submitted); err != nil {
		t.Fatal(err)
	}
	waitForRoleplayBoundaryJob(t, repository, submitted.Job.ID)

	page, err := repository.ListChannelMessages(t.Context(), channel.ID, 10, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 2 {
		t.Fatalf("completed transcript has %d messages: %#v", len(page.Messages), page.Messages)
	}
	userMessage, assistantMessage := page.Messages[0], page.Messages[1]
	if userMessage.Role != model.ChannelMessageRoleUser || userMessage.SpeakerName != "Gryph" ||
		userMessage.Content != "[Message]\nHey" || userMessage.Turn == nil ||
		userMessage.Turn.Status != model.JobStatusCompleted || userMessage.Roleplay == nil ||
		userMessage.Roleplay.CharacterID != model.RoleplayCharacterID(gryph.ID) ||
		userMessage.Roleplay.ContributionKind != string(roleplay.UserContributionDialogue) {
		t.Fatalf("typed user transcript authority changed: %#v", userMessage)
	}
	if assistantMessage.Role != model.ChannelMessageRoleAssistant ||
		assistantMessage.SpeakerName != "Mara" || assistantMessage.Content != roleplayBoundaryReply {
		t.Fatalf("assistant transcript was not published: %#v", assistantMessage)
	}
	provider.assertCompleted(t)
}

func writeBoundaryPersona(t *testing.T, store *roleplay.Store, characterID, summary, voice string) {
	t.Helper()
	if _, err := store.WritePersona(t.Context(), roleplay.PersonaWriteRequest{
		CharacterID: characterID, ExpectedRevision: 0,
		Sheet: roleplay.PersonaSheet{Summary: summary, Voice: voice, Traits: []string{}, Goals: []string{}},
	}); err != nil {
		t.Fatal(err)
	}
}

func waitForRoleplayBoundaryJob(t *testing.T, repository *queue.Repository, jobID int64) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		details, err := repository.CurrentJobDetails(t.Context(), jobID)
		if err != nil {
			t.Fatal(err)
		}
		switch details.Job.Status {
		case model.JobStatusCompleted:
			return
		case model.JobStatusFailed:
			t.Fatalf("roleplay job failed: %s", details.Job.Error)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("roleplay job %d did not finish", jobID)
}

type roleplayBoundaryOllama struct {
	mu              sync.Mutex
	generatedModels []string
	stationSchemas  []string
	unexpected      []string
}

func newRoleplayBoundaryOllama() *roleplayBoundaryOllama { return &roleplayBoundaryOllama{} }

func (provider *roleplayBoundaryOllama) serveHTTP(w http.ResponseWriter, request *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch request.URL.Path {
	case "/api/version":
		writeRoleplayBoundaryJSON(w, map[string]any{"version": "0.24.0"})
	case "/api/tags":
		writeRoleplayBoundaryJSON(w, map[string]any{"models": []any{roleplayBoundaryModel(roleplayBoundaryAlias, false)}})
	case "/api/show":
		writeRoleplayBoundaryJSON(w, roleplayBoundaryShowResponse())
	case "/api/ps":
		writeRoleplayBoundaryJSON(w, map[string]any{"models": []any{roleplayBoundaryModel(roleplayBoundaryCanonical, true)}})
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
	case assemblyline.ConversationResponseSchemaV1:
		raw, _ := json.Marshal(assemblyline.ConversationResponseDecision{
			Schema: assemblyline.ConversationResponseSchemaV1, Text: roleplayBoundaryReply,
		})
		candidate = string(raw)
	case assemblyline.RoleplayCanonExtractionSchemaV1:
		candidate = `{"schema":"omnidex.roleplay-canon-extraction.v1","facts":[]}`
	case assemblyline.RoleplayOngoingStateLeafV1:
		candidate = `{"schema":"omnidex.roleplay-ongoing-action.v1","ongoing_action":null}`
	default:
		provider.recordUnexpected("station schema " + schema)
		http.Error(w, `{"error":"unexpected station"}`, http.StatusBadRequest)
		return
	}
	provider.mu.Lock()
	provider.generatedModels = append(provider.generatedModels, modelName)
	provider.stationSchemas = append(provider.stationSchemas, schema)
	provider.mu.Unlock()
	writeRoleplayBoundaryJSON(w, map[string]any{
		"model": modelName, "created_at": "2026-08-09T22:00:00Z", "response": candidate,
		"done": true, "done_reason": "stop", "total_duration": int64(101),
		"load_duration": int64(11), "prompt_eval_count": 41,
		"prompt_eval_duration": int64(21), "eval_count": 7, "eval_duration": int64(31),
	})
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
		assemblyline.RoleplayCanonExtractionSchemaV1,
		assemblyline.ConversationResponseSchemaV1,
		assemblyline.RoleplayOngoingStateLeafV1,
		assemblyline.RoleplayCanonExtractionSchemaV1,
	}
	if strings.Join(provider.stationSchemas, ",") != strings.Join(wantSchemas, ",") {
		t.Fatalf("station calls=%v want=%v", provider.stationSchemas, wantSchemas)
	}
	for _, modelName := range provider.generatedModels {
		if modelName != roleplayBoundaryAlias {
			t.Fatalf("generation used model %q instead of configured alias", modelName)
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
	model := map[string]any{
		"name": name, "model": name, "digest": strings.Repeat("a", 64),
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
