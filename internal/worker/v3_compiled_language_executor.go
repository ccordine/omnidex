package worker

import "github.com/gryph/omnidex/internal/assemblyline"

type directCodingCompiledLanguageExecutor struct {
	generator *directCodingLanguageSourceGenerator
	workspace *directCodingCompiledLanguageWorkspace
}

func newDirectCodingCompiledLanguageSourceGenerator(session *directCodingSession, program directCodingProgram) (directCodingProjectSourceGenerator, error) {
	generator, err := newDirectCodingLanguageSourceGeneratorForProgram(session, program)
	if err != nil {
		return nil, err
	}
	workspace, err := newDirectCodingCompiledLanguageWorkspace(session, program)
	if err != nil {
		return nil, err
	}
	return &directCodingCompiledLanguageExecutor{generator: generator, workspace: workspace}, nil
}

func (executor *directCodingCompiledLanguageExecutor) GenerateBlock(context assemblyline.ApplicationTaskContext, program *directCodingProgram, ref assemblyline.SourceBlockRef) (string, error) {
	return executor.generator.GenerateBlock(context, program, ref)
}

func (executor *directCodingCompiledLanguageExecutor) VerifyTask(context assemblyline.ApplicationTaskContext, program *directCodingProgram) error {
	return executor.workspace.VerifyTask(context, program)
}

func (executor *directCodingCompiledLanguageExecutor) VerifyFinal(program *directCodingProgram) error {
	return executor.workspace.VerifyFinal(program)
}

func (executor *directCodingCompiledLanguageExecutor) Close() error {
	return executor.workspace.Close()
}
