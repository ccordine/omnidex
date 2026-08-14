package objectiveadvisory

import (
	"context"
	"strings"
	"testing"
)

func TestOffModeIsInertAndMakesNoCalls(t *testing.T) {
	runtime := newAdvisoryRuntime(t, ModeOff, nil, nil, nil)
	report, err := runtime.Run(context.Background(), advisoryInput(), advisoryGap(t))
	if err != nil {
		t.Fatalf("run off advisory: %v", err)
	}
	if report.Mode != ModeOff || report.ReductionStatus != StatusNotRun ||
		len(report.Artifacts) != 0 || len(report.Chunks) != 0 || len(report.CandidateCapsules) != 0 ||
		report.Metrics.AdvisoryCalls != 0 || report.Metrics.EmbeddingCalls != 0 {
		t.Fatalf("off mode was not inert: %+v", report)
	}
	requireNoActiveAdvice(t, report)
}

func TestShadowModeMeasuresRelevantAdviceWithoutDownstreamContext(t *testing.T) {
	const advice = "The declared callback type may require an explicit HandlerFunc adapter before assignment."
	source := advisorySource("analytical", "qwen3.5:9b-q4_K_M")
	provider := &scriptedProvider{results: map[string]Generation{
		source.ID: advisoryGeneration(advice, source.Model),
	}}
	gap := advisoryGap(t)
	embedder := embedderFor(t, gap, map[string][]float64{advice: {0.95, 0.05}})
	runtime := newAdvisoryRuntime(t, ModeShadow, []SourceConfig{source}, provider, embedder)
	report, err := runtime.Run(context.Background(), advisoryInput(), gap)
	if err != nil {
		t.Fatalf("run shadow advisory: %v", err)
	}
	if len(report.Artifacts) != 1 || report.Artifacts[0].RawText != advice ||
		len(report.CandidateCapsules) != 1 || report.Metrics.PotentialCapsuleContentBytes != len(advice) ||
		report.Metrics.PotentialCapsuleContentTokens != (len(advice)+3)/4 {
		t.Fatalf("shadow measurements are incomplete: %+v", report)
	}
	if report.Metrics.AdvisoryCalls != 1 || report.Metrics.EmbeddingCalls != 2 ||
		report.Metrics.ChunksProduced != 1 || report.Metrics.UnselectedChunks != 1 {
		t.Fatalf("shadow accounting is inconsistent: %+v", report.Metrics)
	}
	requireNoActiveAdvice(t, report)
}

func TestActiveModeSelectsOneBoundedProvenancedCapsule(t *testing.T) {
	const advice = "The declared callback type may require an explicit HandlerFunc adapter before assignment."
	source := advisorySource("analytical", "qwen3.5:9b-q4_K_M")
	provider := &scriptedProvider{results: map[string]Generation{
		source.ID: advisoryGeneration(advice, source.Model),
	}}
	gap := advisoryGap(t)
	runtime := newAdvisoryRuntime(t, ModeActive, []SourceConfig{source}, provider,
		embedderFor(t, gap, map[string][]float64{advice: {1, 0}}))
	report, err := runtime.Run(context.Background(), advisoryInput(), gap)
	if err != nil {
		t.Fatalf("run active advisory: %v", err)
	}
	if len(report.ActiveCapsules) != 1 || report.Metrics.SelectedCapsules != 1 ||
		report.Metrics.SelectedCapsuleContentBytes != len(advice) ||
		report.Metrics.SelectedCapsuleContentTokens != (len(advice)+3)/4 {
		t.Fatalf("expected exactly one active capsule: %+v", report)
	}
	capsule := report.ActiveCapsules[0]
	if err := capsule.ValidateFor(advisoryInput().ObjectiveID, advisoryInput().Generation); err != nil {
		t.Fatalf("validate active capsule: %v", err)
	}
	if capsule.Content != advice || capsule.SourceAdvisoryID != report.Artifacts[0].ID ||
		capsule.SourceChunkID != report.Chunks[0].ID || capsule.RequestedModel != source.Model ||
		!strings.HasPrefix(capsule.RelevanceBasis, "cosine_embedding_v1:") {
		t.Fatalf("active capsule lost provenance: %+v", capsule)
	}
}

func TestIrrelevantAdviceProducesZeroDownstreamBytes(t *testing.T) {
	const advice = "A separate release may benefit from a different color palette."
	source := advisorySource("analytical", "qwen3.5:9b-q4_K_M")
	gap := advisoryGap(t)
	runtime := newAdvisoryRuntime(t, ModeActive, []SourceConfig{source}, &scriptedProvider{
		results: map[string]Generation{source.ID: advisoryGeneration(advice, source.Model)},
	}, embedderFor(t, gap, map[string][]float64{advice: {0, 1}}))
	report, err := runtime.Run(context.Background(), advisoryInput(), gap)
	if err != nil {
		t.Fatalf("run irrelevant advisory: %v", err)
	}
	if len(report.CandidateCapsules) != 0 || report.ReductionStatus != StatusSucceeded {
		t.Fatalf("irrelevant advice was retained: %+v", report)
	}
	requireNoActiveAdvice(t, report)
}

func TestConfiguredAdvisorsRemainIndependentWithoutVoting(t *testing.T) {
	const first = "The callback requires an adapter to satisfy the declared type."
	const second = "The callback should remain direct and needs no adapter."
	sources := []SourceConfig{
		advisorySource("model-a", "qwen3.5:9b-q4_K_M"),
		advisorySource("model-b", "qwen3.5:27b-q4_K_M"),
	}
	provider := &scriptedProvider{results: map[string]Generation{
		sources[0].ID: advisoryGeneration(first, sources[0].Model),
		sources[1].ID: advisoryGeneration(second, sources[1].Model),
	}}
	gap := advisoryGap(t)
	runtime := newAdvisoryRuntime(t, ModeActive, sources, provider, embedderFor(t, gap,
		map[string][]float64{first: {1, 0}, second: {0.9, 0.1}}))
	report, err := runtime.Run(context.Background(), advisoryInput(), gap)
	if err != nil {
		t.Fatalf("run conflicting advisors: %v", err)
	}
	if len(provider.requests) != 2 || len(report.Artifacts) != 2 || len(report.CandidateCapsules) != 2 ||
		len(report.ActiveCapsules) != 1 {
		t.Fatalf("configured advisors were combined, dropped, or voted: %+v", report)
	}
	if report.Artifacts[0].RequestedModel == report.Artifacts[1].RequestedModel {
		t.Fatalf("model-independent configurations collapsed: %+v", report.Artifacts)
	}
	if report.Artifacts[0].Authority != AuthorityNonAuthoritative ||
		report.Artifacts[1].Authority != AuthorityNonAuthoritative {
		t.Fatalf("conflicting advisors acquired authority: %+v", report.Artifacts)
	}
}
