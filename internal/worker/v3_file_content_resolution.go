package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

// resolveDirectCodingFileContents gives each accepted tree leaf its own
// content-scoping call. The tree model never receives or returns this data.
func resolveDirectCodingFileContents(
	runtime typedWorkerRuntime,
	modelName string,
	correctionModel string,
	root string,
	specification assemblyline.ApplicationSpecification,
	tree assemblyline.TargetTree,
) (assemblyline.TargetTree, error) {
	stack, err := directCodingProjectStackByID(tree.StackID)
	if err != nil {
		return assemblyline.TargetTree{}, err
	}
	requirements := make([]assemblyline.FileContentRequirement, len(specification.Requirements))
	for index, requirement := range specification.Requirements {
		requirements[index] = assemblyline.FileContentRequirement{ID: requirement.ID, Statement: requirement.SourceQuote}
	}
	if contents, forced, err := directCodingForcedFileContents(stack, tree.Paths, requirements); err != nil {
		return assemblyline.TargetTree{}, err
	} else if forced {
		tree.Contents = contents
		return tree, nil
	}
	if runtime.Context == nil || runtime.Execute == nil {
		return assemblyline.TargetTree{}, fmt.Errorf("file-content resolution requires a portable execution runtime")
	}
	modelName = strings.TrimSpace(modelName)
	correctionModel = strings.TrimSpace(correctionModel)
	if modelName == "" || correctionModel == "" {
		return assemblyline.TargetTree{}, fmt.Errorf("file-content resolution requires configured initial and correction models")
	}
	contents := make([]assemblyline.TargetTreeFileContent, 0, len(tree.Paths))
	for _, filePath := range tree.Paths {
		kind, err := directCodingTargetTreeFileKind(stack, filePath)
		if err != nil {
			return assemblyline.TargetTree{}, err
		}
		existingSource, err := directCodingTargetTreeExistingSource(root, filePath)
		if err != nil {
			return assemblyline.TargetTree{}, err
		}
		input := assemblyline.FileContentInput{
			Objective: specification.ProductQuote, Path: filePath, Kind: kind,
			Requirements: requirements, ExistingSource: existingSource,
		}
		content, err := resolveDirectCodingFileContent(runtime, modelName, correctionModel, input)
		if err != nil {
			return assemblyline.TargetTree{}, fmt.Errorf("resolve file content for %s: %w", filePath, err)
		}
		contents = append(contents, content)
	}
	tree.Contents = contents
	for _, requirement := range specification.Requirements {
		if _, err := tree.RequirementFiles(requirement.ID); err != nil {
			return assemblyline.TargetTree{}, err
		}
	}
	return tree, nil
}

// directCodingForcedFileContents resolves only the topology that has one
// possible requirement binding: exactly one implementation leaf and exactly
// one verification leaf. Asking a model to choose in that state is illegal;
// each accepted requirement must bind to the sole leaf of each required kind.
func directCodingForcedFileContents(
	stack directCodingProjectStack,
	paths []string,
	requirements []assemblyline.FileContentRequirement,
) ([]assemblyline.TargetTreeFileContent, bool, error) {
	if len(paths) != 2 || len(requirements) == 0 {
		return nil, false, nil
	}
	var implementationPath string
	var verificationPath string
	for _, filePath := range paths {
		_, kind, err := directCodingArtifactAdapterForTreePath(stack, filePath)
		if err != nil {
			return nil, false, err
		}
		switch kind {
		case assemblyline.TargetArtifactImplementation:
			if implementationPath != "" {
				return nil, false, nil
			}
			implementationPath = filePath
		case assemblyline.TargetArtifactVerification:
			if verificationPath != "" {
				return nil, false, nil
			}
			verificationPath = filePath
		default:
			return nil, false, fmt.Errorf("target-tree file %q has unsupported artifact kind %q", filePath, kind)
		}
	}
	if implementationPath == "" || verificationPath == "" {
		return nil, false, nil
	}
	ids := make([]string, len(requirements))
	for index, requirement := range requirements {
		ids[index] = requirement.ID
	}
	return []assemblyline.TargetTreeFileContent{
		{Path: implementationPath, Kind: assemblyline.TargetArtifactImplementation, RequirementIDs: append([]string(nil), ids...)},
		{Path: verificationPath, Kind: assemblyline.TargetArtifactVerification, RequirementIDs: append([]string(nil), ids...)},
	}, true, nil
}

// directCodingTargetTreeFileKind is adapter-owned syntax classification, not a
// model decision. The selected project stack constrains which registered
// artifact adapters may own a returned leaf.
func directCodingTargetTreeFileKind(
	stack directCodingProjectStack,
	path string,
) (assemblyline.TargetArtifactKind, error) {
	_, kind, err := directCodingArtifactAdapterForTreePath(stack, path)
	return kind, err
}

func resolveDirectCodingFileContent(
	runtime typedWorkerRuntime,
	modelName string,
	correctionModel string,
	input assemblyline.FileContentInput,
) (assemblyline.TargetTreeFileContent, error) {
	var lastCandidate string
	var lastFailure error
	seen := map[string]struct{}{}
	for attempt := 1; attempt <= runtime.MaxAttempts; attempt++ {
		attemptInput := input
		dispatchModel := modelName
		if lastFailure != nil {
			attemptInput.Correction = &assemblyline.FileContentCorrection{
				CandidateJSON: lastCandidate, Failure: trimForBudget(lastFailure.Error(), 1200),
			}
			dispatchModel = correctionModel
		}
		job, err := assemblyline.NewFileContentJob(attemptInput)
		if err != nil {
			return assemblyline.TargetTreeFileContent{}, err
		}
		result, err := runtime.Execute(job, dispatchModel)
		if err != nil {
			return assemblyline.TargetTreeFileContent{}, err
		}
		if err := result.ValidateFor(job); err != nil {
			return assemblyline.TargetTreeFileContent{}, finalizeTypedWorkerResult(runtime, job, result, err)
		}
		candidate := strings.TrimSpace(result.Candidate)
		if _, duplicate := seen[candidate]; duplicate {
			err := fmt.Errorf("file-content replacement made no semantic progress")
			return assemblyline.TargetTreeFileContent{}, finalizeTypedWorkerResult(runtime, job, result, err)
		}
		seen[candidate] = struct{}{}
		content, validationErr := assemblyline.DecodeFileContentCandidate(input, candidate)
		if validationErr == nil {
			if err := finalizeTypedWorkerResult(runtime, job, result, nil); err != nil {
				return assemblyline.TargetTreeFileContent{}, err
			}
			return content, nil
		}
		if err := persistTargetTreeRejection(runtime, job, result, validationErr); err != nil {
			return assemblyline.TargetTreeFileContent{}, err
		}
		lastCandidate, lastFailure = candidate, validationErr
	}
	return assemblyline.TargetTreeFileContent{}, fmt.Errorf("file-content candidate failed %d bounded replacement attempts: %w", runtime.MaxAttempts, lastFailure)
}
