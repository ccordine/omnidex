package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelgauntlet"
)

func TestExecuteCapabilityRelationGauntletLoadsLabelsOnlyAfterInference(t *testing.T) {
	directory := t.TempDir()
	casesPath := filepath.Join(directory, "cases.json")
	labelsPath := filepath.Join(directory, "labels.json")
	outputPath := filepath.Join(directory, "result.json")
	writeGauntletTestFile(t, casesPath, `{
		"schema":"omnidex.model-gauntlet.capability-relation-cases.v1",
		"cases":[{"id":"one","input":{"local_context":"cart","left_need":"show total","right_need":"change items"}}]
	}`)
	writeGauntletTestFile(t, labelsPath, `{"schema":"not-yet-valid"}`)

	generator := &gauntletCLIStub{onCall: func(call int, request modelgauntlet.GenerateRequest) modelgauntlet.GenerateResponse {
		if call == 4 {
			writeGauntletTestFile(t, labelsPath, `{
				"schema":"omnidex.model-gauntlet.capability-relation-labels.v1",
				"labels":[{"case_id":"one","relation":"left_reads_right"}]
			}`)
		}
		switch request.Stage {
		case modelgauntlet.StageBriefing:
			return modelgauntlet.GenerateResponse{Content: `{"schema":"omnidex.deliberation-lens.v1","lens":"state_flow"}`}
		case modelgauntlet.StageDeliberation:
			return modelgauntlet.GenerateResponse{Thinking: "trace state", Content: "the total reads cart state"}
		default:
			raw, _ := json.Marshal(assemblyline.CapabilityRelationDecision{
				Schema: assemblyline.CapabilityRelationSchemaV1, Relation: assemblyline.CapabilityLeftReadsRight,
			})
			return modelgauntlet.GenerateResponse{Content: string(raw)}
		}
	}}
	result, err := executeCapabilityRelationGauntlet(context.Background(), modelGauntletOptions{
		CasesPath: casesPath, LabelsPath: labelsPath, OutputPath: outputPath,
		StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384, KeepAlive: "5m",
	}, generator)
	if err != nil {
		t.Fatal(err)
	}
	if generator.calls != 4 {
		t.Fatalf("calls=%d want 4", generator.calls)
	}
	if result.Evaluation.Scores[modelgauntlet.VariantDeliberated].Correct != 1 || result.LabelSHA256 == "" {
		t.Fatalf("result=%#v", result)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("result evidence missing: %v", err)
	}
}

func TestExecuteRequirementPartitionGauntletLoadsLabelsOnlyAfterInference(t *testing.T) {
	directory := t.TempDir()
	casesPath := filepath.Join(directory, "cases.json")
	labelsPath := filepath.Join(directory, "labels.json")
	outputPath := filepath.Join(directory, "result.json")
	writeGauntletTestFile(t, casesPath, `{
		"schema":"omnidex.model-gauntlet.requirement-partition-cases.v1",
		"cases":[{"id":"one","input":{"source_text":"lap history","mode":"split_feature"}}]
	}`)
	writeGauntletTestFile(t, labelsPath, `{"schema":"not-yet-valid"}`)

	generator := &gauntletCLIStub{onCall: func(call int, request modelgauntlet.GenerateRequest) modelgauntlet.GenerateResponse {
		if call == 4 {
			writeGauntletTestFile(t, labelsPath, `{
				"schema":"omnidex.model-gauntlet.requirement-partition-labels.v1",
				"labels":[{"case_id":"one","feature_quotes":["lap history"]}]
			}`)
		}
		switch request.Stage {
		case modelgauntlet.StageBriefing:
			return modelgauntlet.GenerateResponse{Content: `{"schema":"omnidex.requirement-partition-briefing.v1","lens":"atomicity"}`}
		case modelgauntlet.StageDeliberation:
			return modelgauntlet.GenerateResponse{Thinking: "inspect atomicity", Content: "one feature"}
		default:
			raw, _ := json.Marshal(assemblyline.RequirementPartitionDecision{
				Schema: assemblyline.RequirementPartitionSchemaV1, FeatureQuotes: []string{"lap history"},
			})
			return modelgauntlet.GenerateResponse{Content: string(raw)}
		}
	}}
	result, err := executeRequirementPartitionGauntlet(context.Background(), modelGauntletOptions{
		CasesPath: casesPath, LabelsPath: labelsPath, OutputPath: outputPath,
		StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384, KeepAlive: "5m",
	}, generator)
	if err != nil {
		t.Fatal(err)
	}
	if generator.calls != 4 {
		t.Fatalf("calls=%d want 4", generator.calls)
	}
	if result.Evaluation.Scores[modelgauntlet.VariantDeliberated].Correct != 1 || result.LabelSHA256 == "" {
		t.Fatalf("result=%#v", result)
	}
	if _, err := os.Stat(outputPath); err != nil {
		t.Fatalf("result evidence missing: %v", err)
	}
}

