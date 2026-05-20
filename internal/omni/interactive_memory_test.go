package omni

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

type fakePromptTagger struct {
	results []PromptTagResult
	errors  []error
	inputs  []PromptTagInput
}

func (f *fakePromptTagger) TagPrompt(ctx context.Context, input PromptTagInput) (PromptTagResult, error) {
	f.inputs = append(f.inputs, input)
	if len(f.errors) > 0 {
		err := f.errors[0]
		f.errors = f.errors[1:]
		if err != nil {
			return PromptTagResult{}, err
		}
	}
	if len(f.results) == 0 {
		return PromptTagResult{}, nil
	}
	result := f.results[0]
	f.results = f.results[1:]
	return result, nil
}

func TestInteractiveTurnLoadsTaggedPGMemoryIntoSummaryAndPersistsPromptResponse(t *testing.T) {
	out := &bytes.Buffer{}
	app := NewApp(strings.NewReader(""), out, &bytes.Buffer{})
	runner := newFakeMemoryRunner()
	app.memory = NewPGMemoryStore(runner)
	app.promptTagger = &fakePromptTagger{results: []PromptTagResult{{Tags: []string{"React", "Project"}}}}
	summarizer := &fakeContextSummarizer{contexts: []MinimalContext{{
		Summary: "memory-aware context",
		Facts:   []string{"loaded prior memory"},
	}}}
	app.contextSummarizer = summarizer
	app.runLogger, _ = NewRunLogger(t.TempDir(), "interactive-memory-test")
	defer app.runLogger.Close()

	if _, err := app.memory.AddMemory(context.Background(), "memory_specialist", "preference", "Existing memory: prefer minimal React scaffolds unless asked otherwise.", []string{"react", "project"}); err != nil {
		t.Fatal(err)
	}
	app.plannerClient = &fakeCommandDecisionClient{responses: []string{
		`{"command":"printf 'created react project\n'","done":false,"answer":""}`,
		`{"command":"","done":true,"answer":"Created the React project."}`,
	}}

	session := &Session{
		WorkspacePath:       t.TempDir(),
		WorkspaceHash:       "interactive-memory-test",
		ActiveDirectoryPath: t.TempDir(),
		Permission:          PermissionFull,
	}
	turn, response, err := app.handleTurn(session, "create a new React project", &activityIndicator{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(response, "Created the React project") {
		t.Fatalf("response missing planner answer:\n%s", response)
	}
	if len(summarizer.inputs) == 0 {
		t.Fatal("context summarizer was not called")
	}
	if !sessionMemoriesContain(summarizer.inputs[0].SessionMemories, "prefer minimal React scaffolds") {
		t.Fatalf("summary input missing retrieved memory: %#v", summarizer.inputs[0].SessionMemories)
	}
	if countEventsOfType(turn.Events, "memory_tags_generated") != 1 {
		t.Fatalf("missing memory_tags_generated event: %#v", turn.Events)
	}
	if countEventsOfType(turn.Events, "memory_context_loaded") != 1 {
		t.Fatalf("missing memory_context_loaded event: %#v", turn.Events)
	}
	if countEventsOfType(turn.Events, "memory_turn_persisted") != 1 {
		t.Fatalf("missing memory_turn_persisted event: %#v", turn.Events)
	}

	promptMemories, err := app.memory.SearchMemory(context.Background(), "create a new React project", []string{"prompt", "react"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !memoryRecordsContain(promptMemories, "create a new React project") {
		t.Fatalf("prompt memory not persisted: %#v", promptMemories)
	}
	responseMemories, err := app.memory.SearchMemory(context.Background(), "Created the React project", []string{"response", "react"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if !memoryRecordsContain(responseMemories, "Created the React project") {
		t.Fatalf("response memory not persisted: %#v", responseMemories)
	}
	if !runner.SawSQL("INSERT INTO memory_chunks") || !runner.SawSQL("INSERT INTO tags") || !runner.SawSQL("FROM memory_chunks") {
		t.Fatalf("expected memory insert/search SQL, got:\n%s", strings.Join(runner.SQLLog, "\n---\n"))
	}
}

func TestInteractiveMemorySkipsRetrievalWithoutSpecialistTags(t *testing.T) {
	app := NewApp(strings.NewReader(""), &bytes.Buffer{}, &bytes.Buffer{})
	app.memory = NewPGMemoryStore(newFakeMemoryRunner())
	app.promptTagger = &fakePromptTagger{results: []PromptTagResult{{Tags: nil}}}

	events := []Event{}
	ctx := app.loadInteractiveMemoryContext(context.Background(), "hello", t.TempDir(), func(eventType, summary string, details map[string]string) {
		events = append(events, Event{Type: eventType, Summary: summary, Details: details})
	})
	if len(ctx.Tags) != 0 || len(ctx.Memories) != 0 {
		t.Fatalf("context = %#v, want empty", ctx)
	}
	if countEventsOfType(events, "memory_context_skipped") != 1 {
		t.Fatalf("expected memory_context_skipped event: %#v", events)
	}
}

func sessionMemoriesContain(memories []SessionMemory, needle string) bool {
	for _, memory := range memories {
		if strings.Contains(strings.ToLower(memory.Content), strings.ToLower(needle)) {
			return true
		}
	}
	return false
}
