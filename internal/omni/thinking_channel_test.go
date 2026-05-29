package omni

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeThinkingClient struct {
	responses []string
	calls     int
}

func (f *fakeThinkingClient) ChatRaw(ctx context.Context, req OllamaChatRequest) (OllamaChatResponse, error) {
	if f.calls >= len(f.responses) {
		return OllamaChatResponse{Content: `{"thought":"done","done":true,"conclusion":"no further action"}`}, nil
	}
	resp := f.responses[f.calls]
	f.calls++
	return OllamaChatResponse{Content: resp, Thinking: "native reasoning trace"}, nil
}

func TestThoughtChannelStorePersistsMessages(t *testing.T) {
	store, err := NewThoughtChannelStore(t.TempDir(), "ws_test", "turn_001")
	if err != nil {
		t.Fatal(err)
	}
	channel := ThoughtChannel{ID: "ch_1", Trigger: "test", CreatedAt: nowUTC(), UpdatedAt: nowUTC()}
	if err := store.AppendChannel(channel); err != nil {
		t.Fatal(err)
	}
	msg := ThoughtMessage{ChannelID: "ch_1", Role: "assistant", Content: "diagnose failure", CreatedAt: nowUTC()}
	if err := store.AppendMessage(msg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.filePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "diagnose failure") {
		t.Fatalf("expected persisted thought message, got %q", string(data))
	}
}

func TestOllamaThinkingServiceRunsToolThenConcludes(t *testing.T) {
	client := &fakeThinkingClient{responses: []string{
		`{"thought":"need observations","tool":"observations","done":false}`,
		`{"thought":"write substantive app files","done":true,"conclusion":"scaffold missing","recovery_tool_task":"Create src/App.js with graphing calculator UI"}`,
	}}
	store, err := NewThoughtChannelStore(t.TempDir(), "ws_think", "turn_002")
	if err != nil {
		t.Fatal(err)
	}
	svc := NewOllamaThinkingService(client, store, 4)
	events := []StructuredCommandEvent{}
	result, err := svc.Reason(context.Background(), ThinkingInput{
		TurnID:     "turn_002",
		Step:       3,
		Trigger:    "progression_gate_force_recovery",
		UserPrompt: "Build a graphing calculator in React",
		WorkingDir: t.TempDir(),
		GateReason: "empty project files remain",
		ActivePrompt: NewActivePromptContext("Build a graphing calculator in React", "", nil),
	}, func(evt StructuredCommandEvent) { events = append(events, evt) })
	if err != nil {
		t.Fatal(err)
	}
	if result.RecoveryToolTask == "" {
		t.Fatalf("expected recovery tool task, got %#v", result)
	}
	if client.calls < 2 {
		t.Fatalf("expected at least 2 thinking calls, got %d", client.calls)
	}
	if !structuredEventsContain(events, "thinking_channel_started") || !structuredEventsContain(events, "thinking_channel_concluded") {
		t.Fatalf("missing thinking lifecycle events: %#v", events)
	}
}

func TestExecuteThinkingToolObservations(t *testing.T) {
	out := executeThinkingTool(context.Background(), ThinkingToolDeps{}, "observations", "", ThinkingInput{
		Observations: []StructuredCommandObservation{{Step: 1, Command: "touch src/App.js", ExitCode: 1, Stderr: "rejected"}},
	}, nil, "ch_test")
	if !strings.Contains(out, "touch src/App.js") {
		t.Fatalf("unexpected tool output: %s", out)
	}
}

func TestThinkingModelFromEnv(t *testing.T) {
	t.Setenv("OMNI_THINKING_MODEL", "custom-think:latest")
	if got := thinkingModelFromEnv(); got != "custom-think:latest" {
		t.Fatalf("model = %q", got)
	}
}

func TestNewThoughtChannelStoreCreatesWorkspacePath(t *testing.T) {
	root := t.TempDir()
	store, err := NewThoughtChannelStore(root, "abc123", "turn_x")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "abc123", "turn_x", "channels.jsonl")
	if store.filePath != want {
		t.Fatalf("filePath = %q, want %q", store.filePath, want)
	}
}

func TestParseThinkingStepPayload(t *testing.T) {
	payload, err := parseThinkingStepPayload(`{"thought":"analyze","tool":"project_map","done":false}`)
	if err != nil {
		t.Fatal(err)
	}
	if payload.Tool != "project_map" || payload.Thought != "analyze" {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestBuildThinkingContextPayloadIncludesActivePrompt(t *testing.T) {
	raw := buildThinkingContextPayload(ThinkingInput{
		Trigger:      "loop_exhausted",
		ActivePrompt: NewActivePromptContext("graphing calculator", "", nil),
	}, thinkingModeRecovery)
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["active_prompt"]; !ok {
		t.Fatalf("missing active_prompt in payload: %s", raw)
	}
}
