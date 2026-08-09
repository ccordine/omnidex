package modelgauntlet

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRequirementPartitionGauntletUsesRegisteredAdvisoryProtocol(t *testing.T) {
	fixture := RequirementPartitionCase{ID: "features", Input: assemblyline.RequirementPartitionInput{
		SourceText: "Build a timer with lap history and keyboard controls.",
		Mode:       assemblyline.RequirementExtractFeatures,
	}}
	generator := &scriptedGenerator{generate: func(request GenerateRequest) (GenerateResponse, error) {
		switch request.Stage {
		case StageBriefing:
			return GenerateResponse{Content: `{"schema":"omnidex.requirement-partition-briefing.v1","lens":"coverage"}`}, nil
		case StageDeliberation:
			return GenerateResponse{Thinking: "enumerate explicit feature spans", Content: "Two independent features are requested."}, nil
		case StageDirect, StageSynthesis:
			raw, _ := json.Marshal(assemblyline.RequirementPartitionDecision{
				Schema:        assemblyline.RequirementPartitionSchemaV1,
				FeatureQuotes: []string{"lap history", "keyboard controls"},
			})
			return GenerateResponse{Content: string(raw)}, nil
		default:
			return GenerateResponse{}, nil
		}
	}}
	report, err := RunRequirementPartition(context.Background(), RequirementPartitionConfig{
		StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384, KeepAlive: "5m",
	}, []RequirementPartitionCase{fixture}, generator)
	if err != nil {
		t.Fatal(err)
	}

	wantStages := []CallStage{StageDirect, StageBriefing, StageDeliberation, StageSynthesis}
	if len(report.Calls) != len(wantStages) {
		t.Fatalf("calls=%d want %d", len(report.Calls), len(wantStages))
	}
	for index, stage := range wantStages {
		if report.Calls[index].Request.Stage != stage {
			t.Fatalf("call[%d] stage=%q want %q", index, report.Calls[index].Request.Stage, stage)
		}
	}
	if report.Config.PromptRenderer != RequirementPartitionPromptRendererV2 {
		t.Fatalf("renderer=%q", report.Config.PromptRenderer)
	}
	for _, prediction := range report.Predictions {
		if !prediction.Valid || len(prediction.FeatureQuotes) != 2 || prediction.Error != "" {
			t.Fatalf("prediction=%#v", prediction)
		}
	}
	synthesis := report.Calls[len(report.Calls)-1].Request.SystemPrompt
	if !strings.Contains(synthesis, "ORIGINAL_AUTHORITATIVE_PROMPT:") ||
		!strings.Contains(synthesis, "UNTRUSTED_ADVISORY_MEMO_JSON:") {
		t.Fatalf("synthesis omitted advisory boundary:\n%s", synthesis)
	}
	if strings.Contains(synthesis, "enumerate explicit feature spans") {
		t.Fatalf("synthesis leaked native thinking instead of consuming only final memo content:\n%s", synthesis)
	}
}

func TestRequirementPartitionEvaluationRequiresExactOrderedLabels(t *testing.T) {
	fixture := RequirementPartitionCase{ID: "features", Input: assemblyline.RequirementPartitionInput{
		SourceText: "Build a timer with lap history and keyboard controls.",
		Mode:       assemblyline.RequirementExtractFeatures,
	}}
	report := RequirementPartitionReport{
		Schema: RequirementPartitionReportSchemaV1,
		Cases:  []RequirementPartitionCase{fixture},
		Predictions: []RequirementPartitionPrediction{
			{CaseID: "features", Variant: VariantDirect, Valid: true, FeatureQuotes: []string{"lap history"}},
			{CaseID: "features", Variant: VariantDeliberated, Valid: true, FeatureQuotes: []string{"lap history", "keyboard controls"}},
		},
	}
	evaluation, err := EvaluateRequirementPartition(report, []RequirementPartitionLabel{{
		CaseID: "features", FeatureQuotes: []string{"lap history", "keyboard controls"},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Scores[VariantDirect].Correct != 0 || evaluation.Scores[VariantDeliberated].Correct != 1 {
		t.Fatalf("scores=%#v", evaluation.Scores)
	}

	_, err = EvaluateRequirementPartition(report, []RequirementPartitionLabel{{
		CaseID: "features", FeatureQuotes: []string{"keyboard controls", "lap history"},
	}})
	if err == nil || !strings.Contains(err.Error(), "source order") {
		t.Fatalf("reordered label error=%v", err)
	}
}
