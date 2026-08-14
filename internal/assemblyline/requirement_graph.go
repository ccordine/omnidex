package assemblyline

import (
	"fmt"
	"strings"
)

const (
	maxRequirementCount      = 10
	maxRequirementQuoteBytes = 1024
)

type Requirement struct {
	ID          string
	SourceQuote string
}

type RequirementGraph struct {
	Requirements []Requirement
}

func BuildRequirementGraph(request string, sourceQuotes []string) (RequirementGraph, error) {
	request = strings.TrimSpace(request)
	if request == "" {
		return RequirementGraph{}, fmt.Errorf("requirement graph requires the current user request")
	}
	if len(sourceQuotes) == 0 {
		return RequirementGraph{}, fmt.Errorf("requirement graph requires at least one grounded feature quote")
	}
	if len(sourceQuotes) > maxRequirementCount {
		return RequirementGraph{}, fmt.Errorf("requirement graph exceeds the %d-requirement limit", maxRequirementCount)
	}
	if err := validateOrderedRequirementQuotes(request, sourceQuotes, validateRequirementQuote); err != nil {
		return RequirementGraph{}, fmt.Errorf("requirement graph source quotes: %w", err)
	}
	requirements := make([]Requirement, 0, len(sourceQuotes))
	for index, quote := range sourceQuotes {
		requirements = append(requirements, Requirement{
			ID: fmt.Sprintf("requirement_%03d", index+1), SourceQuote: quote,
		})
	}
	return RequirementGraph{Requirements: requirements}, nil
}

func validateOrderedRequirementQuotes(
	source string,
	quotes []string,
	validate func(string, string) error,
) error {
	accepted := make([]textSpan, 0, len(quotes))
	seen := make(map[string]struct{}, len(quotes))
	for index, quote := range quotes {
		if err := validate("requirement feature", quote); err != nil {
			return fmt.Errorf("feature quote %d: %w", index, err)
		}
		if _, duplicate := seen[quote]; duplicate {
			return fmt.Errorf("feature quote %d duplicates %q", index, quote)
		}
		seen[quote] = struct{}{}
		span, err := uniqueTextSpan(source, quote)
		if err != nil {
			return fmt.Errorf("feature quote %d %q: %w", index, quote, err)
		}
		for _, prior := range accepted {
			if span.Overlaps(prior) {
				return fmt.Errorf("feature quote %d %q overlaps another feature quote", index, quote)
			}
		}
		if len(accepted) > 0 && span.Start < accepted[len(accepted)-1].Start {
			return fmt.Errorf("feature quotes must preserve source order")
		}
		accepted = append(accepted, span)
	}
	return nil
}
