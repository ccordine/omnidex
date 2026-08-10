package assemblyline

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestCompleteRequirementPartitionExhaustsResidualAndSplitsToFixedPoint(t *testing.T) {
	t.Parallel()
	source := "Build alerts and CSV exports with careful quality."
	var calls []RequirementPartitionInput
	decision, err := CompleteRequirementPartition(source, func(input RequirementPartitionInput) (RequirementPartitionDecision, error) {
		calls = append(calls, input)
		quotes := []string{}
		switch {
		case input.Mode == RequirementExtractFeatures && strings.Contains(input.SourceText, "alerts"):
			quotes = []string{"alerts"}
		case input.Mode == RequirementExtractFeatures && strings.Contains(input.SourceText, "CSV exports"):
			quotes = []string{"CSV exports"}
		case input.Mode == RequirementSplitFeature:
			quotes = []string{input.SourceText}
		}
		return RequirementPartitionDecision{Schema: RequirementPartitionSchemaV1, FeatureQuotes: quotes}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decision.FeatureQuotes, []string{"alerts", "CSV exports"}) {
		t.Fatalf("complete decision=%+v", decision)
	}
	extracts := 0
	for _, call := range calls {
		if call.Mode == RequirementExtractFeatures {
			extracts++
		}
	}
	if extracts != 3 {
		t.Fatalf("fixed-point extraction calls=%d want 3: %+v", extracts, calls)
	}
}

func TestCompleteRequirementPartitionRejectsNoProgressAndBoundOverflow(t *testing.T) {
	t.Parallel()
	extractionCalls := 0
	_, err := CompleteRequirementPartition("Build alerts and exports.", func(input RequirementPartitionInput) (RequirementPartitionDecision, error) {
		if input.Mode == RequirementExtractFeatures {
			extractionCalls++
			quotes := []string{}
			if extractionCalls == 1 {
				quotes = []string{"alerts and exports"}
			}
			return RequirementPartitionDecision{Schema: RequirementPartitionSchemaV1, FeatureQuotes: quotes}, nil
		}
		return RequirementPartitionDecision{Schema: RequirementPartitionSchemaV1, FeatureQuotes: []string{"alerts and exports", "alerts"}}, nil
	})
	if err == nil || !strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("invalid split error=%v", err)
	}

	parts := make([]string, maxCompleteRequirementPartitionCalls+4)
	for index := range parts {
		parts[index] = fmt.Sprintf("feature%03d", index)
	}
	large := strings.Join(parts, " ")
	index := 0
	_, err = CompleteRequirementPartition(large, func(input RequirementPartitionInput) (RequirementPartitionDecision, error) {
		if input.Mode == RequirementExtractFeatures {
			quote := parts[index]
			index++
			return RequirementPartitionDecision{Schema: RequirementPartitionSchemaV1, FeatureQuotes: []string{quote}}, nil
		}
		return RequirementPartitionDecision{Schema: RequirementPartitionSchemaV1, FeatureQuotes: []string{input.SourceText}}, nil
	})
	if err == nil {
		t.Fatalf("bounded partition unexpectedly succeeded after %d calls", index)
	}
}
