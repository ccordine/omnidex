package worker

import (
	"fmt"
	"strconv"
	"sync"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type directCodingCapabilityBinding struct {
	RequirementID string
	CapabilityID  string
	Purpose       string
}

type directCodingCapabilityGraph map[string][]directCodingCapabilityBinding

type directCodingCapabilityPair struct {
	LeftIndex  int
	RightIndex int
	Input      assemblyline.CapabilityRelationInput
}

type directCodingCapabilityResult struct {
	Pair      directCodingCapabilityPair
	Decision  assemblyline.CapabilityRelationDecision
	Discarded bool
	Err       error
}

func (s *directCodingSession) deriveRequirementCapabilities(
	localContext string,
	requirements []assemblyline.Requirement,
) (directCodingCapabilityGraph, error) {
	if len(requirements) == 0 {
		return nil, fmt.Errorf("capability projection requires at least one accepted requirement")
	}
	if len(requirements) == 1 {
		return directCodingCapabilityGraph{requirements[0].ID: nil}, nil
	}
	routing, err := s.runtime.modelRouting()
	if err != nil {
		return nil, err
	}
	modelName, err := stationModel(routing, station.CodingCapabilityRelation)
	if err != nil {
		return nil, err
	}
	pairs := directCodingCapabilityPairs(localContext, requirements)
	concurrency, err := directCodingFragmentConcurrency(s.runtime.svc.fragmentConcurrency)
	if err != nil {
		return nil, err
	}
	results := runDirectCodingCapabilityPairs(
		directCodingWorkerRuntime(s), modelName, pairs, concurrency,
	)
	for _, result := range results {
		if result.Err != nil {
			return nil, result.Err
		}
	}
	return assembleDirectCodingCapabilityGraph(requirements, results)
}

func directCodingCapabilityPairs(
	localContext string,
	requirements []assemblyline.Requirement,
) []directCodingCapabilityPair {
	pairs := make([]directCodingCapabilityPair, 0, len(requirements)*(len(requirements)-1)/2)
	for left := 0; left < len(requirements); left++ {
		for right := left + 1; right < len(requirements); right++ {
			pairs = append(pairs, directCodingCapabilityPair{
				LeftIndex: left, RightIndex: right,
				Input: assemblyline.CapabilityRelationInput{
					LocalContext: localContext,
					LeftNeed:     requirements[left].SourceQuote,
					RightNeed:    requirements[right].SourceQuote,
				},
			})
		}
	}
	return pairs
}

func runDirectCodingCapabilityPairs(
	runtime typedWorkerRuntime,
	modelName string,
	pairs []directCodingCapabilityPair,
	maxConcurrency int,
) []directCodingCapabilityResult {
	results := make([]directCodingCapabilityResult, len(pairs))
	run := func(index int, pair directCodingCapabilityPair) {
		job, err := assemblyline.NewCapabilityRelationJob(pair.Input)
		if err != nil {
			results[index] = directCodingCapabilityResult{Pair: pair, Err: err}
			return
		}
		decision, err := runDirectCodingSemanticLeafCall(
			runtime, modelName, fmt.Sprintf("capability_relation_%03d_%03d", pair.LeftIndex+1, pair.RightIndex+1),
			job, nil,
			func(raw string) (assemblyline.CapabilityRelationDecision, error) {
				return assemblyline.DecodeCapabilityRelationDecision(pair.Input, raw)
			},
		)
		if isDirectCodingSemanticLeafRejection(err) {
			results[index] = directCodingCapabilityResult{Pair: pair, Discarded: true}
			return
		}
		results[index] = directCodingCapabilityResult{Pair: pair, Decision: decision, Err: err}
	}
	if maxConcurrency == 1 {
		for index, pair := range pairs {
			run(index, pair)
			if results[index].Err != nil {
				break
			}
		}
		return results
	}
	semaphore := make(chan struct{}, maxConcurrency)
	var wait sync.WaitGroup
	for index, pair := range pairs {
		index, pair := index, pair
		wait.Add(1)
		go func() {
			defer wait.Done()
			semaphore <- struct{}{}
			defer func() { <-semaphore }()
			run(index, pair)
		}()
	}
	wait.Wait()
	return results
}

func directCodingFragmentConcurrency(raw string) (int, error) {
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("CODING_FRAGMENT_CONCURRENCY must be an integer: %w", err)
	}
	if value < 1 {
		return 0, fmt.Errorf("CODING_FRAGMENT_CONCURRENCY must be at least 1, received %d", value)
	}
	return value, nil
}

