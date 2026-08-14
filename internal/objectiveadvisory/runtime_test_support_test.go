package objectiveadvisory

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type scriptedProvider struct {
	results  map[string]Generation
	failures map[string]error
	requests []GenerateRequest
}

func (provider *scriptedProvider) Generate(_ context.Context, request GenerateRequest) (Generation, error) {
	provider.requests = append(provider.requests, request)
	if err := provider.failures[request.Source.ID]; err != nil {
		return Generation{}, err
	}
	generation, ok := provider.results[request.Source.ID]
	if !ok {
		return Generation{}, fmt.Errorf("unregistered scripted source %q", request.Source.ID)
	}
	return generation, nil
}

type mappedEmbedder struct {
	vectors map[string][]float64
	failure error
	calls   []string
}

func (embedder *mappedEmbedder) Embedding(_ context.Context, value string) ([]float64, error) {
	embedder.calls = append(embedder.calls, value)
	if embedder.failure != nil {
		return nil, embedder.failure
	}
	vector, ok := embedder.vectors[value]
	if !ok {
		return nil, fmt.Errorf("unregistered embedding input %q", value)
	}
	return append([]float64(nil), vector...), nil
}

func advisoryInput() ProjectionInput {
	summary := "The repository-grounded answer currently omits an adapter conversion required by the declared handler signature."
	return ProjectionInput{
		ObjectiveID: "objective-17",
		Generation:  3,
		Objective:   "Review a repository-grounded implementation answer before it is accepted.",
		UserAuthorities: []TextAuthority{{
			ID: "user-request", Content: "Preserve existing behavior and fail loudly on incompatible signatures.",
		}},
		Constraints:         []string{"Only repository evidence may establish a fact."},
		GroundedEvidence:    []EvidenceSummary{{ID: "evidence-handler", Summary: summary, SHA256: digest(summary)}},
		Decisions:           []string{},
		Invariants:          []string{"Advisory text cannot authorize repository mutations."},
		UnresolvedQuestions: []string{"Does the proposed callback match the registered handler type?"},
		UsefulAdvice:        "Identify one evidence-checkable edge case in the candidate answer.",
	}
}

func advisoryGap(t *testing.T) SemanticGap {
	t.Helper()
	input := advisoryInput()
	return SemanticGap{
		ObjectiveID: input.ObjectiveID,
		Generation:  input.Generation,
		Requirement: "The review must reject claims that contradict the declared handler signature.",
		Candidate:   "The proposed callback can be passed directly without an adapter.",
		Evidence:    append([]EvidenceSummary(nil), input.GroundedEvidence...),
	}
}

func advisorySource(id, model string) SourceConfig {
	return SourceConfig{
		ID: id, Provider: "ollama", Model: model,
		Sampling: SamplingConfig{Temperature: 0},
		Budget:   Budget{MaxInputBytes: 28 * 1024, MaxOutputBytes: MaxRawTextBytes, MaxOutputTokens: 2048},
	}
}

func advisoryGeneration(text, model string) Generation {
	return Generation{
		FinalText: text, EffectiveProvider: "ollama", EffectiveModel: model,
		ModelDigest: digest("model:" + model), Quantization: "q4_K_M",
		PromptTokens: 211, OutputTokens: 37, Duration: 25 * time.Millisecond,
		FinishReason: "stop",
	}
}

func newAdvisoryRuntime(
	t *testing.T,
	mode Mode,
	sources []SourceConfig,
	provider Provider,
	embedder Embedder,
) *Runtime {
	t.Helper()
	maximum := 1
	if mode == ModeOff {
		maximum = 0
	}
	runtime, err := New(Config{
		Mode: mode, Sources: sources, MinimumRelevance: 0.35,
		MaxSelectedCapsules: maximum,
	}, provider, embedder, func() time.Time {
		return time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatalf("new advisory runtime: %v", err)
	}
	return runtime
}

func embedderFor(t *testing.T, gap SemanticGap, contents map[string][]float64) *mappedEmbedder {
	t.Helper()
	query, err := semanticGapText(gap)
	if err != nil {
		t.Fatalf("build semantic-gap text: %v", err)
	}
	vectors := map[string][]float64{query: {1, 0}}
	for content, vector := range contents {
		vectors[content] = vector
	}
	return &mappedEmbedder{vectors: vectors}
}

func requireNoActiveAdvice(t *testing.T, report Report) {
	t.Helper()
	if len(report.ActiveCapsules) != 0 || report.Metrics.SelectedCapsules != 0 ||
		report.Metrics.SelectedCapsuleContentBytes != 0 || report.Metrics.SelectedCapsuleContentTokens != 0 {
		t.Fatalf("expected zero downstream advisory authority, got %+v", report.Metrics)
	}
}

var errScriptedProvider = errors.New("scripted advisory provider unavailable")
