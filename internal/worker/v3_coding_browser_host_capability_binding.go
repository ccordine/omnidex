package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func bindDirectCodingBrowserHostCapabilities(
	program directCodingProgram,
	graph directCodingRuntimeCapabilityGraph,
) (directCodingProgram, error) {
	if program.StackID != genericTypeScriptBrowserAdapter {
		return directCodingProgram{}, fmt.Errorf(
			"browser host capability binding received project stack %s", program.StackID,
		)
	}
	if len(program.Generated) != 0 {
		return directCodingProgram{}, fmt.Errorf(
			"browser host capabilities must be bound before source generation",
		)
	}
	registered, err := registeredDirectCodingBrowserHostCapabilities()
	if err != nil {
		return directCodingProgram{}, err
	}
	candidates := make([]directCodingRuntimeCapability, len(registered))
	byID := make(map[string]directCodingBrowserHostCapability, len(registered))
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
	selectedCapabilities := make([]directCodingBrowserHostCapability, 0, len(selectedUnion))
	for _, capability := range registered {
		if _, selected := selectedUnion[capability.ID]; selected {
			selectedCapabilities = append(selectedCapabilities, capability)
		}
	}

	bound := program
	bound.Source = cloneDirectCodingRuntimeCapabilityBlueprint(program.Source)
	runtimeIndex := -1
	implementationByRequirement := make(map[string]int, len(requirements))
	harnessByRequirement := make(map[string]int, len(requirements))
	requirementByTask := make(map[string]string, len(program.Workload.Tasks))
	sequenceByTask := make(map[string]int, len(program.Workload.Tasks))
	importsByDocument := make(map[int]map[string]struct{})
	acceptanceDocuments := make(map[int]bool)
	for taskIndex, task := range program.Workload.Tasks {
		requirementByTask[task.ID] = task.RequirementID
		sequenceByTask[task.ID] = taskIndex + 1
	}
	for documentIndex := range bound.Source.Documents {
		document := &bound.Source.Documents[documentIndex]
		if document.ID == "application_runtime" {
			if runtimeIndex >= 0 {
				return directCodingProgram{}, fmt.Errorf("browser source repeats application runtime document")
			}
			runtimeIndex = documentIndex
			if len(document.Blocks) != 3 || document.Blocks[0].ID != "runtime.api" ||
				document.Blocks[1].ID != "runtime.host_bridge" ||
				document.Blocks[2].ID != "runtime.factory" {
				return directCodingProgram{}, fmt.Errorf(
					"browser base runtime already contains non-selected host capability source",
				)
			}
			continue
		}
		for blockIndex := range document.Blocks {
			block := &document.Blocks[blockIndex]
			if err := rejectDirectCodingBrowserHostCapabilityAuthority(block, byID); err != nil {
				return directCodingProgram{}, err
			}
			if sequence, exists := sequenceByTask[block.TaskID]; exists &&
				block.ID == fmt.Sprintf("acceptance.harness.%03d", sequence) {
				requirementID := requirementByTask[block.TaskID]
				expectedBase := genericBrowserAcceptanceHarnessSource(
					fmt.Sprintf("RunFeature%03dAcceptance", sequence),
					fmt.Sprintf("VerifyFeature%03d", sequence),
					fmt.Sprintf("Feature%03d", sequence),
					genericApplicationCapabilityID(sequence),
					nil,
				)
				if strings.TrimSpace(block.Static) != strings.TrimSpace(expectedBase) {
					return directCodingProgram{}, fmt.Errorf(
						"browser acceptance harness %s has unexpected pre-capability source", block.ID,
					)
				}
				harnessByRequirement[requirementID]++
				if _, exists := acceptanceDocuments[documentIndex]; !exists {
					acceptanceDocuments[documentIndex] = false
				}
				selected := graph[requirementID]
				if len(selected) != 0 {
					block.Static = genericBrowserAcceptanceHarnessSource(
						fmt.Sprintf("RunFeature%03dAcceptance", sequence),
						fmt.Sprintf("VerifyFeature%03d", sequence),
						fmt.Sprintf("Feature%03d", sequence),
						genericApplicationCapabilityID(sequence),
						selected,
					)
					block.DependsOn = append(block.DependsOn, "runtime.host_bridge")
					acceptanceDocuments[documentIndex] = true
				}
			}
			if block.Role != assemblyline.SourceBlockTaskImplementation {
				continue
			}
			if err := rejectDirectCodingBrowserHostCapabilitySymbols(block, registered); err != nil {
				return directCodingProgram{}, err
			}
			if importsByDocument[documentIndex] == nil {
				importsByDocument[documentIndex] = make(map[string]struct{})
			}
			requirementID, exists := requirementByTask[block.TaskID]
			if !exists {
				return directCodingProgram{}, fmt.Errorf(
					"browser implementation %s names unknown task %s", block.ID, block.TaskID,
				)
			}
			implementationByRequirement[requirementID]++
			selected := graph[requirementID]
			block.DependsOn = append(block.DependsOn, selected...)
			block.Capabilities = append(block.Capabilities, selected...)
			for _, capabilityID := range selected {
				capability := byID[capabilityID]
				for _, callName := range capability.CallNames {
					block.Policy.RequiredCalls = append(
						block.Policy.RequiredCalls,
						assemblyline.SourceCallRequirement{Callees: []string{callName}},
					)
					importsByDocument[documentIndex][callName] = struct{}{}
				}
			}
		}
	}
	if runtimeIndex < 0 {
		return directCodingProgram{}, fmt.Errorf("browser source omits application runtime document")
	}
	for _, requirement := range requirements {
		if implementationByRequirement[requirement.ID] != 1 {
			return directCodingProgram{}, fmt.Errorf(
				"browser requirement %s has %d implementation owners for host capabilities",
				requirement.ID, implementationByRequirement[requirement.ID],
			)
		}
		if harnessByRequirement[requirement.ID] != 1 {
			return directCodingProgram{}, fmt.Errorf(
				"browser requirement %s has %d acceptance harnesses for host receipts",
				requirement.ID, harnessByRequirement[requirement.ID],
			)
		}
	}
	for documentIndex, selectedCalls := range importsByDocument {
		document := &bound.Source.Documents[documentIndex]
		if strings.TrimSpace(document.Preamble) != genericBrowserFeaturePreamble(document.Path, nil) {
			return directCodingProgram{}, fmt.Errorf(
				"browser implementation document %s has unexpected pre-capability imports", document.ID,
			)
		}
		calls := make([]string, 0, len(selectedCalls))
		for _, capability := range registered {
			for _, callName := range capability.CallNames {
				if _, selected := selectedCalls[callName]; selected {
					calls = append(calls, callName)
				}
			}
		}
		document.Preamble = genericBrowserFeaturePreamble(document.Path, calls)
	}
	for documentIndex, includeHostReceiptObserver := range acceptanceDocuments {
		document := &bound.Source.Documents[documentIndex]
		runtimeModule := typeScriptRelativeModule(document.Path, "src/runtime.tsx")
		if strings.TrimSpace(document.Preamble) !=
			strings.TrimSpace(genericBrowserAcceptancePreamble(runtimeModule, false)) {
			return directCodingProgram{}, fmt.Errorf(
				"browser acceptance document %s has unexpected pre-capability imports", document.ID,
			)
		}
		document.Preamble = genericBrowserAcceptancePreamble(
			runtimeModule, includeHostReceiptObserver,
		)
	}

	runtimeDocument := genericBrowserRuntimeDocument(requirements)
	runtimeDocument.AdapterID = bound.Source.Documents[runtimeIndex].AdapterID
	runtimeDocument.Blocks = append(
		append(
			[]assemblyline.SourceBlock{runtimeDocument.Blocks[0], runtimeDocument.Blocks[1]},
			directCodingBrowserHostCapabilityBlocks(selectedCapabilities)...,
		),
		runtimeDocument.Blocks[2],
	)
	bound.Source.Documents[runtimeIndex] = runtimeDocument

	stack, err := directCodingProjectStackByID(program.StackID)
	if err != nil {
		return directCodingProgram{}, err
	}
	if err := stack.ValidateBlueprint(bound.Source); err != nil {
		return directCodingProgram{}, fmt.Errorf(
			"validate browser host capability source blueprint: %w", err,
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

func rejectDirectCodingBrowserHostCapabilitySymbols(
	block *assemblyline.SourceBlock,
	registered []directCodingBrowserHostCapability,
) error {
	registeredCalls := make(map[string]struct{})
	for _, capability := range registered {
		for _, callName := range capability.CallNames {
			registeredCalls[callName] = struct{}{}
		}
	}
	for _, global := range block.Globals {
		if _, exists := registeredCalls[global]; exists {
			return fmt.Errorf(
				"browser base implementation %s already exposes host call %s", block.ID, global,
			)
		}
	}
	for _, call := range block.Policy.RequiredCalls {
		for _, callee := range call.Callees {
			if _, exists := registeredCalls[callee]; exists {
				return fmt.Errorf(
					"browser base implementation %s already requires host call %s", block.ID, callee,
				)
			}
		}
	}
	return nil
}

func rejectDirectCodingBrowserHostCapabilityAuthority(
	block *assemblyline.SourceBlock,
	registered map[string]directCodingBrowserHostCapability,
) error {
	for _, ids := range [][]string{block.DependsOn, block.Capabilities} {
		for _, capabilityID := range ids {
			if _, exists := registered[capabilityID]; exists {
				return fmt.Errorf(
					"browser base source block %s already has host capability %s",
					block.ID, capabilityID,
				)
			}
		}
	}
	return nil
}
