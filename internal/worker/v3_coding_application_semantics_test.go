package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
)

func TestApplicationInterpreterUsesOneExtractionThenBlindFeatureJobs(t *testing.T) {
	t.Parallel()

	request := "Build a browser tool where I can filter the catalog and remember my selection."
	responses := []any{
		browserClassification(), applicationIdentity("browser tool"),
		partitionDecision("filter the catalog", "remember my selection"),
		partitionDecision("filter the catalog"), partitionDecision("remember my selection"),
	}
	script := scriptedSemanticRuntime(t, responses)
	specification, err := runDirectCodingApplicationInterpreter(
		script.runtime, "partition-model", "split-model", "surface-model", "identity-model", "artifact-model", request, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if specification.Surface != assemblyline.ApplicationSurfaceBrowser || len(specification.Requirements) != 2 {
		t.Fatalf("specification=%#v", specification)
	}
	if countValue(script.models, "partition-model") != 2 || countValue(script.models, "split-model") != 4 ||
		countValue(script.models, "identity-model") != 1 ||
		countValue(script.models, "artifact-model") != 0 {
		t.Fatalf("unexpected semantic routing: models=%#v", script.models)
	}
	if len(script.advisoryModels) != 3 || countValue(script.advisoryModels, "adviser-model") != 3 {
		t.Fatalf("unexpected advisory routing: models=%#v", script.advisoryModels)
	}
	for index, modelName := range script.models {
		if modelName == "split-model" && strings.Contains(script.prompts[index], request) {
			t.Fatalf("small job %d received the broad request:\n%s", index, script.prompts[index])
		}
	}
	if specification.ProductQuote != "browser tool" {
		t.Fatalf("grounded product identity=%q", specification.ProductQuote)
	}
}

func TestRequirementFeatureEnvelopesRecursivelySplitToFixedPoint(t *testing.T) {
	t.Parallel()

	request := "Build a browser inventory with grouped records and my own saved filter and printable summary, sized for a weekend."
	responses := []any{
		browserClassification(), applicationIdentity("browser inventory"),
		partitionDecision("grouped records", "my own saved filter and printable summary"),
		partitionDecision("grouped records"),
		partitionDecision("my own saved filter", "printable summary"),
		partitionDecision("my own saved filter"), partitionDecision("printable summary"),
	}
	script := scriptedSemanticRuntime(t, responses)
	specification, err := runDirectCodingApplicationInterpreter(
		script.runtime, "partition", "split", "surface", "identity", "artifact", request, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if got := requirementSourceQuotes(specification.Requirements); !equalStrings(got, []string{
		"grouped records", "my own saved filter", "printable summary",
	}) {
		t.Fatalf("atomic feature quotes=%#v", got)
	}
	if specification.ProductQuote != "browser inventory" {
		t.Fatalf("grounded product=%q", specification.ProductQuote)
	}
}

func TestFeatureSplitFailureGetsDirectCorrectionOnTheSameSmallJob(t *testing.T) {
	t.Parallel()

	request := "Build a browser tool that can filter the records."
	responses := []any{
		browserClassification(), applicationIdentity("browser tool"), partitionDecision("filter the records"),
		partitionDecision(), map[string]any{"feature_quotes": []string{"filter the records"}},
	}
	script := scriptedSemanticRuntime(t, responses)
	_, err := runDirectCodingApplicationInterpreter(
		script.runtime, "partition", "split", "surface", "identity", "artifact", request, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	var correctionPrompt string
	for _, prompt := range script.prompts {
		if strings.Contains(prompt, "JSON merge patch") {
			correctionPrompt = prompt
			break
		}
	}
	if !strings.Contains(correctionPrompt, "requires at least one") || strings.Contains(correctionPrompt, request) {
		t.Fatalf("direct local correction was not delivered:\n%s", correctionPrompt)
	}
}

func TestApplicationInterpreterFailsWhenExtractionFindsNoFeatures(t *testing.T) {
	t.Parallel()

	script := scriptedSemanticRuntime(t, []any{
		browserClassification(), applicationIdentity("browser application"), partitionDecision(),
	})
	_, err := runDirectCodingApplicationInterpreter(
		script.runtime, "partition", "split", "surface", "identity", "artifact", "Build a browser application.", nil,
	)
	if err == nil || !strings.Contains(err.Error(), "no grounded application features") {
		t.Fatalf("expected explicit empty extraction failure, got %v", err)
	}
}

type semanticScript struct {
	models          []string
	prompts         []string
	advisoryModels  []string
	advisoryPrompts []string
	runtime         typedWorkerRuntime
}

func scriptedSemanticRuntime(t *testing.T, responses []any) *semanticScript {
	t.Helper()
	script := &semanticScript{
		models: make([]string, 0, len(responses)*2), prompts: make([]string, 0, len(responses)*2),
		advisoryModels: make([]string, 0, len(responses)), advisoryPrompts: make([]string, 0, len(responses)),
	}
	responseIndex := 0
	script.runtime = typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3, AdvisoryModel: "adviser-model",
		Execute: func(job assemblyline.PortableJob, model string) (assemblyline.PortableResult, error) {
			prompt, _, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return assemblyline.PortableResult{}, err
			}
			script.models = append(script.models, model)
			script.prompts = append(script.prompts, prompt)
			var response any
			if job.Kind == assemblyline.WorkRequirementBriefing {
				response = assemblyline.RequirementPartitionBriefingDecision{
					Schema: assemblyline.RequirementPartitionBriefingSchemaV1,
					Lens:   assemblyline.RequirementLensCoverage,
				}
			} else {
				if responseIndex >= len(responses) {
					t.Fatalf("unexpected semantic call %d:\n%s", len(script.models), prompt)
				}
				response = responses[responseIndex]
				responseIndex++
			}
			encoded, err := json.Marshal(response)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(encoded)}, err
		},
		Advise: func(job assemblyline.PortableJob, model string) (llm.AdvisoryResponse, error) {
			prompt, schema, err := assemblyline.RenderPortableJob(job)
			if err != nil {
				return llm.AdvisoryResponse{}, err
			}
			if schema != nil {
				t.Fatalf("advisory job returned schema %#v", schema)
			}
			script.advisoryModels = append(script.advisoryModels, model)
			script.advisoryPrompts = append(script.advisoryPrompts, prompt)
			return llm.AdvisoryResponse{Thinking: "evidence only", Content: "bounded final critique memo"}, nil
		},
	}
	return script
}

func browserClassification() assemblyline.ApplicationClassification {
	return assemblyline.ApplicationClassification{
		Schema: assemblyline.ApplicationClassificationSchemaV1, Surface: assemblyline.ApplicationSurfaceBrowser,
	}
}

func applicationIdentity(productQuote string) assemblyline.ApplicationIdentity {
	return assemblyline.ApplicationIdentity{
		Schema: assemblyline.ApplicationIdentitySchemaV1, ProductQuote: productQuote,
	}
}

func partitionDecision(quotes ...string) assemblyline.RequirementPartitionDecision {
	return assemblyline.RequirementPartitionDecision{
		Schema: assemblyline.RequirementPartitionSchemaV1, FeatureQuotes: append([]string(nil), quotes...),
	}
}

func countValue(values []string, target string) int {
	count := 0
	for _, value := range values {
		if value == target {
			count++
		}
	}
	return count
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
