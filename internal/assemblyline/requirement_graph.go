package assemblyline

import (
	"fmt"
	"strings"
)

const (
	maxRequirementCount          = 96
	maxRequirementPartitionCount = 32
	maxRequirementQuoteBytes     = 1024
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
	if _, err := BuildRequirementResidual(request, sourceQuotes); err != nil {
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
