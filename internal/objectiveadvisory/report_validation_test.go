package objectiveadvisory

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type constantReportValidationEmbedder struct{}

func (constantReportValidationEmbedder) Embedding(context.Context, string) ([]float64, error) {
	return []float64{1, 0}, nil
}

func TestReportValidationRejectsForgedProvenanceGraphAndGapReplay(t *testing.T) {
	const advice = "Check the exact callback declaration before accepting the candidate."
	source := advisorySource("report-source", "qwen3.5:9b-q4_K_M")
	runtime := newAdvisoryRuntime(t, ModeActive, []SourceConfig{source}, &scriptedProvider{
		results: map[string]Generation{source.ID: advisoryGeneration(advice, source.Model)},
	}, constantReportValidationEmbedder{})
	input, gap, config := advisoryInput(), advisoryGap(t), runtime.Configuration()
	report, err := runtime.Run(context.Background(), input, gap)
	if err != nil {
		t.Fatal(err)
	}
	if err := report.ValidateFor(input, gap, config); err != nil {
		t.Fatalf("validate owned report: %v", err)
	}

	replayedGap := gap
	replayedGap.Candidate = "A different candidate answer under the same objective and generation."
	if err := report.ValidateFor(input, replayedGap, config); err == nil {
		t.Fatal("same-objective report replayed against a different semantic gap")
	}

	tests := map[string]func(*Report){
		"mode":               func(value *Report) { value.Mode = ModeShadow },
		"gap identity":       func(value *Report) { value.SemanticGapSHA256 = digest("forged-gap") },
		"projection input":   mutateReportProjectionInput(t),
		"artifact authority": func(value *Report) { value.Artifacts[0].Authority = "fact" },
		"artifact status":    func(value *Report) { value.Artifacts[0].Status = StatusInvalid },
		"artifact raw bytes": func(value *Report) { value.Artifacts[0].RawBytes++ },
		"artifact raw hash":  func(value *Report) { value.Artifacts[0].RawTextSHA256 = digest("other") },
		"artifact ID":        func(value *Report) { value.Artifacts[0].ID = digest("other") },
		"artifact trigger":   func(value *Report) { value.Artifacts[0].TriggerVersion = "v2" },
		"artifact source":    func(value *Report) { value.Artifacts[0].SourceID = "other" },
		"chunk ID":           func(value *Report) { value.Chunks[0].ID = digest("other") },
		"chunk span":         func(value *Report) { value.Chunks[0].StartByte++ },
		"chunk hash":         func(value *Report) { value.Chunks[0].ContentSHA256 = digest("other") },
		"chunk minification": func(value *Report) { value.Chunks[0].Content += " altered" },
		"chunk tags":         func(value *Report) { value.Chunks[0].Tags[0] = "source:forged" },
		"candidate ID":       func(value *Report) { value.CandidateCapsules[0].ID = digest("other") },
		"candidate content":  func(value *Report) { value.CandidateCapsules[0].Content += " altered" },
		"candidate provider": func(value *Report) { value.CandidateCapsules[0].Provider = "other" },
		"candidate model":    func(value *Report) { value.CandidateCapsules[0].EffectiveModel = "other" },
		"candidate gap":      func(value *Report) { value.CandidateCapsules[0].SemanticGapSHA256 = digest("other") },
		"candidate source":   func(value *Report) { value.CandidateCapsules[0].SourceChunkID = digest("other") },
		"active selection":   func(value *Report) { value.ActiveCapsules = []Capsule{} },
		"failed reduction with active context": func(value *Report) {
			value.ReductionStatus = StatusFailed
			value.ReductionError = "embedding failed"
		},
		"metrics": func(value *Report) { value.Metrics.RawBytes++ },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			forged := cloneAdvisoryReport(t, report)
			mutate(&forged)
			if err := forged.ValidateFor(input, gap, config); err == nil {
				t.Fatalf("forged report was accepted: %#v", forged)
			}
		})
	}
}

func mutateReportProjectionInput(t *testing.T) func(*Report) {
	t.Helper()
	return func(value *Report) {
		input := value.Projection.Input
		input.Objective = strings.ReplaceAll(input.Objective, "answer", "response")
		projection, err := BuildProjection(input)
		if err != nil {
			t.Fatal(err)
		}
		value.Projection = projection
	}
}

func cloneAdvisoryReport(t *testing.T, report Report) Report {
	t.Helper()
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	var clone Report
	if err := json.Unmarshal(raw, &clone); err != nil {
		t.Fatal(err)
	}
	return clone
}
