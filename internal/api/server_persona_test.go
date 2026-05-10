package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
)

type fakeLLMClient struct {
	outputs            []string
	preparePrompts     []string
	generatePromptHint []string
}

func (f *fakeLLMClient) Generate(ctx context.Context, model, prompt string) (string, error) {
	prepared, err := f.PrepareContextModel(ctx, model, prompt)
	if err != nil {
		return "", err
	}
	prepared.PromptHint = llm.MinimalGeneratePrompt
	return f.GeneratePrepared(ctx, prepared)
}

func (f *fakeLLMClient) PrepareContextModel(_ context.Context, model, prompt string) (llm.PreparedModel, error) {
	f.preparePrompts = append(f.preparePrompts, prompt)
	if strings.TrimSpace(model) == "" {
		model = "fake-default-model"
	}
	return llm.PreparedModel{
		BaseModel:    strings.TrimSpace(model),
		ContextModel: strings.TrimSpace(model),
		Prompt:       strings.TrimSpace(prompt),
	}, nil
}

func (f *fakeLLMClient) GeneratePrepared(_ context.Context, prepared llm.PreparedModel) (string, error) {
	f.generatePromptHint = append(f.generatePromptHint, strings.TrimSpace(prepared.PromptHint))
	if len(f.outputs) == 0 {
		return "ok", nil
	}
	next := f.outputs[0]
	f.outputs = f.outputs[1:]
	return next, nil
}

func (f *fakeLLMClient) CleanupPreparedModel(_ llm.PreparedModel) {}

func (f *fakeLLMClient) Embedding(context.Context, string) ([]float64, error) {
	return nil, nil
}

func (f *fakeLLMClient) SuggestTags(context.Context, string, int) ([]string, error) {
	return nil, nil
}

func (f *fakeLLMClient) SuggestTagsWithModel(context.Context, string, string, int) ([]string, error) {
	return nil, nil
}

func TestInstructRouteUsesPromptAsPromptHint(t *testing.T) {
	llmClient := &fakeLLMClient{outputs: []string{"instruction-output"}}
	server := NewServer(nil, llmClient)

	body := `{
		"model":"test-model",
		"system":"You are a deterministic assistant.",
		"context":{"setting":"dockyard","temperature":"rainy"},
		"history":[{"role":"assistant","content":"Prior turn"}],
		"prompt":"Open the gate."
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/instruct", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	if len(llmClient.generatePromptHint) != 1 {
		t.Fatalf("generate calls=%d want 1", len(llmClient.generatePromptHint))
	}
	if llmClient.generatePromptHint[0] != "Open the gate." {
		t.Fatalf("prompt hint=%q want %q", llmClient.generatePromptHint[0], "Open the gate.")
	}

	if len(llmClient.preparePrompts) != 1 {
		t.Fatalf("prepare calls=%d want 1", len(llmClient.preparePrompts))
	}
	compiledPrompt := llmClient.preparePrompts[0]
	if !strings.Contains(compiledPrompt, "REQUEST_CONTEXT_JSON") || !strings.Contains(compiledPrompt, "dockyard") {
		t.Fatalf("compiled prompt missing context: %q", compiledPrompt)
	}

	var payload personaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Persona != "instruct" {
		t.Fatalf("persona=%q want %q", payload.Persona, "instruct")
	}
	if payload.Output != "instruction-output" {
		t.Fatalf("output=%q want %q", payload.Output, "instruction-output")
	}
}

func TestReasoningRouteRunsThreeStageChain(t *testing.T) {
	llmClient := &fakeLLMClient{outputs: []string{"parse-output", "deliberate-output", "final-output"}}
	server := NewServer(nil, llmClient)

	body := `{
		"model":"reasoning-model",
		"system":"Interpret world state before answering.",
		"context":{"world":"test"},
		"history":[{"role":"user","content":"Earlier action"}],
		"prompt":"What should happen next?"
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/reasoning", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}

	if len(llmClient.generatePromptHint) != 3 {
		t.Fatalf("generate calls=%d want 3", len(llmClient.generatePromptHint))
	}
	for i, hint := range llmClient.generatePromptHint {
		if hint != "What should happen next?" {
			t.Fatalf("call=%d prompt hint=%q", i+1, hint)
		}
	}

	if len(llmClient.preparePrompts) != 3 {
		t.Fatalf("prepare calls=%d want 3", len(llmClient.preparePrompts))
	}
	if !strings.Contains(llmClient.preparePrompts[1], "parse-output") {
		t.Fatalf("deliberation stage prompt missing parse output: %q", llmClient.preparePrompts[1])
	}
	if !strings.Contains(llmClient.preparePrompts[2], "deliberate-output") {
		t.Fatalf("final stage prompt missing deliberate output: %q", llmClient.preparePrompts[2])
	}

	var payload personaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Output != "final-output" {
		t.Fatalf("output=%q want %q", payload.Output, "final-output")
	}
	if len(payload.Stages) != 3 {
		t.Fatalf("stages=%d want 3", len(payload.Stages))
	}
	if payload.Stages[0].Name != "parse" || payload.Stages[2].Name != "final" {
		t.Fatalf("stage names=%+v", payload.Stages)
	}
}

func TestQueueRoutesNotRegisteredWhenRepoIsNil(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})
	req := httptest.NewRequest(http.MethodGet, "/v1/jobs", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusNotFound)
	}
}

