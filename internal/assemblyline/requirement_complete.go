package assemblyline

import (
	"fmt"
	"sort"
	"strings"
)

const maxCompleteRequirementPartitionCalls = 128

type RequirementPartitionCall func(RequirementPartitionInput) (RequirementPartitionDecision, error)

func ValidateCompleteRequirementPartition(source string, candidate RequirementPartitionDecision) error {
	input := RequirementPartitionInput{SourceText: source, Mode: RequirementExtractFeatures}
	if err := input.validate(); err != nil {
		return err
	}
	if err := candidate.ValidateFor(input); err != nil {
		return err
	}
	if len(candidate.FeatureQuotes) == 0 {
		return fmt.Errorf("complete requirement partition requires at least one grounded feature quote")
	}
	if _, err := BuildRequirementResidual(source, candidate.FeatureQuotes); err != nil {
		return fmt.Errorf("complete requirement partition residual: %w", err)
	}
	if _, err := BuildRequirementGraph(source, candidate.FeatureQuotes); err != nil {
		return fmt.Errorf("complete requirement partition graph: %w", err)
	}
	return nil
}

// CompleteRequirementPartition owns extraction-to-fixed-point, strict split
// progress, source ordering, residual validation, and graph construction. The
// callback owns only one tiny semantic decision at a time.
func CompleteRequirementPartition(
	source string,
	call RequirementPartitionCall,
) (RequirementPartitionDecision, error) {
	if call == nil {
		return RequirementPartitionDecision{}, fmt.Errorf("complete requirement partition requires one semantic call")
	}
	if err := (RequirementPartitionInput{
		SourceText: source, Mode: RequirementExtractFeatures,
	}).validate(); err != nil {
		return RequirementPartitionDecision{}, err
	}
	calls := 0
	invoke := func(input RequirementPartitionInput) (RequirementPartitionDecision, error) {
		if calls >= maxCompleteRequirementPartitionCalls {
			return RequirementPartitionDecision{}, fmt.Errorf(
				"complete requirement partition exceeded %d bounded semantic calls",
				maxCompleteRequirementPartitionCalls,
			)
		}
		calls++
		decision, err := call(input)
		if err != nil {
			return RequirementPartitionDecision{}, err
		}
		if err := decision.ValidateFor(input); err != nil {
			return RequirementPartitionDecision{}, err
		}
		return decision, nil
	}

	envelopes := make([]string, 0)
	for {
		residual, err := BuildRequirementResidual(source, envelopes)
		if err != nil {
			return RequirementPartitionDecision{}, err
		}
		if strings.TrimSpace(residual) == "" {
			break
		}
		decision, err := invoke(RequirementPartitionInput{
			SourceText: residual, Mode: RequirementExtractFeatures,
		})
		if err != nil {
			return RequirementPartitionDecision{}, err
		}
		if len(decision.FeatureQuotes) == 0 {
			break
		}
		envelopes = append(envelopes, decision.FeatureQuotes...)
		if len(envelopes) > maxRequirementCount {
			return RequirementPartitionDecision{}, fmt.Errorf(
				"complete requirement partition exceeds %d grounded envelopes", maxRequirementCount,
			)
		}
	}
	if len(envelopes) == 0 {
		return RequirementPartitionDecision{}, fmt.Errorf("complete requirement extraction returned no feature envelopes")
	}

	queue := append([]string(nil), envelopes...)
	features := make([]string, 0, len(queue))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		decision, err := invoke(RequirementPartitionInput{
			SourceText: current, Mode: RequirementSplitFeature,
		})
		if err != nil {
			return RequirementPartitionDecision{}, err
		}
		if len(decision.FeatureQuotes) == 1 && decision.FeatureQuotes[0] == current {
			features = append(features, current)
			continue
		}
		for _, child := range decision.FeatureQuotes {
			if len(child) >= len(current) {
				return RequirementPartitionDecision{}, fmt.Errorf(
					"requirement split %q did not make strict progress from %q", child, current,
				)
			}
			queue = append(queue, child)
		}
	}
	sort.SliceStable(features, func(left, right int) bool {
		return strings.Index(source, features[left]) < strings.Index(source, features[right])
	})
	decision := RequirementPartitionDecision{
		Schema: RequirementPartitionSchemaV1, FeatureQuotes: features,
	}
	if err := ValidateCompleteRequirementPartition(source, decision); err != nil {
		return RequirementPartitionDecision{}, err
	}
	return decision, nil
}
