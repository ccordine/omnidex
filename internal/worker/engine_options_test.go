package worker

import (
	"context"
	"io"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
	"github.com/gryph/omnidex/internal/station"
)

type startupTestLLM struct{}

func (startupTestLLM) RequireExactPreparedContract() error { return nil }

func (startupTestLLM) ValidateExactPreparedProvider(expected llm.ProviderIdentityExpectation) error {
	return expected.Validate()
}

func (startupTestLLM) ValidateExactPreparedContract(llm.PreparedModel) error { return nil }

func (startupTestLLM) GeneratePreparedExact(
	context.Context,
	llm.PreparedModel,
) (llm.PreparedGeneration, error) {
	return llm.PreparedGeneration{}, nil
}

func (startupTestLLM) DiscoverProviderIdentityEvidence(
	context.Context,
	llm.ProviderIdentitySelection,
	string,
) (llm.ObservedProviderIdentity, error) {
	return llm.ObservedProviderIdentity{}, nil
}

func (startupTestLLM) Embedding(context.Context, string) ([]float64, error) {
	return nil, nil
}

func validWorkerOptions() Options {
	return Options{
		WorkerCount:               2,
		FragmentConcurrency:       1,
		PollInterval:              time.Second,
		InferenceContextTokens:    32768,
		EmbeddingProvider:         "ollama",
		EmbeddingModel:            "nomic-embed-text",
		ObjectiveAdvisoryProvider: "ollama",
		Models: ModelRouting{
			Stations: validStationModels(),
		},
		Workspace: WorkspaceSettings{},
		Logger:    log.New(io.Discard, "", 0),
	}
}

func validStationModels() map[station.ID]string {
	models := make(map[station.ID]string, len(station.All()))
	for _, id := range station.All() {
		models[id] = "station-model"
	}
	return models
}

func TestValidateWorkerOptionsDoesNotRequireBroadModelRoles(t *testing.T) {
	opts := validWorkerOptions()
	if err := validateWorkerOptions(opts); err != nil {
		t.Fatalf("validateWorkerOptions() rejected station-only routing: %v", err)
	}
}

func TestWorkerObjectiveAdvisoryModeIsOffByDefaultAndRejectsUnknownValues(t *testing.T) {
	opts := validWorkerOptions()
	if err := validateWorkerOptions(opts); err != nil {
		t.Fatal(err)
	}
	if got := normalizeWorkerOptions(opts).ObjectiveAdvisoryMode; got != objectiveadvisory.ModeOff {
		t.Fatalf("normalized mode=%q want off", got)
	}

	opts.ObjectiveAdvisoryMode = objectiveadvisory.Mode("enabled")
	if err := validateWorkerOptions(opts); err == nil || !strings.Contains(err.Error(), "objective advisory mode") {
		t.Fatalf("invalid advisory mode error=%v", err)
	}
}

func TestWorkerObjectiveAdvisoryEnabledModeRequiresExactProvider(t *testing.T) {
	opts := validWorkerOptions()
	opts.ObjectiveAdvisoryMode = objectiveadvisory.ModeShadow

	opts.ObjectiveAdvisoryProvider = ""
	if err := validateWorkerOptions(opts); err == nil ||
		!strings.Contains(err.Error(), "objective advisory provider") {
		t.Fatalf("missing provider error=%v", err)
	}

	opts.ObjectiveAdvisoryProvider = "openai"
	if err := validateWorkerOptions(opts); err == nil ||
		!strings.Contains(err.Error(), "supports only exact provider") {
		t.Fatalf("inexact provider error=%v", err)
	}

	opts.ObjectiveAdvisoryProvider = llm.ExactPreparedProviderBackend
	if err := validateWorkerOptions(opts); err != nil {
		t.Fatalf("exact provider rejected: %v", err)
	}
}

func TestWorkerObjectiveAdvisoryOffModeDoesNotRequireProvider(t *testing.T) {
	opts := validWorkerOptions()
	opts.ObjectiveAdvisoryProvider = ""
	if err := validateWorkerOptions(opts); err != nil {
		t.Fatalf("off mode resolved provider configuration: %v", err)
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
		{name: "inference context", mutate: func(opts *Options) { opts.InferenceContextTokens = 4095 }, message: "inference_context_tokens"},
		{name: "workspace root", mutate: func(opts *Options) { opts.Workspace.Root = "relative" }, message: "workspace.root"},
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

func TestValidateWorkerOptionsRejectsPersonaShapedStation(t *testing.T) {
	opts := validWorkerOptions()
	opts.Models.Stations = map[station.ID]string{"planner_specialist": "forbidden"}

	err := validateWorkerOptions(opts)
	if err == nil || !strings.Contains(err.Error(), "unregistered semantic station") {
		t.Fatalf("validateWorkerOptions() error=%v, want station registry failure", err)
	}
}

func TestValidateWorkerOptionsAllowsMissingUnreachedStation(t *testing.T) {
	opts := validWorkerOptions()
	delete(opts.Models.Stations, station.GroundedAnswer)

	if err := validateWorkerOptions(opts); err != nil {
		t.Fatalf("unreached station was eagerly required: %v", err)
	}
}