func TestInstructRouteEnqueueIntegrationQueuesJobWithoutLLMCall(t *testing.T) {
	llmClient := &fakeLLMClient{outputs: []string{"llm-should-not-run"}}
	server := NewServer(nil, llmClient)

	server.instructIntegration.enqueue = func(_ context.Context, instruction, pipeline string, metadata json.RawMessage) (model.Job, error) {
		if instruction != "Create release checklist" {
			t.Fatalf("instruction=%q want %q", instruction, "Create release checklist")
		}
		if pipeline != model.PipelineAssistant {
			t.Fatalf("pipeline=%q want %q", pipeline, model.PipelineAssistant)
		}
		if strings.TrimSpace(string(metadata)) != `{"source":"integration-test"}` {
			t.Fatalf("metadata=%s", strings.TrimSpace(string(metadata)))
		}
		return model.Job{
			ID:          77,
			Instruction: instruction,
			Pipeline:    pipeline,
			Status:      model.JobStatusPending,
			Metadata:    metadata,
		}, nil
	}

	body := `{
		"prompt":"Create release checklist",
		"integration":{
			"action":"enqueue_job",
			"pipeline":"assistant",
			"metadata":{"source":"integration-test"}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/instruct", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(llmClient.generatePromptHint) != 0 {
		t.Fatalf("llm calls=%d want 0", len(llmClient.generatePromptHint))
	}

	var payload personaResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Integration == nil {
		t.Fatalf("integration payload missing")
	}
	if payload.Integration.Action != "enqueue_job" {
		t.Fatalf("integration.action=%q", payload.Integration.Action)
	}
	if payload.Integration.Job == nil || payload.Integration.Job.ID != 77 {
		t.Fatalf("integration.job=%+v", payload.Integration.Job)
	}
}

func TestInstructRouteEnqueueIntegrationFailsWhenQueueUnavailable(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})

	body := `{
		"prompt":"Create release checklist",
		"integration":{"action":"enqueue_job"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/instruct", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "queue integration is unavailable") {
		t.Fatalf("unexpected body=%s", rec.Body.String())
	}
}

func TestInstructRouteRejectsUnsupportedIntegrationAction(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})

	body := `{
		"prompt":"Create release checklist",
		"integration":{"action":"explode_sun"}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/instruct", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unsupported integration.action") {
		t.Fatalf("unexpected body=%s", rec.Body.String())
	}
}

func TestInstructRouteIntegrationAllowsInstructionWithoutPrompt(t *testing.T) {
	server := NewServer(nil, &fakeLLMClient{})

	server.instructIntegration.enqueue = func(_ context.Context, instruction, pipeline string, metadata json.RawMessage) (model.Job, error) {
		return model.Job{
			ID:          501,
			Instruction: instruction,
			Pipeline:    pipeline,
			Status:      model.JobStatusPending,
			Metadata:    metadata,
		}, nil
	}

	body := `{
		"integration":{
			"action":"enqueue_job",
			"instruction":"Generate release candidate checklist"
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/v1/instruct", strings.NewReader(body))
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
