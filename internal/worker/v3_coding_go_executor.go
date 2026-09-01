package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingGoProjectStageExecutor struct {
	generator *directCodingLanguageSourceGenerator
	workspace *directCodingGoStageWorkspace
}

func newDirectCodingGoSourceGenerator(
	session *directCodingSession,
	program directCodingProgram,
) (_ directCodingProjectSourceGenerator, resultErr error) {
	generated, err := newDirectCodingLanguageSourceGeneratorForProgram(session, program)
	if err != nil {
		return nil, err
	}
	generator, ok := generated.(*directCodingLanguageSourceGenerator)
	if !ok {
		return nil, fmt.Errorf("Go source generation requires the bounded language generator")
	}
	workspace, err := newDirectCodingGoStageWorkspace(session, program)
	if err != nil {
		return nil, err
	}
	return &directCodingGoProjectStageExecutor{
		generator: generator, workspace: workspace,
	}, nil
}

func (executor *directCodingGoProjectStageExecutor) GenerateBlock(
	context assemblyline.ApplicationTaskContext,
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (string, error) {
	if executor == nil || executor.generator == nil {
		return "", fmt.Errorf("Go source generation requires one active bounded generator")
	}
	return generateDirectCodingGoBlock(executor.generator, context, stage, ref)
}

func (executor *directCodingGoProjectStageExecutor) VerifyTask(
	context assemblyline.ApplicationTaskContext,
	stage *directCodingProgram,
) error {
	if executor == nil || executor.workspace == nil {
		return fmt.Errorf("Go task verification requires one active isolated workspace")
	}
	testName, err := directCodingGoTaskAcceptanceName(*stage, context.Task.TaskID)
	if err != nil {
		return err
	}
	return executor.workspace.VerifyTask(stage, context, testName)
}

func (executor *directCodingGoProjectStageExecutor) VerifyFinal(
	program *directCodingProgram,
) error {
	if executor == nil || executor.workspace == nil {
		return fmt.Errorf("Go final verification requires one active isolated workspace")
	}
	return executor.workspace.VerifyFinal(program)
}

func (executor *directCodingGoProjectStageExecutor) Close() error {
	if executor == nil || executor.workspace == nil {
		return nil
	}
	return executor.workspace.Close()
}