func TestExecuteRepositoryRetrievalGauntletLoadsLabelsOnlyAfterInference(t *testing.T) {
	directory := t.TempDir()
	casesPath := filepath.Join(directory, "cases.json")
	labelsPath := filepath.Join(directory, "labels.json")
	outputPath := filepath.Join(directory, "result.json")
	writeGauntletTestFile(t, casesPath, `{
		"schema":"omnidex.model-gauntlet.repository-retrieval-cases.v1",
		"cases":[{"id":"one","input":{"research_need":"Find direct references to ParseEnvelope."}}]
	}`)
	writeGauntletTestFile(t, labelsPath, `{"schema":"not-yet-valid"}`)

	generator := &gauntletCLIStub{onCall: func(call int, request modelgauntlet.GenerateRequest) modelgauntlet.GenerateResponse {
		if call == 4 {
			writeGauntletTestFile(t, labelsPath, `{
				"schema":"omnidex.model-gauntlet.repository-retrieval-labels.v1",
				"labels":[{"case_id":"one","operation":"direct_references","query_quote":"ParseEnvelope"}]
			}`)
		}
		switch request.Stage {
		case modelgauntlet.StageBriefing:
			return modelgauntlet.GenerateResponse{Content: `{"schema":"omnidex.repository-retrieval-briefing.v1","lens":"relation_direction"}`}
		case modelgauntlet.StageDeliberation:
			return modelgauntlet.GenerateResponse{Thinking: "evidence only", Content: "retrieve incoming references"}
		default:
			raw, _ := json.Marshal(assemblyline.RepositoryRetrievalDecision{
				Schema:    assemblyline.RepositoryRetrievalSchemaV1,
				Operation: assemblyline.RetrievalDirectReferences, QueryQuote: "ParseEnvelope",
			})
			return modelgauntlet.GenerateResponse{Content: string(raw)}
		}
	}}
	result, err := executeRepositoryRetrievalGauntlet(context.Background(), modelGauntletOptions{
		CasesPath: casesPath, LabelsPath: labelsPath, OutputPath: outputPath,
		StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384, KeepAlive: "5m",
	}, generator)
	if err != nil {
		t.Fatal(err)
	}
	if generator.calls != 4 || result.Evaluation.Scores[modelgauntlet.VariantDeliberated].Correct != 1 {
		t.Fatalf("calls=%d result=%#v", generator.calls, result)
	}
}

func TestExecuteCompleteRequirementGauntletLoadsLabelsOnlyAfterEveryVariantStops(t *testing.T) {
	directory := t.TempDir()
	casesPath := filepath.Join(directory, "cases.json")
	labelsPath := filepath.Join(directory, "labels.json")
	outputPath := filepath.Join(directory, "result.json")
	writeGauntletTestFile(t, casesPath, `{
		"schema":"omnidex.model-gauntlet.complete-requirement-cases.v1",
		"cases":[{"id":"one","source_text":"Build a timer with lap history."}]
	}`)
	writeGauntletTestFile(t, labelsPath, `{"schema":"not-yet-valid"}`)

	generator := &gauntletCLIStub{onCall: func(call int, request modelgauntlet.GenerateRequest) modelgauntlet.GenerateResponse {
		if call == 12 {
			writeGauntletTestFile(t, labelsPath, `{
				"schema":"omnidex.model-gauntlet.complete-requirement-labels.v1",
				"labels":[{"case_id":"one","feature_quotes":["lap history"]}]
			}`)
		}
		switch request.Stage {
		case modelgauntlet.StageBriefing:
			return modelgauntlet.GenerateResponse{Content: `{"schema":"omnidex.requirement-partition-briefing.v1","lens":"coverage"}`}
		case modelgauntlet.StageDeliberation:
			return modelgauntlet.GenerateResponse{Thinking: "inspect", Content: "the candidate covers the explicit feature"}
		default:
			return modelgauntlet.GenerateResponse{Content: `{"schema":"omnidex.requirement-partition.v1","feature_quotes":["lap history"]}`}
		}
	}}
	result, err := executeCompleteRequirementGauntlet(context.Background(), completeRequirementGauntletOptions{
		CasesPath: casesPath, LabelsPath: labelsPath, OutputPath: outputPath,
		StableModel: "stable", ReasoningModel: "reasoner", ContextTokens: 16384,
		KeepAlive: "5m", Repetitions: 1, HardwareClass: "test", Backend: "test",
	}, generator)
	if err != nil {
		t.Fatal(err)
	}
	if generator.calls != 12 {
		t.Fatalf("calls=%d want 12", generator.calls)
	}
	if result.Evaluation.Scores[modelgauntlet.VariantFinalPassAdvisory].Correct != 1 || result.LabelSHA256 == "" {
		t.Fatalf("result=%#v", result)
	}
	if result.Evaluation.Promotion.Eligible {
		t.Fatal("one-case experiment unexpectedly passed the promotion gate")
	}
}

type gauntletCLIStub struct {
	calls  int
	onCall func(int, modelgauntlet.GenerateRequest) modelgauntlet.GenerateResponse
}

func (stub *gauntletCLIStub) Generate(_ context.Context, request modelgauntlet.GenerateRequest) (modelgauntlet.GenerateResponse, error) {
	stub.calls++
	return stub.onCall(stub.calls, request), nil
}

func writeGauntletTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
