package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func bindDirectCodingSourceBlueprintAdapters(
	stack directCodingProjectStack,
	blueprint assemblyline.SourceBlueprint,
) (assemblyline.SourceBlueprint, error) {
	bound := assemblyline.SourceBlueprint{
		Documents: append([]assemblyline.SourceDocument(nil), blueprint.Documents...),
	}
	for index := range bound.Documents {
		document := &bound.Documents[index]
		adapter, _, err := directCodingArtifactAdapterForProjectPath(stack, document.Path)
		if err != nil {
			return assemblyline.SourceBlueprint{}, fmt.Errorf("bind source document %s: %w", document.ID, err)
		}
		if adapter.ComposeDocument == nil {
			return assemblyline.SourceBlueprint{}, fmt.Errorf(
				"artifact adapter %s cannot compose bounded source documents", adapter.ID,
			)
		}
		if document.AdapterID != "" && document.AdapterID != adapter.ID {
			return assemblyline.SourceBlueprint{}, fmt.Errorf(
				"source document %s claims adapter %s but path %q resolves to %s",
				document.ID, document.AdapterID, document.Path, adapter.ID,
			)
		}
		document.AdapterID = adapter.ID
	}
	if err := stack.ValidateBlueprint(bound); err != nil {
		return assemblyline.SourceBlueprint{}, fmt.Errorf("validate %s source blueprint: %w", stack.ID, err)
	}
	return bound, nil
}

func composeDirectCodingSourceProgram(
	program directCodingProgram,
) ([]assemblyline.ComposedSourceDocument, error) {
	if len(program.Source.Documents) == 0 {
		return nil, nil
	}
	stack, err := directCodingProjectStackByID(program.StackID)
	if err != nil {
		return nil, err
	}
	if err := stack.ValidateBlueprint(program.Source); err != nil {
		return nil, fmt.Errorf("validate %s source blueprint: %w", stack.ID, err)
	}
	composition := assemblyline.SourceComposition{
		Generated:  make(map[string]string, len(program.Generated)),
		Interfaces: make(map[string]string),
	}
	for blockID, source := range program.Generated {
		composition.Generated[blockID] = source
	}
	for _, document := range program.Source.Documents {
		for _, block := range document.Blocks {
			composition.Interfaces[block.ID] = block.API
		}
	}
	composed := make([]assemblyline.ComposedSourceDocument, 0, len(program.Source.Documents))
	for _, document := range program.Source.Documents {
		adapter, _, err := directCodingArtifactAdapterForProjectPath(stack, document.Path)
		if err != nil {
			return nil, fmt.Errorf("compose source document %s: %w", document.ID, err)
		}
		if document.AdapterID == "" || document.AdapterID != adapter.ID {
			return nil, fmt.Errorf(
				"source document %s adapter identity %q differs from resolved adapter %s",
				document.ID, document.AdapterID, adapter.ID,
			)
		}
		if adapter.ComposeDocument == nil {
			return nil, fmt.Errorf("artifact adapter %s cannot compose source documents", adapter.ID)
		}
		source, err := adapter.ComposeDocument(document, composition)
		if err != nil {
			return nil, fmt.Errorf("compose source document %s with adapter %s: %w", document.ID, adapter.ID, err)
		}
		composed = append(composed, source)
	}
	return composed, nil
}

func validateDirectCodingApplicationSourceOwnership(
	workload assemblyline.FrozenApplicationWorkload,
	blueprint assemblyline.SourceBlueprint,
) error {
	tasks := make(map[string]struct{}, len(workload.Tasks))
	generated := make(map[string]int, len(workload.Tasks))
	for _, task := range workload.Tasks {
		tasks[task.ID] = struct{}{}
	}
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.TaskID == "" {
				continue
			}
			if _, exists := tasks[block.TaskID]; !exists {
				return fmt.Errorf("source block %s names unknown task %s", block.ID, block.TaskID)
			}
			if block.Generated() {
				generated[block.TaskID]++
			}
		}
	}
	for _, task := range workload.Tasks {
		if generated[task.ID] == 0 {
			return fmt.Errorf("task %s requires at least one owned generated source node", task.ID)
		}
	}
	return nil
}

