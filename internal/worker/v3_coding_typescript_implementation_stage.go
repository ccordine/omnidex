package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func (executor *directCodingTypeScriptProjectStageExecutor) closeImplementationBeforeVerification(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	source string,
) (string, error) {
	return closeDirectCodingTypeScriptImplementation(
		stage,
		ref.Block.TaskID,
		ref.Block.ID,
		source,
		func(projection *directCodingProgram, validators ...func(*directCodingProgram) error) error {
			return executor.workspace.Verify(
				projection,
				directCodingVerificationPhaseIsolatedImplementation,
				directCodingImplementationStageCommands(),
				validators...,
			)
		},
	)
}

func closeDirectCodingTypeScriptImplementation(
	stage *directCodingProgram,
	taskID string,
	implementationID string,
	source string,
	runStage func(*directCodingProgram, ...func(*directCodingProgram) error) error,
) (string, error) {
	if runStage == nil {
		return "", fmt.Errorf("TypeScript implementation closure requires one compiler stage")
	}
	projection, err := projectDirectCodingTypeScriptImplementationStage(
		stage, taskID, implementationID,
	)
	if err != nil {
		return "", err
	}
	projection.Generated[implementationID] = source
	validate := func(current *directCodingProgram) error {
		if current == nil {
			return fmt.Errorf("TypeScript implementation closure lost projected state")
		}
		candidate := strings.TrimSpace(current.Generated[implementationID])
		if candidate == "" {
			return fmt.Errorf(
				"TypeScript implementation closure omitted block %s", implementationID,
			)
		}
		binding, bindingErr := deriveDirectCodingBrowserPublicSurfaceBinding(current, taskID)
		if bindingErr != nil {
			return fmt.Errorf(
				"revalidate TypeScript implementation closure %s: %w",
				implementationID, bindingErr,
			)
		}
		if binding.implementationBlockID != implementationID {
			return fmt.Errorf(
				"TypeScript implementation closure resolved block %s instead of %s",
				binding.implementationBlockID, implementationID,
			)
		}
		return nil
	}
	if err := runStage(&projection, validate); err != nil {
		return "", fmt.Errorf(
			"close TypeScript implementation %s before verification generation: %w",
			implementationID, err,
		)
	}
	if err := validate(&projection); err != nil {
		return "", fmt.Errorf(
			"validate compiler-closed TypeScript implementation %s: %w",
			implementationID, err,
		)
	}
	return strings.TrimSpace(projection.Generated[implementationID]), nil
}

func projectDirectCodingTypeScriptImplementationStage(
	stage *directCodingProgram,
	taskID string,
	implementationID string,
) (directCodingProgram, error) {
	if stage == nil || strings.TrimSpace(taskID) == "" || strings.TrimSpace(implementationID) == "" {
		return directCodingProgram{}, fmt.Errorf(
			"TypeScript implementation closure requires one stage, task, and implementation",
		)
	}
	expectedImplementationID, err := directCodingTaskBlockIDByRole(
		stage.Source, taskID, assemblyline.SourceBlockTaskImplementation,
	)
	if err != nil {
		return directCodingProgram{}, err
	}
	if implementationID != expectedImplementationID {
		return directCodingProgram{}, fmt.Errorf(
			"TypeScript implementation closure block %s differs from task %s implementation %s",
			implementationID, taskID, expectedImplementationID,
		)
	}
	verificationID, err := directCodingTaskBlockIDByRole(
		stage.Source, taskID, assemblyline.SourceBlockTaskVerification,
	)
	if err != nil {
		return directCodingProgram{}, err
	}
	excluded := map[string]struct{}{verificationID: {}}
	changed := true
	for changed {
		changed = false
		for _, document := range stage.Source.Documents {
			for _, block := range document.Blocks {
				if _, alreadyExcluded := excluded[block.ID]; alreadyExcluded {
					continue
				}
				for _, dependencyID := range block.DependsOn {
					if _, dependsOnExcluded := excluded[dependencyID]; dependsOnExcluded {
						excluded[block.ID] = struct{}{}
						changed = true
						break
					}
				}
			}
		}
	}
	documents := make([]assemblyline.SourceDocument, 0, len(stage.Source.Documents))
	retained := make(map[string]struct{})
	for _, document := range stage.Source.Documents {
		blocks := make([]assemblyline.SourceBlock, 0, len(document.Blocks))
		for _, block := range document.Blocks {
			if _, remove := excluded[block.ID]; remove {
				continue
			}
			if block.Generated() && block.ID != implementationID &&
				strings.TrimSpace(stage.Generated[block.ID]) == "" {
				return directCodingProgram{}, fmt.Errorf(
					"TypeScript implementation closure retained unresolved generated block %s",
					block.ID,
				)
			}
			blocks = append(blocks, block)
			retained[block.ID] = struct{}{}
		}
		if len(blocks) == 0 {
			continue
		}
		document.Blocks = blocks
		documents = append(documents, document)
	}
	if _, exists := retained[implementationID]; !exists {
		return directCodingProgram{}, fmt.Errorf(
			"TypeScript implementation closure removed implementation %s", implementationID,
		)
	}
	projection := *stage
	staticFiles, err := cloneValidatedDirectCodingStaticFiles(
		stage.Project.Stack, stage.StaticFiles,
	)
	if err != nil {
		return directCodingProgram{}, fmt.Errorf(
			"project TypeScript implementation static-file authority: %w", err,
		)
	}
	projection.StaticFiles = staticFiles
	projection.Source = assemblyline.SourceBlueprint{Documents: documents}
	pair, err := directCodingTaskSinglePair(stage.Coverage, taskID)
	if err != nil {
		return directCodingProgram{}, err
	}
	projection.TargetTree.Paths = nil
	for _, targetPath := range stage.TargetTree.Paths {
		if targetPath != pair.VerificationPath {
			projection.TargetTree.Paths = append(projection.TargetTree.Paths, targetPath)
		}
	}
	projection.Generated = make(map[string]string)
	for blockID, generatedSource := range stage.Generated {
		if _, exists := retained[blockID]; exists && strings.TrimSpace(generatedSource) != "" {
			projection.Generated[blockID] = generatedSource
		}
	}
	if err := projection.Source.Validate(); err != nil {
		return directCodingProgram{}, fmt.Errorf(
			"validate TypeScript implementation closure: %w", err,
		)
	}
	return projection, nil
}
