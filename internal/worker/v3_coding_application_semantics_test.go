package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
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
	if countValue(script.models, "partition-model") != 1 || countValue(script.models, "split-model") != 2 ||
		countValue(script.models, "identity-model") != 1 ||
		countValue(script.models, "artifact-model") != 0 {
		t.Fatalf("unexpected semantic routing: models=%#v", script.models)
	}
	for _, index := range []int{3, 4} {
		if strings.Contains(script.prompts[index], request) {
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
	if !strings.Contains(script.prompts[4], "JSON merge patch") ||
		!strings.Contains(script.prompts[4], "requires at least one") ||
		strings.Contains(script.prompts[4], request) {
		t.Fatalf("direct local correction was not delivered:\n%s", script.prompts[4])
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
	models  []string
	prompts []string
	runtime typedWorkerRuntime
}

func scriptedSemanticRuntime(t *testing.T, responses []any) *semanticScript {
	t.Helper()
	script := &semanticScript{models: make([]string, 0, len(responses)), prompts: make([]string, 0, len(responses))}
	script.runtime = typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: testPortableExecutor(func(_ string, model string, prompt string, _ map[string]any) (string, error) {
			script.models = append(script.models, model)
			script.prompts = append(script.prompts, prompt)
			if len(script.models) > len(responses) {
				t.Fatalf("unexpected semantic call %d:\n%s", len(script.models), prompt)
			}
			encoded, err := json.Marshal(responses[len(script.models)-1])
			return string(encoded), err
		}),
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
