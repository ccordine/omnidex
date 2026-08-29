package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func bindDirectCodingGoRuntimeCapabilities(
	program directCodingProgram,
	graph directCodingRuntimeCapabilityGraph,
) (directCodingProgram, error) {
	if program.StackID != genericGoCommandLineAdapter {
		return directCodingProgram{}, fmt.Errorf(
			"Go runtime capability binding received project stack %s", program.StackID,
		)
	}
	if len(program.Generated) != 0 {
		return directCodingProgram{}, fmt.Errorf(
			"Go runtime capabilities must be bound before source generation",
		)
	}
	registered, err := registeredDirectCodingGoStandardLibraryCapabilities()
	if err != nil {
		return directCodingProgram{}, err
	}
	candidates := make([]directCodingRuntimeCapability, len(registered))
	byID := make(map[string]directCodingGoStandardLibraryCapability, len(registered))
	for index, capability := range registered {
		candidates[index] = directCodingRuntimeCapability{
			ID: capability.ID, Purpose: capability.Purpose,
		}
		byID[capability.ID] = capability
	}
	requirements, err := directCodingRequirementsFromFrozenWorkload(program.Workload)
	if err != nil {
		return directCodingProgram{}, err
	}
	if err := validateDirectCodingRuntimeCapabilityGraph(requirements, candidates, graph); err != nil {
		return directCodingProgram{}, err
	}
	selectedUnion := make(map[string]struct{}, len(registered))
	for _, selected := range graph {
		for _, capabilityID := range selected {
			selectedUnion[capabilityID] = struct{}{}
		}
	}
	selectedCapabilities := make([]directCodingGoStandardLibraryCapability, 0, len(selectedUnion))
	for _, capability := range registered {
		if _, selected := selectedUnion[capability.ID]; selected {
			selectedCapabilities = append(selectedCapabilities, capability)
		}
	}
	runtimeDocument, err := goCommandLineRuntimeDocument(selectedCapabilities)
	if err != nil {
		return directCodingProgram{}, err
	}

	bound := program
	bound.Source = cloneDirectCodingRuntimeCapabilityBlueprint(program.Source)
	runtimeIndex := -1
	implementationByRequirement := make(map[string]int, len(requirements))
	requirementByTask := make(map[string]string, len(program.Workload.Tasks))
	for _, task := range program.Workload.Tasks {
		requirementByTask[task.ID] = task.RequirementID
	}
	for documentIndex := range bound.Source.Documents {
		document := &bound.Source.Documents[documentIndex]
		if document.ID == "application_runtime" {
			if runtimeIndex >= 0 {
				return directCodingProgram{}, fmt.Errorf("Go source repeats application runtime document")
			}
			runtimeIndex = documentIndex
			if len(document.Blocks) != 1 || document.Blocks[0].ID != "runtime.api" {
				return directCodingProgram{}, fmt.Errorf(
					"Go base runtime already contains non-selected capability source",
				)
			}
			runtimeDocument.AdapterID = document.AdapterID
			continue
		}
		for blockIndex := range document.Blocks {
			block := &document.Blocks[blockIndex]
			if err := rejectDirectCodingGoRuntimeCapabilityAuthority(block, byID); err != nil {
				return directCodingProgram{}, err
			}
			if block.Role != assemblyline.SourceBlockTaskImplementation {
				continue
			}
			requirementID, exists := requirementByTask[block.TaskID]
			if !exists {
				return directCodingProgram{}, fmt.Errorf(
					"Go implementation %s names unknown task %s", block.ID, block.TaskID,
				)
			}
			implementationByRequirement[requirementID]++
			block.DependsOn, err = insertDirectCodingGoRuntimeCapabilityIDs(
				block.ID, block.DependsOn, graph[requirementID],
			)
			if err != nil {
				return directCodingProgram{}, err
			}
			block.Capabilities, err = insertDirectCodingGoRuntimeCapabilityIDs(
				block.ID, block.Capabilities, graph[requirementID],
			)
			if err != nil {
				return directCodingProgram{}, err
			}
		}
	}
	if runtimeIndex < 0 {
		return directCodingProgram{}, fmt.Errorf("Go source omits application runtime document")
	}
	bound.Source.Documents[runtimeIndex] = runtimeDocument
	for _, requirement := range requirements {
		if implementationByRequirement[requirement.ID] != 1 {
			return directCodingProgram{}, fmt.Errorf(
				"Go requirement %s has %d implementation owners for runtime capabilities",
				requirement.ID, implementationByRequirement[requirement.ID],
			)
		}
	}
	stack, err := directCodingProjectStackByID(program.StackID)
	if err != nil {
		return directCodingProgram{}, err
	}
	if err := stack.ValidateBlueprint(bound.Source); err != nil {
		return directCodingProgram{}, fmt.Errorf(
			"validate Go runtime capability source blueprint: %w", err,
		)
	}
	if err := validateDirectCodingApplicationSourceOwnership(bound.Workload, bound.Source); err != nil {
		return directCodingProgram{}, err
	}
	if err := stack.ValidateSourceOwnership(bound.Workload, bound.Source); err != nil {
		return directCodingProgram{}, err
	}
	return bound, nil
}

func cloneDirectCodingRuntimeCapabilityBlueprint(
	blueprint assemblyline.SourceBlueprint,
) assemblyline.SourceBlueprint {
	cloned := assemblyline.SourceBlueprint{
		Documents: append([]assemblyline.SourceDocument(nil), blueprint.Documents...),
	}
	for documentIndex := range cloned.Documents {
		document := &cloned.Documents[documentIndex]
		document.Blocks = append([]assemblyline.SourceBlock(nil), document.Blocks...)
		document.ScopedPreambles = append(
			[]assemblyline.SourcePreamble(nil), document.ScopedPreambles...,
		)
		for blockIndex := range document.Blocks {
			block := &document.Blocks[blockIndex]
			block.DependsOn = append([]string(nil), block.DependsOn...)
			block.Capabilities = append([]string(nil), block.Capabilities...)
			block.Globals = append([]string(nil), block.Globals...)
		}
	}
	return cloned
}

func rejectDirectCodingGoRuntimeCapabilityAuthority(
	block *assemblyline.SourceBlock,
	registered map[string]directCodingGoStandardLibraryCapability,
) error {
	for _, ids := range [][]string{block.DependsOn, block.Capabilities} {
		for _, capabilityID := range ids {
			if _, exists := registered[capabilityID]; exists {
				return fmt.Errorf(
					"Go base source block %s already has runtime capability %s",
					block.ID, capabilityID,
				)
			}
		}
	}
	return nil
}

func insertDirectCodingGoRuntimeCapabilityIDs(
	blockID string,
	existing []string,
	selected []string,
) ([]string, error) {
	if len(existing) == 0 || existing[0] != "runtime.api" {
		return nil, fmt.Errorf(
			"Go implementation %s must directly depend on runtime.api before capability binding",
			blockID,
		)
	}
	bound := make([]string, 0, len(existing)+len(selected))
	bound = append(bound, "runtime.api")
	bound = append(bound, selected...)
	bound = append(bound, existing[1:]...)
	return bound, nil
}
