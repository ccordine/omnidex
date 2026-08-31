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
	modelName, err := s.workerModel(station.CodingRuntimeCapabilityNecessity)
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
		station.CodingRuntimeCapabilityNecessity, modelName,
	)
	if err != nil {
		return nil, err
	}
	graph := emptyDirectCodingRuntimeCapabilityGraph(requirements)
	for requirementIndex, requirement := range requirements {
		selectionContext := directCodingRuntimeCapabilityLocalContext(
			localContext, dependencies[requirement.ID],
		)
		for candidateIndex, candidate := range candidates {
			input := assemblyline.RuntimeCapabilityNecessityInput{
				LocalContext:     selectionContext,
				Need:             requirement.SourceQuote,
				Dialect:          dialect,
				CandidatePurpose: candidate.Purpose,
			}
			job, err := assemblyline.NewRuntimeCapabilityNecessityJob(input)
			if err != nil {
				return nil, fmt.Errorf(
					"runtime capability necessity input for requirement %s candidate %s: %w",
					requirement.ID, candidate.ID, err,
				)
			}
			decision, err := runDirectCodingSemanticLeafCall(
				runtime, modelName,
				fmt.Sprintf("runtime_capability_necessity_%03d_%03d", requirementIndex+1, candidateIndex+1),
				job, nil,
				func(raw string) (assemblyline.RuntimeCapabilityNecessityDecision, error) {
					return assemblyline.DecodeRuntimeCapabilityNecessityDecision(input, raw)
				},
				func(value assemblyline.RuntimeCapabilityNecessityDecision) error {
					return value.ValidateFor(input)
				},
			)
			if err != nil {
				if isDirectCodingSemanticLeafRejection(err) {
					continue
				}
				return nil, err
			}
			if decision.Relation == assemblyline.RuntimeCapabilityNecessary {
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
