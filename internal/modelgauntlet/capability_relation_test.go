package modelgauntlet

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestCapabilityRelationGauntletUsesPhasedBoundedPrompts(t *testing.T) {
	if maxDeliberationTokens != 1024 {
		t.Fatalf("deliberation budget=%d want quality-first 1024-token bound", maxDeliberationTokens)
	}
	cases := []CapabilityRelationCase{
		capabilityCase("inventory-total", "inventory screen", "show the total for current items", "add and remove current items"),
		capabilityCase("account-history", "account history", "choose the active account", "show history for the active account"),
	}
	generator := &scriptedGenerator{generate: successfulGauntletResponse}
	report, err := RunCapabilityRelation(context.Background(), CapabilityRelationConfig{
		StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384,
		KeepAlive: "5m",
	}, cases, generator)
	if err != nil {
		t.Fatal(err)
	}

	wantStages := []CallStage{
		StageDirect, StageDirect,
		StageBriefing, StageBriefing,
		StageDeliberation, StageDeliberation,
		StageSynthesis, StageSynthesis,
	}
	if len(generator.requests) != len(wantStages) {
		t.Fatalf("requests=%d want %d", len(generator.requests), len(wantStages))
	}
	for index, want := range wantStages {
		request := generator.requests[index]
		if request.Stage != want {
			t.Fatalf("request[%d] stage=%q want %q", index, request.Stage, want)
		}
		if request.ContextTokens != 16384 || request.KeepAlive != "5m" {
			t.Fatalf("request[%d] bounds=%#v", index, request)
		}
		if strings.Contains(request.SystemPrompt, "EXPECTED_RELATION") || strings.Contains(request.UserPrompt, "EXPECTED_RELATION") {
			t.Fatalf("request[%d] leaked evaluation authority", index)
		}
		if want == StageDeliberation {
			if !request.Think || len(request.ResponseSchema) != 0 || request.MaxOutputTokens != maxDeliberationTokens {
				t.Fatalf("deliberation request=%#v", request)
			}
			continue
		}
		if request.Think || len(request.ResponseSchema) == 0 {
			t.Fatalf("structured request=%#v", request)
		}
		if !strings.Contains(request.SystemPrompt, "RESPONSE_SCHEMA_JSON:") {
			t.Fatalf("structured request omitted its model-visible response contract:\n%s", request.SystemPrompt)
		}
	}

	if report.Schema != CapabilityRelationReportSchemaV1 || len(report.Calls) != len(wantStages) {
		t.Fatalf("report identity=%#v calls=%d", report, len(report.Calls))
	}
	if report.Config.PromptRenderer != CapabilityRelationPromptRendererV6 {
		t.Fatalf("prompt renderer=%q", report.Config.PromptRenderer)
	}
	if len(report.Predictions) != len(cases)*2 {
		t.Fatalf("predictions=%d want %d", len(report.Predictions), len(cases)*2)
	}
	for _, call := range report.Calls {
		if call.PromptSHA256 == "" || call.Request.SystemPrompt == "" || call.Response.Content == "" {
			t.Fatalf("incomplete exact call evidence=%#v", call)
		}
	}
	for _, call := range report.Calls {
		if call.Request.Stage != StageSynthesis {
			continue
		}
		if !strings.Contains(call.Request.SystemPrompt, "UNTRUSTED_DELIBERATION_JSON") ||
			!strings.Contains(call.Request.SystemPrompt, cases[0].Input.LeftNeed) &&
				!strings.Contains(call.Request.SystemPrompt, cases[1].Input.LeftNeed) {
			t.Fatalf("synthesis prompt lost authority or memo boundary:\n%s", call.Request.SystemPrompt)
		}
	}
}