func validateDirectCodingSingleImplementationSourceOwnership(
	workload assemblyline.FrozenApplicationWorkload,
	blueprint assemblyline.SourceBlueprint,
) error {
	implementation := make(map[string]int, len(workload.Tasks))
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			switch block.Role {
			case assemblyline.SourceBlockTaskImplementation:
				implementation[block.TaskID]++
			case assemblyline.SourceBlockTaskVerification:
				return fmt.Errorf("source block %s is an obsolete generated verification artifact", block.ID)
			}
		}
	}
	for _, task := range workload.Tasks {
		if implementation[task.ID] != 1 {
			return fmt.Errorf(
				"task %s requires exactly one generated implementation source node", task.ID,
			)
		}
	}
	return nil
}

func validateDirectCodingSinglePairSourceOwnership(
	workload assemblyline.FrozenApplicationWorkload,
	blueprint assemblyline.SourceBlueprint,
) error {
	implementation := make(map[string]int, len(workload.Tasks))
	verification := make(map[string]int, len(workload.Tasks))
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			switch block.Role {
			case assemblyline.SourceBlockTaskImplementation:
				implementation[block.TaskID]++
			case assemblyline.SourceBlockTaskVerification:
				verification[block.TaskID]++
			}
		}
	}
	for _, task := range workload.Tasks {
		if implementation[task.ID] != 1 || verification[task.ID] != 1 {
			return fmt.Errorf(
				"task %s requires exactly one generated implementation and verification source node", task.ID,
			)
		}
	}
	return nil
}

func directCodingTaskGeneratedBlockRefs(
	blueprint assemblyline.SourceBlueprint,
	taskID string,
) ([]assemblyline.SourceBlockRef, error) {
	waves, err := blueprint.BuildWaves()
	if err != nil {
		return nil, err
	}
	blocks := make([]assemblyline.SourceBlockRef, 0)
	for _, wave := range waves {
		for _, ref := range wave {
			if ref.Block.TaskID != taskID || !ref.Block.Generated() {
				continue
			}
			blocks = append(blocks, ref)
			switch ref.Block.Role {
			case assemblyline.SourceBlockTaskImplementation,
				assemblyline.SourceBlockTaskRepresentation:
			case assemblyline.SourceBlockTaskVerification:
				return nil, fmt.Errorf(
					"task %s contains obsolete generated verification block %s", taskID, ref.Block.ID,
				)
			default:
				return nil, fmt.Errorf("generated task block %s has role %q", ref.Block.ID, ref.Block.Role)
			}
		}
	}
	if len(blocks) == 0 {
		return nil, fmt.Errorf("task %s lacks generated source", taskID)
	}
	return blocks, nil
}

func directCodingTaskBlockIDByRole(
	blueprint assemblyline.SourceBlueprint,
	taskID string,
	role assemblyline.SourceBlockRole,
) (string, error) {
	matched := ""
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.TaskID != taskID || block.Role != role {
				continue
			}
			if matched != "" {
				return "", fmt.Errorf("task %s has multiple %s source blocks", taskID, role)
			}
			matched = block.ID
		}
	}
	if matched == "" {
		return "", fmt.Errorf("task %s has no %s source block", taskID, role)
	}
	return matched, nil
}

func directCodingTaskStageBlockIDs(
	blueprint assemblyline.SourceBlueprint,
	taskID string,
) (map[string]struct{}, error) {
	byID := make(map[string]assemblyline.SourceBlock)
	included := make(map[string]struct{})
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			byID[block.ID] = block
			if block.TaskID == taskID {
				included[block.ID] = struct{}{}
			}
		}
	}
	if len(included) == 0 {
		return nil, fmt.Errorf("task %s owns no source blocks", taskID)
	}
	pending := make([]string, 0, len(included))
	for blockID := range included {
		pending = append(pending, blockID)
	}
	for len(pending) > 0 {
		blockID := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		for _, dependencyID := range byID[blockID].DependsOn {
			dependency, exists := byID[dependencyID]
			if !exists {
				return nil, fmt.Errorf("task %s source block %s has unknown dependency %s", taskID, blockID, dependencyID)
			}
			if dependency.TaskID != "" && dependency.TaskID != taskID {
				return nil, fmt.Errorf("task %s source stage crosses into task %s through block %s", taskID, dependency.TaskID, dependencyID)
			}
			if _, exists := included[dependencyID]; exists {
				continue
			}
			included[dependencyID] = struct{}{}
			pending = append(pending, dependencyID)
		}
	}
	return included, nil
}
