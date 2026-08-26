package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func validateDirectCodingPHPAcceptance(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
	source string,
) error {
	input, err := directCodingLanguageFragmentInput(stage, ref, "php")
	if err != nil {
		return err
	}
	validated, err := validateDirectCodingPHPFragment(input, source)
	if err != nil {
		return fmt.Errorf("PHP acceptance block %s: %w", ref.Block.ID, err)
	}
	featureName, err := phpAcceptanceImplementationName(stage, ref)
	if err != nil {
		return fmt.Errorf("PHP acceptance block %s: %w", ref.Block.ID, err)
	}
	fixtureName, err := phpAcceptanceFixtureName(stage, ref)
	if err != nil {
		return fmt.Errorf("PHP acceptance block %s: %w", ref.Block.ID, err)
	}
	requiredConditions, err := phpAcceptanceCriterionCount(stage, ref)
	if err != nil {
		return fmt.Errorf("PHP acceptance block %s: %w", ref.Block.ID, err)
	}
	inspection, err := inspectPHPAcceptance(
		[]byte(validated), featureName, fixtureName, requiredConditions,
	)
	if err != nil {
		return fmt.Errorf("PHP acceptance block %s: %w", ref.Block.ID, err)
	}
	if !inspection.Called || !inspection.ShapeChecked ||
		inspection.ConditionCount < requiredConditions {
		return fmt.Errorf(
			"PHP acceptance block %s must store the exact %s(%s()) result, pass it once to RuntimeAssertions::requireResult, and provide %d distinct result-field comparisons",
			ref.Block.ID, featureName, fixtureName, requiredConditions,
		)
	}
	return nil
}

func phpAcceptanceImplementationName(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (string, error) {
	if stage == nil {
		return "", fmt.Errorf("acceptance stage is nil")
	}
	name := ""
	for _, dependencyID := range ref.Block.DependsOn {
		dependency, exists := directCodingSourceBlueprintBlock(stage.Source, dependencyID)
		if !exists || dependency.Role != assemblyline.SourceBlockTaskImplementation {
			continue
		}
		const prefix = "function "
		if !strings.HasPrefix(dependency.Signature, prefix) {
			return "", fmt.Errorf("implementation signature %q is not a PHP function", dependency.Signature)
		}
		separator := strings.IndexByte(dependency.Signature, '(')
		if separator <= len(prefix) {
			return "", fmt.Errorf("implementation signature %q has no callable name", dependency.Signature)
		}
		if name != "" {
			return "", fmt.Errorf("multiple implementation owners")
		}
		name = strings.TrimSpace(dependency.Signature[len(prefix):separator])
	}
	if name == "" {
		return "", fmt.Errorf("no implementation owner")
	}
	return name, nil
}

func phpAcceptanceFixtureName(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (string, error) {
	name := ""
	for _, dependencyID := range ref.Block.DependsOn {
		dependency, exists := directCodingSourceBlueprintBlock(stage.Source, dependencyID)
		if !exists || !strings.HasPrefix(dependency.ID, "acceptance.fixture.") {
			continue
		}
		matches := phpTopLevelAPIFunction.FindAllStringSubmatch(dependency.API, -1)
		if len(matches) != 1 || dependency.Role != assemblyline.SourceBlockTaskSupport ||
			dependency.TaskID != ref.Block.TaskID || name != "" {
			return "", fmt.Errorf("acceptance fixture dependency is not one exact task-local helper")
		}
		name = matches[0][1]
	}
	if name == "" {
		return "", fmt.Errorf("acceptance has no code-owned task input fixture")
	}
	return name, nil
}

func phpAcceptanceCriterionCount(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (int, error) {
	if stage == nil {
		return 0, fmt.Errorf("acceptance stage is nil")
	}
	for _, task := range stage.Workload.Tasks {
		if task.ID == ref.Block.TaskID {
			if len(task.AcceptanceCriteria) == 0 {
				return 0, fmt.Errorf("acceptance task has no frozen criteria")
			}
			return len(task.AcceptanceCriteria), nil
		}
	}
	return 0, fmt.Errorf("acceptance task %s is absent from frozen workload", ref.Block.TaskID)
}