func TestCapabilityRelationGauntletNeverFallsBackAfterDeliberationFailure(t *testing.T) {
	cases := []CapabilityRelationCase{
		capabilityCase("broken", "terminal session", "show the most recent exit status", "execute the entered command"),
		capabilityCase("healthy", "service control", "choose the active environment", "restart in the active environment"),
	}
	generator := &scriptedGenerator{generate: func(request GenerateRequest) (GenerateResponse, error) {
		response, err := successfulGauntletResponse(request)
		if request.CaseID == "broken" && request.Stage == StageDeliberation {
			return GenerateResponse{}, nil
		}
		return response, err
	}}
	report, err := RunCapabilityRelation(context.Background(), CapabilityRelationConfig{
		StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384,
		KeepAlive: "5m",
	}, cases, generator)
	if err != nil {
		t.Fatal(err)
	}

	prediction := findPrediction(t, report.Predictions, "broken", VariantDeliberated)
	if prediction.Valid || !strings.Contains(prediction.Error, "final memo") {
		t.Fatalf("broken deliberation prediction=%#v", prediction)
	}
	for _, request := range generator.requests {
		if request.CaseID == "broken" && request.Stage == StageSynthesis {
			t.Fatal("failed deliberation silently fell back to synthesis")
		}
	}
	if !findPrediction(t, report.Predictions, "broken", VariantDirect).Valid {
		t.Fatal("independent direct candidate should remain measurable")
	}
	if !findPrediction(t, report.Predictions, "healthy", VariantDeliberated).Valid {
		t.Fatal("one failed case prevented an unrelated case from completing")
	}
}

func TestCapabilityRelationEvaluationLoadsIndependentCompleteLabels(t *testing.T) {
	cases := []CapabilityRelationCase{
		capabilityCase("left", "cart", "show the current total", "change current cart items"),
		capabilityCase("right", "history", "choose an account", "show the chosen account history"),
	}
	report := CapabilityRelationReport{
		Schema: CapabilityRelationReportSchemaV1,
		Cases:  cases,
		Predictions: []CapabilityRelationPrediction{
			{CaseID: "left", Variant: VariantDirect, Valid: true, Relation: assemblyline.CapabilityIndependent},
			{CaseID: "left", Variant: VariantDeliberated, Valid: true, Relation: assemblyline.CapabilityLeftReadsRight},
			{CaseID: "right", Variant: VariantDirect, Valid: false, Error: "invalid JSON"},
			{CaseID: "right", Variant: VariantDeliberated, Valid: true, Relation: assemblyline.CapabilityRightReadsLeft},
		},
	}
	evaluation, err := EvaluateCapabilityRelation(report, []CapabilityRelationLabel{
		{CaseID: "left", Relation: assemblyline.CapabilityLeftReadsRight},
		{CaseID: "right", Relation: assemblyline.CapabilityRightReadsLeft},
	})
	if err != nil {
		t.Fatal(err)
	}
	if evaluation.Scores[VariantDirect].Correct != 0 || evaluation.Scores[VariantDirect].Valid != 1 {
		t.Fatalf("direct score=%#v", evaluation.Scores[VariantDirect])
	}
	if evaluation.Scores[VariantDeliberated].Correct != 2 || evaluation.Scores[VariantDeliberated].Valid != 2 {
		t.Fatalf("deliberated score=%#v", evaluation.Scores[VariantDeliberated])
	}

	_, err = EvaluateCapabilityRelation(report, []CapabilityRelationLabel{{
		CaseID: "left", Relation: assemblyline.CapabilityLeftReadsRight,
	}})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("incomplete labels error=%v", err)
	}
}

