package modelgauntlet

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestRunCompleteRequirementPartitionComparesAllThreeVariants(t *testing.T) {
	t.Parallel()

	cases := []CompleteRequirementCase{{
		ID: "timer", SourceText: "Build a timer with lap history.",
	}}
	recorder := &completeRequirementRecordingGenerator{}
	report, err := RunCompleteRequirementPartition(context.Background(), CompleteRequirementConfig{
		StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384,
		KeepAlive: "1m", Repetitions: 2, CasesSHA256: strings.Repeat("a", 64),
		HardwareClass: "test-machine", Backend: "test-backend",
	}, cases, recorder)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Predictions) != 6 {
		t.Fatalf("predictions=%d", len(report.Predictions))
	}
	for _, prediction := range report.Predictions {
		if !prediction.Valid || len(prediction.FeatureQuotes) != 1 || prediction.FeatureQuotes[0] != "lap history" {
			t.Fatalf("prediction=%#v", prediction)
		}
	}
	if len(report.Calls) != 24 {
		t.Fatalf("calls=%d, want 24", len(report.Calls))
	}
	for _, call := range report.Calls {
		if call.Request.Repetition < 1 || strings.TrimSpace(call.Request.Operation) == "" {
			t.Fatalf("call identity=%#v", call.Request)
		}
	}
	for _, request := range recorder.Requests {
		if request.Operation == "final.advisory" && request.MaxOutputTokens != maxFinalRequirementDeliberationTokens {
			t.Fatalf("final advisory budget=%d", request.MaxOutputTokens)
		}
		if strings.Contains(request.Operation, ".advisory") && request.Operation != "final.advisory" && request.MaxOutputTokens != maxDeliberationTokens {
			t.Fatalf("per-split advisory budget=%d", request.MaxOutputTokens)
		}
	}
}

func TestEvaluateCompleteRequirementPromotesOnlyFinalPassWithFrozenEvidence(t *testing.T) {
	t.Parallel()

	report, labels := promotionReadyCompleteRequirementReport(t)
	evaluation, err := EvaluateCompleteRequirementPartition(report, labels)
	if err != nil {
		t.Fatal(err)
	}
	if !evaluation.Promotion.Eligible || len(evaluation.Promotion.Reasons) != 0 {
		t.Fatalf("promotion=%#v", evaluation.Promotion)
	}
	transition := evaluation.Transitions[VariantFinalPassAdvisory]
	if transition.DirectPassAssistedFail != 0 || transition.DirectFailAssistedPass != 2 {
		t.Fatalf("final transitions=%#v", transition)
	}
	if evaluation.Stability[VariantFinalPassAdvisory].Stable != minimumCompleteRequirementCases {
		t.Fatalf("stability=%#v", evaluation.Stability)
	}

	broken := report
	broken.Predictions = append([]CompleteRequirementPrediction(nil), report.Predictions...)
	for index := range broken.Predictions {
		prediction := &broken.Predictions[index]
		if prediction.CaseID == "case-002" && prediction.Repetition == 1 && prediction.Variant == VariantFinalPassAdvisory {
			prediction.FeatureQuotes = []string{"feature alpha 002"}
			break
		}
	}
	evaluation, err = EvaluateCompleteRequirementPartition(broken, labels)
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Promotion.Eligible || !containsReason(evaluation.Promotion.Reasons, "regression") {
		t.Fatalf("regression promotion=%#v", evaluation.Promotion)
	}
}

func TestFinalAdvisoryBudgetIsBoundedAcrossUnrelatedFixtures(t *testing.T) {
	t.Parallel()

	recorder := &completeRequirementRecordingGenerator{}
	_, err := RunCompleteRequirementPartition(context.Background(), CompleteRequirementConfig{
		StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384,
		KeepAlive: "1m", Repetitions: 1, CasesSHA256: strings.Repeat("b", 64),
		HardwareClass: "test-machine", Backend: "test-backend",
	}, []CompleteRequirementCase{
		{ID: "timer", SourceText: "Build a timer with lap history."},
		{ID: "board", SourceText: "Create a board with card search."},
	}, recorder)
	if err != nil {
		t.Fatal(err)
	}
	finalCalls := 0
	for _, request := range recorder.Requests {
		if request.Operation != "final.advisory" {
			continue
		}
		finalCalls++
		if request.MaxOutputTokens != maxFinalRequirementDeliberationTokens {
			t.Fatalf("case=%s final budget=%d", request.CaseID, request.MaxOutputTokens)
		}
	}
	if finalCalls != 2 {
		t.Fatalf("final advisory calls=%d", finalCalls)
	}
}

