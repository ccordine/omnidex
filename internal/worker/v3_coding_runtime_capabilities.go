package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

const maxDirectCodingRuntimeCapabilitiesPerRequirement = 5

type directCodingRuntimeCapability struct {
	ID      string
	Purpose string
}

type directCodingRuntimeCapabilityGraph map[string][]string

func (s *directCodingSession) selectRequirementRuntimeCapabilities(
	stack directCodingProjectStack,
	localContext string,
	dialect string,
	requirements []assemblyline.Requirement,
	dependencies directCodingCapabilityGraph,
) (directCodingRuntimeCapabilityGraph, error) {
	if stack.RuntimeCapabilities == nil {
		return emptyDirectCodingRuntimeCapabilityGraph(requirements), nil
	}
	candidates, err := stack.RuntimeCapabilities()
	if err != nil {
		return nil, fmt.Errorf("load %s runtime capabilities: %w", stack.ID, err)
	}
	if err := validateDirectCodingRuntimeCapabilityRegistry(candidates); err != nil {
		return nil, fmt.Errorf("validate %s runtime capabilities: %w", stack.ID, err)
	}
	modelName, err := s.workerModel(station.CodingRuntimeCapabilitySelection)
	if err != nil {
		return nil, err
	}
	return selectDirectCodingRuntimeCapabilities(
		directCodingWorkerRuntime(s), modelName, localContext, dialect,
		requirements, dependencies, candidates,
	)
}

func emptyDirectCodingRuntimeCapabilityGraph(
	requirements []assemblyline.Requirement,
) directCodingRuntimeCapabilityGraph {
	graph := make(directCodingRuntimeCapabilityGraph, len(requirements))
	for _, requirement := range requirements {
		graph[requirement.ID] = nil
	}
	return graph
}

func selectDirectCodingRuntimeCapabilities(
	runtime typedWorkerRuntime,
	modelName string,
	localContext string,
	dialect string,
	requirements []assemblyline.Requirement,
	dependencies directCodingCapabilityGraph,
	candidates []directCodingRuntimeCapability,
) (directCodingRuntimeCapabilityGraph, error) {
	if err := validateDirectCodingRequirementCount(requirements); err != nil {
		return nil, err
	}
	if err := validateDirectCodingRuntimeCapabilityRegistry(candidates); err != nil {
		return nil, err
	}
	if err := validateDirectCodingCapabilityGraph(requirements, dependencies); err != nil {
		return nil, err
	}
	modelName, err := requireDirectCodingModel(
		station.CodingRuntimeCapabilitySelection, modelName,
	)
	if err != nil {
		return nil, err
	}
	graph := emptyDirectCodingRuntimeCapabilityGraph(requirements)
	for requirementIndex, requirement := range requirements {
		selectionContext := directCodingRuntimeCapabilityLocalContext(
			localContext, dependencies[requirement.ID],
		)
		selected := make(map[string]struct{}, len(candidates))
		selectedPurposes := make([]string, 0, len(candidates))
		for len(selected) < len(candidates) {
			input, candidateByToken, err := directCodingRuntimeCapabilitySelectionInput(
				selectionContext, requirement.SourceQuote, dialect,
				candidates, selected, selectedPurposes,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"runtime capability selection input for requirement %s: %w",
					requirement.ID, err,
				)
			}
			job, err := assemblyline.NewRuntimeCapabilitySelectionJob(input)
			if err != nil {
				return nil, err
			}
			decision, err := runDirectCodingSemanticLeafCall(
				runtime, modelName,
				fmt.Sprintf("runtime_capability_selection_%03d_%03d", requirementIndex+1, len(selected)+1),
				job, nil,
				func(raw string) (assemblyline.RuntimeCapabilitySelectionDecision, error) {
					return assemblyline.DecodeRuntimeCapabilitySelectionDecision(input, raw)
				},
				func(value assemblyline.RuntimeCapabilitySelectionDecision) error {
					return value.ValidateFor(input)
				},
			)
			if err != nil {
				return nil, err
			}
			if decision.Selected == assemblyline.RuntimeCapabilitySelectionNone {
				break
			}
			candidate, exists := candidateByToken[decision.Selected]
			if !exists {
				return nil, fmt.Errorf("runtime capability selection escaped its code-owned candidate map")
			}
			if _, duplicate := selected[candidate.ID]; duplicate {
				return nil, fmt.Errorf("runtime capability selection repeated registered ID %s", candidate.ID)
			}
			selected[candidate.ID] = struct{}{}
			selectedPurposes = append(selectedPurposes, candidate.Purpose)
			if len(selected) > maxDirectCodingRuntimeCapabilitiesPerRequirement {
				return nil, fmt.Errorf(
					"requirement %s exceeds the %d runtime-capability bound",
					requirement.ID, maxDirectCodingRuntimeCapabilitiesPerRequirement,
				)
			}
		}
		for _, candidate := range candidates {
			if _, exists := selected[candidate.ID]; exists {
				graph[requirement.ID] = append(graph[requirement.ID], candidate.ID)
			}
		}
	}
	if err := validateDirectCodingRuntimeCapabilityGraph(requirements, candidates, graph); err != nil {
		return nil, err
	}
	return graph, nil
}

func directCodingRuntimeCapabilityLocalContext(
	productContext string,
	dependencies []directCodingCapabilityBinding,
) string {
	if len(dependencies) == 0 {
		return productContext
	}
	lines := []string{productContext, "Direct dependency results already provide:"}
	for _, dependency := range dependencies {
		lines = append(lines, "- "+dependency.Purpose)
	}
	return strings.Join(lines, "\n")
}

func directCodingRuntimeCapabilitySelectionInput(
	localContext string,
	need string,
	dialect string,
	candidates []directCodingRuntimeCapability,
	selected map[string]struct{},
	selectedPurposes []string,
) (assemblyline.RuntimeCapabilitySelectionInput, map[string]directCodingRuntimeCapability, error) {
	remaining := make([]assemblyline.RuntimeCapabilityCandidateSummary, 0, len(candidates)-len(selected))
	byToken := make(map[string]directCodingRuntimeCapability, len(candidates)-len(selected))
	for _, candidate := range candidates {
		if _, exists := selected[candidate.ID]; exists {
			continue
		}
		token := fmt.Sprintf("RUNTIME_CAPABILITY_%d", len(remaining)+1)
		remaining = append(remaining, assemblyline.RuntimeCapabilityCandidateSummary{
			CandidateID: token, Purpose: candidate.Purpose,
		})
		byToken[token] = candidate
	}
	input := assemblyline.RuntimeCapabilitySelectionInput{
		LocalContext: localContext,
		Need:         need,
		Dialect:      dialect,
		AcceptedPurposes: append(
			[]string{}, selectedPurposes...,
		),
		Candidates: remaining,
	}
	if _, err := assemblyline.NewRuntimeCapabilitySelectionJob(input); err != nil {
		return assemblyline.RuntimeCapabilitySelectionInput{}, nil, err
	}
	return input, byToken, nil
}
