package worker

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/queue"
)

type startupTestLLM struct{}

func (startupTestLLM) Generate(context.Context, string, string) (string, error) {
	return "", nil
}

func (startupTestLLM) PrepareContextModel(context.Context, string, string) (llm.PreparedModel, error) {
	return llm.PreparedModel{}, nil
}

func (startupTestLLM) GeneratePrepared(context.Context, llm.PreparedModel) (string, error) {
	return "", nil
}

func (startupTestLLM) CleanupPreparedModel(llm.PreparedModel) {}

func (startupTestLLM) Embedding(context.Context, string) ([]float64, error) {
	return nil, nil
}

func (startupTestLLM) SuggestTags(context.Context, string, int) ([]string, error) {
	return nil, nil
}

func (startupTestLLM) SuggestTagsWithModel(context.Context, string, string, int) ([]string, error) {
	return nil, nil
}

func validWorkerOptions() Options {
	return Options{
		WorkerCount:            2,
		PollInterval:           time.Second,
		RetrievalLimit:         8,
		ContextBudget:          4000,
		InferenceContextTokens: 32768,
		Models: ModelRouting{
			Default:    "default-model",
			Fast:       "fast-model",
			Reasoning:  "reasoning-model",
			Tagging:    "tagging-model",
			Plan:       "plan-model",
			Analyze:    "analyze-model",
			Response:   "response-model",
			Search:     "search-model",
			Memory:     "memory-model",
			Specialist: map[string]string{"planner": "planner-model"},
		},
		Cognition: CognitionSettings{
			SufficientContextChars:  1400,
			MemoryInferenceMaxItems: 3,
		},
		Tournament: TournamentSettings{
			ChunkChars:   2200,
			SummaryChars: 750,
			MaxRounds:    4,
		},
		Workspace: WorkspaceSettings{
			MaxFiles:      5000,
			ContextBudget: 6000,
		},
		HallucinationRetryLimit: 2,
		OllamaRestartTimeout:    20 * time.Second,
		Logger:                  log.New(io.Discard, "", 0),
	}
}

func TestValidateWorkerOptionsRejectsMissingModelRole(t *testing.T) {
	opts := validWorkerOptions()
	opts.Models.Search = ""

	err := validateWorkerOptions(opts)
	if err == nil || !strings.Contains(err.Error(), "models.search") {
		t.Fatalf("validateWorkerOptions() error=%v, want models.search failure", err)
	}
}

func TestValidateWorkerOptionsRejectsInvalidRuntimeBounds(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Options)
		message string
	}{
		{name: "worker count", mutate: func(opts *Options) { opts.WorkerCount = 0 }, message: "worker_count"},
		{name: "poll interval", mutate: func(opts *Options) { opts.PollInterval = 0 }, message: "poll_interval"},
		{name: "retrieval limit", mutate: func(opts *Options) { opts.RetrievalLimit = 0 }, message: "retrieval_limit"},
		{name: "context budget", mutate: func(opts *Options) { opts.ContextBudget = 0 }, message: "context_budget"},
		{name: "inference context", mutate: func(opts *Options) { opts.InferenceContextTokens = 4095 }, message: "inference_context_tokens"},
		{name: "cognition", mutate: func(opts *Options) { opts.Cognition.SufficientContextChars = 0 }, message: "sufficient_context_chars"},
		{name: "memory items", mutate: func(opts *Options) { opts.Cognition.MemoryInferenceMaxItems = -1 }, message: "memory_inference_max_items"},
		{name: "tournament chunk", mutate: func(opts *Options) { opts.Tournament.ChunkChars = 499 }, message: "tournament.chunk_chars"},
		{name: "tournament summary", mutate: func(opts *Options) { opts.Tournament.SummaryChars = 119 }, message: "tournament.summary_chars"},
		{name: "tournament rounds", mutate: func(opts *Options) { opts.Tournament.MaxRounds = 9 }, message: "tournament.max_rounds"},
		{name: "workspace files", mutate: func(opts *Options) { opts.Workspace.MaxFiles = 0 }, message: "workspace.max_files"},
		{name: "workspace budget", mutate: func(opts *Options) { opts.Workspace.ContextBudget = 0 }, message: "workspace.context_budget"},
		{name: "retry limit", mutate: func(opts *Options) { opts.HallucinationRetryLimit = 7 }, message: "hallucination_retry_limit"},
		{name: "restart timeout", mutate: func(opts *Options) { opts.OllamaRestartTimeout = 0 }, message: "ollama_restart_timeout"},
		{name: "logger", mutate: func(opts *Options) { opts.Logger = nil }, message: "logger"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			opts := validWorkerOptions()
			test.mutate(&opts)
			err := validateWorkerOptions(opts)
			if err == nil || !strings.Contains(err.Error(), test.message) {
				t.Fatalf("validateWorkerOptions() error=%v, want %q failure", err, test.message)
			}
		})
	}
}

func TestNewRejectsMissingV3SkillRegistry(t *testing.T) {
	opts := validWorkerOptions()
	opts.V3Enabled = true
	opts.SkillsRoot = t.TempDir() + "/missing"

	service, err := New(&queue.Repository{}, startupTestLLM{}, nil, opts)
	if err == nil {
		t.Fatal("New() error=nil, want missing skills failure")
	}
	if service != nil {
		t.Fatal("New() returned a service after skill registry failure")
	}
	if !strings.Contains(err.Error(), "load V3 skill registry") {
		t.Fatalf("New() error=%v, want V3 skill registry context", err)
	}
}
