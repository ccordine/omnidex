package worker

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/cognitionpolicy"
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

func (startupTestLLM) RequireExactPreparedContract() error { return nil }

func (startupTestLLM) ValidateExactPreparedContract(llm.PreparedModel) error { return nil }

func (startupTestLLM) Embedding(context.Context, string) ([]float64, error) {
	return nil, nil
}

func validWorkerOptions() Options {
	sampling, err := cognitionpolicy.NewSamplingIdentity(32768, 24576, 4096)
	if err != nil {
		panic(err)
	}
	brain, err := cognitionpolicy.NewBrainRef(
		"analyze-model", strings.Repeat("a", 64), "Q4_K_M",
		"ollama", "test-version", "test-hardware", sampling,
	)
	if err != nil {
		panic(err)
	}
	return Options{
		WorkerCount:            2,
		FragmentConcurrency:    1,
		PollInterval:           time.Second,
		RetrievalLimit:         8,
		ContextBudget:          4000,
		InferenceContextTokens: 32768,
		EmbeddingProvider:      "ollama",
		EmbeddingModel:         "nomic-embed-text",
		Models: ModelRouting{
			Default:    "default-model",
			Fast:       "fast-model",
			Glue:       "glue-model",
			Reasoning:  "reasoning-model",
			Tagging:    "tagging-model",
			Plan:       "plan-model",
			Analyze:    "analyze-model",
			Response:   "response-model",
			Search:     "search-model",
			Memory:     "memory-model",
			Specialist: map[string]string{"planner": "planner-model"},
		},
		CognitionBrain: brain,
		Workspace: WorkspaceSettings{
			MaxFiles:      5000,
			ContextBudget: 6000,
		},
		SkillsRoot: "skills",
		Logger:     log.New(io.Discard, "", 0),
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
		{name: "fragment concurrency", mutate: func(opts *Options) { opts.FragmentConcurrency = 0 }, message: "fragment_concurrency"},
		{name: "poll interval", mutate: func(opts *Options) { opts.PollInterval = 0 }, message: "poll_interval"},
		{name: "retrieval limit", mutate: func(opts *Options) { opts.RetrievalLimit = 0 }, message: "retrieval_limit"},
		{name: "context budget", mutate: func(opts *Options) { opts.ContextBudget = 0 }, message: "context_budget"},
		{name: "inference context", mutate: func(opts *Options) { opts.InferenceContextTokens = 4095 }, message: "inference_context_tokens"},
		{name: "embedding provider", mutate: func(opts *Options) { opts.EmbeddingProvider = "" }, message: "embedding_provider"},
		{name: "embedding model", mutate: func(opts *Options) { opts.EmbeddingModel = "" }, message: "embedding_model"},
		{name: "cognition brain", mutate: func(opts *Options) { opts.CognitionBrain.Model = "" }, message: "cognition_brain"},
		{name: "workspace files", mutate: func(opts *Options) { opts.Workspace.MaxFiles = 0 }, message: "workspace.max_files"},
		{name: "workspace budget", mutate: func(opts *Options) { opts.Workspace.ContextBudget = 0 }, message: "workspace.context_budget"},
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

func TestNewRejectsMissingSkillRegistry(t *testing.T) {
	opts := validWorkerOptions()
	opts.SkillsRoot = t.TempDir() + "/missing"

	service, err := New(&queue.Repository{}, startupTestLLM{}, nil, opts)
	if err == nil {
		t.Fatal("New() error=nil, want missing skills failure")
	}
	if service != nil {
		t.Fatal("New() returned a service after skill registry failure")
	}
	if !strings.Contains(err.Error(), "load specialist registry") {
		t.Fatalf("New() error=%v, want specialist registry context", err)
	}
}