type completeRequirementTestGenerator struct{}

func (completeRequirementTestGenerator) Generate(_ context.Context, request GenerateRequest) (GenerateResponse, error) {
	response := GenerateResponse{
		Model: request.Model, ModelDigest: strings.Repeat("d", 64), Quantization: "Q4_K_M",
		ParameterSize: "test", TotalDuration: 1, EvalCount: 1,
	}
	switch request.Stage {
	case StageBriefing:
		response.Content = `{"schema":"omnidex.requirement-partition-briefing.v1","lens":"coverage"}`
	case StageDeliberation:
		response.Thinking = "private"
		response.Content = "The direct candidate covers the explicit feature."
	case StageDirect, StageSynthesis:
		quote := "lap history"
		if request.CaseID == "board" {
			quote = "card search"
		}
		response.Content = fmt.Sprintf(`{"schema":"omnidex.requirement-partition.v1","feature_quotes":[%q]}`, quote)
	default:
		return GenerateResponse{}, fmt.Errorf("unexpected stage %q", request.Stage)
	}
	return response, nil
}

type completeRequirementRecordingGenerator struct {
	Requests []GenerateRequest
}

func (generator *completeRequirementRecordingGenerator) Generate(
	ctx context.Context,
	request GenerateRequest,
) (GenerateResponse, error) {
	generator.Requests = append(generator.Requests, request)
	return (completeRequirementTestGenerator{}).Generate(ctx, request)
}

func promotionReadyCompleteRequirementReport(t *testing.T) (CompleteRequirementReport, []CompleteRequirementLabel) {
	t.Helper()
	report := CompleteRequirementReport{
		Schema: CompleteRequirementReportSchemaV1,
		Config: CompleteRequirementConfigEvidence{
			StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384,
			KeepAlive: "1m", Repetitions: minimumCompleteRequirementRepeats,
			CasesSHA256: strings.Repeat("a", 64), HardwareClass: "test-machine",
			Backend: "test-backend", PromptRenderer: CompleteRequirementRendererV3,
			StructuredMaxOutputTokens:       maxStructuredTokens,
			PerSplitAdvisoryMaxOutputTokens: maxDeliberationTokens,
			FinalAdvisoryMaxOutputTokens:    maxFinalRequirementDeliberationTokens,
		},
	}
	labels := make([]CompleteRequirementLabel, 0, minimumCompleteRequirementCases)
	for caseNumber := 1; caseNumber <= minimumCompleteRequirementCases; caseNumber++ {
		id := fmt.Sprintf("case-%03d", caseNumber)
		alpha := fmt.Sprintf("feature alpha %03d", caseNumber)
		beta := fmt.Sprintf("feature beta %03d", caseNumber)
		source := fmt.Sprintf("Build tool %03d with %s and %s.", caseNumber, alpha, beta)
		report.Cases = append(report.Cases, CompleteRequirementCase{ID: id, SourceText: source})
		labels = append(labels, CompleteRequirementLabel{CaseID: id, FeatureQuotes: []string{alpha, beta}})
		for repetition := 1; repetition <= minimumCompleteRequirementRepeats; repetition++ {
			directQuotes := []string{alpha, beta}
			if caseNumber == 1 {
				directQuotes = []string{alpha}
			}
			for _, variant := range completeRequirementVariants() {
				quotes := directQuotes
				if variant == VariantFinalPassAdvisory {
					quotes = []string{alpha, beta}
				}
				report.Predictions = append(report.Predictions, CompleteRequirementPrediction{
					CaseID: id, Repetition: repetition, Variant: variant, Valid: true,
					FeatureQuotes: append([]string(nil), quotes...),
				})
			}
		}
	}
	report.Calls = []CallEvidence{
		completeIdentityCall("stable", VariantDirect, StageDirect),
		completeIdentityCall("reasoner", VariantFinalPassAdvisory, StageDeliberation),
	}
	return report, labels
}

func completeIdentityCall(model string, variant Variant, stage CallStage) CallEvidence {
	return CallEvidence{
		Request:  GenerateRequest{CaseID: "case-001", Repetition: 1, Operation: "identity", Variant: variant, Stage: stage, Model: model},
		Response: GenerateResponse{Model: model, ModelDigest: strings.Repeat("d", 64), Quantization: "Q4_K_M"},
	}
}

func containsReason(reasons []string, target string) bool {
	raw, _ := json.Marshal(reasons)
	return strings.Contains(string(raw), target)
}