func TestCapabilityRelationEvaluationAggregatesVariantCost(t *testing.T) {
	fixture := capabilityCase("one", "cart", "show total", "change items")
	report := CapabilityRelationReport{
		Schema: CapabilityRelationReportSchemaV1,
		Cases:  []CapabilityRelationCase{fixture},
		Predictions: []CapabilityRelationPrediction{
			{CaseID: "one", Variant: VariantDirect, Valid: true, Relation: assemblyline.CapabilityLeftReadsRight},
			{CaseID: "one", Variant: VariantDeliberated, Valid: true, Relation: assemblyline.CapabilityLeftReadsRight},
		},
		Calls: []CallEvidence{
			{Request: GenerateRequest{CaseID: "one", Variant: VariantDirect, Stage: StageDirect}, Response: GenerateResponse{TotalDuration: 10, EvalCount: 1, AllocatedBytes: 100, VRAMBytes: 80}},
			{Request: GenerateRequest{CaseID: "one", Variant: VariantDeliberated, Stage: StageBriefing}, Response: GenerateResponse{TotalDuration: 20, EvalCount: 2, AllocatedBytes: 90, VRAMBytes: 70}},
			{Request: GenerateRequest{CaseID: "one", Variant: VariantDeliberated, Stage: StageDeliberation}, Response: GenerateResponse{TotalDuration: 30, EvalCount: 3, AllocatedBytes: 120, VRAMBytes: 60}},
			{Request: GenerateRequest{CaseID: "one", Variant: VariantDeliberated, Stage: StageSynthesis}, Response: GenerateResponse{TotalDuration: 40, EvalCount: 4, AllocatedBytes: 100, VRAMBytes: 80}},
		},
	}
	evaluation, err := EvaluateCapabilityRelation(report, []CapabilityRelationLabel{{
		CaseID: "one", Relation: assemblyline.CapabilityLeftReadsRight,
	}})
	if err != nil {
		t.Fatal(err)
	}
	direct := evaluation.Metrics[VariantDirect]
	deliberated := evaluation.Metrics[VariantDeliberated]
	if direct.Calls != 1 || direct.TotalDuration != 10 || direct.EvalTokens != 1 {
		t.Fatalf("direct metrics=%#v", direct)
	}
	if deliberated.Calls != 3 || deliberated.TotalDuration != 90 || deliberated.EvalTokens != 9 {
		t.Fatalf("deliberated metrics=%#v", deliberated)
	}
	if deliberated.MaxAllocatedBytes != 120 || deliberated.MaxVRAMBytes != 80 {
		t.Fatalf("deliberated resource metrics=%#v", deliberated)
	}
}

func capabilityCase(id, localContext, left, right string) CapabilityRelationCase {
	return CapabilityRelationCase{ID: id, Input: assemblyline.CapabilityRelationInput{
		LocalContext: localContext, LeftNeed: left, RightNeed: right,
	}}
}

type scriptedGenerator struct {
	requests []GenerateRequest
	generate func(GenerateRequest) (GenerateResponse, error)
}

func (generator *scriptedGenerator) Generate(_ context.Context, request GenerateRequest) (GenerateResponse, error) {
	generator.requests = append(generator.requests, request)
	return generator.generate(request)
}

func successfulGauntletResponse(request GenerateRequest) (GenerateResponse, error) {
	switch request.Stage {
	case StageBriefing:
		return GenerateResponse{Content: `{"schema":"omnidex.deliberation-lens.v1","lens":"state_flow"}`}, nil
	case StageDeliberation:
		return GenerateResponse{Thinking: "Check which behavior produces current state.", Content: "The display reads mutation state."}, nil
	case StageDirect, StageSynthesis:
		relation := assemblyline.CapabilityLeftReadsRight
		if request.CaseID == "account-history" || request.CaseID == "healthy" {
			relation = assemblyline.CapabilityRightReadsLeft
		}
		raw, _ := json.Marshal(assemblyline.CapabilityRelationDecision{
			Schema: assemblyline.CapabilityRelationSchemaV1, Relation: relation,
		})
		return GenerateResponse{Content: string(raw)}, nil
	default:
		return GenerateResponse{}, nil
	}
}

func findPrediction(
	t *testing.T,
	predictions []CapabilityRelationPrediction,
	caseID string,
	variant Variant,
) CapabilityRelationPrediction {
	t.Helper()
	for _, prediction := range predictions {
		if prediction.CaseID == caseID && prediction.Variant == variant {
			return prediction
		}
	}
	t.Fatalf("missing prediction case=%s variant=%s", caseID, variant)
	return CapabilityRelationPrediction{}
}