func assembleDirectCodingCapabilityGraph(
	requirements []assemblyline.Requirement,
	results []directCodingCapabilityResult,
) (directCodingCapabilityGraph, error) {
	graph := make(directCodingCapabilityGraph, len(requirements))
	for _, requirement := range requirements {
		graph[requirement.ID] = nil
	}
	add := func(ownerIndex, dependencyIndex int) bool {
		owner := requirements[ownerIndex]
		dependency := requirements[dependencyIndex]
		if directCodingCapabilityPathExists(graph, dependency.ID, owner.ID) {
			return false
		}
		graph[owner.ID] = append(graph[owner.ID], directCodingCapabilityBinding{
			RequirementID: dependency.ID,
			CapabilityID:  genericApplicationCapabilityID(dependencyIndex + 1),
			Purpose:       dependency.SourceQuote,
		})
		return true
	}
	for _, result := range results {
		if result.Err != nil {
			return nil, result.Err
		}
		if result.Discarded {
			continue
		}
		if err := result.Decision.ValidateFor(result.Pair.Input); err != nil {
			return nil, err
		}
		switch result.Decision.Relation {
		case assemblyline.CapabilityIndependent:
		case assemblyline.CapabilityLeftReadsRight:
			add(result.Pair.LeftIndex, result.Pair.RightIndex)
		case assemblyline.CapabilityRightReadsLeft:
			add(result.Pair.RightIndex, result.Pair.LeftIndex)
		}
	}
	if err := validateDirectCodingCapabilityGraph(requirements, graph); err != nil {
		return nil, err
	}
	return graph, nil
}

func directCodingCapabilityPathExists(
	graph directCodingCapabilityGraph,
	from, target string,
) bool {
	seen := make(map[string]struct{}, len(graph))
	var visit func(string) bool
	visit = func(current string) bool {
		if current == target {
			return true
		}
		if _, exists := seen[current]; exists {
			return false
		}
		seen[current] = struct{}{}
		for _, dependency := range graph[current] {
			if visit(dependency.RequirementID) {
				return true
			}
		}
		return false
	}
	return visit(from)
}

func validateDirectCodingCapabilityGraph(
	requirements []assemblyline.Requirement,
	graph directCodingCapabilityGraph,
) error {
	if len(graph) != len(requirements) {
		return fmt.Errorf("capability graph=%d does not cover requirements=%d", len(graph), len(requirements))
	}
	indices := make(map[string]int, len(requirements))
	for index, requirement := range requirements {
		indices[requirement.ID] = index
	}
	for _, owner := range requirements {
		dependencies, exists := graph[owner.ID]
		if !exists {
			return fmt.Errorf("capability graph omits requirement %s", owner.ID)
		}
		seen := make(map[string]struct{}, len(dependencies))
		lastIndex := -1
		for _, dependency := range dependencies {
			index, exists := indices[dependency.RequirementID]
			if !exists || dependency.RequirementID == owner.ID {
				return fmt.Errorf("requirement %s has invalid capability dependency %s", owner.ID, dependency.RequirementID)
			}
			if dependency.CapabilityID != genericApplicationCapabilityID(index+1) ||
				dependency.Purpose != requirements[index].SourceQuote {
				return fmt.Errorf("requirement %s capability dependency %s is not code-owned", owner.ID, dependency.RequirementID)
			}
			if _, duplicate := seen[dependency.RequirementID]; duplicate || index <= lastIndex {
				return fmt.Errorf("requirement %s capability dependencies are duplicated or unordered", owner.ID)
			}
			seen[dependency.RequirementID] = struct{}{}
			lastIndex = index
		}
	}
	return validateDirectCodingCapabilityGraphAcyclic(requirements, graph)
}

func validateDirectCodingCapabilityGraphAcyclic(
	requirements []assemblyline.Requirement,
	graph directCodingCapabilityGraph,
) error {
	const (
		capabilityVisiting = iota + 1
		capabilityVisited
	)
	states := make(map[string]int, len(requirements))
	var visit func(string) error
	visit = func(requirementID string) error {
		switch states[requirementID] {
		case capabilityVisiting:
			return fmt.Errorf("capability graph contains a cycle through requirement %s", requirementID)
		case capabilityVisited:
			return nil
		}
		states[requirementID] = capabilityVisiting
		for _, dependency := range graph[requirementID] {
			if err := visit(dependency.RequirementID); err != nil {
				return err
			}
		}
		states[requirementID] = capabilityVisited
		return nil
	}
	for _, requirement := range requirements {
		if err := visit(requirement.ID); err != nil {
			return err
		}
	}
	return nil
}
