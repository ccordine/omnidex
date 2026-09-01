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

func BuildRequirementGraph(request string, statements []string) (RequirementGraph, error) {
	request = strings.TrimSpace(request)
	if request == "" {
		return RequirementGraph{}, fmt.Errorf("requirement graph requires the current user request")
	}
	if len(statements) == 0 {
		return RequirementGraph{}, fmt.Errorf("requirement graph requires at least one grounded statement")
	}
	if len(statements) > maxRequirementCount {
		return RequirementGraph{}, fmt.Errorf("requirement graph exceeds the %d-requirement limit", maxRequirementCount)
	}
	seen := make(map[string]struct{}, len(statements))
	requirements := make([]Requirement, 0, len(statements))
	for index, statement := range statements {
		if err := validateRequirementQuote("requirement statement", statement); err != nil {
			return RequirementGraph{}, fmt.Errorf("requirement statement %d: %w", index, err)
		}
		if _, duplicate := seen[statement]; duplicate {
			return RequirementGraph{}, fmt.Errorf("requirement statement %d duplicates %q", index, statement)
		}
		seen[statement] = struct{}{}
		requirements = append(requirements, Requirement{
			ID: fmt.Sprintf("requirement_%03d", index+1), SourceQuote: statement,
		})
	}
	return RequirementGraph{Requirements: requirements}, nil
}
