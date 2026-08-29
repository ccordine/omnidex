package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingTypeScriptProjectStageExecutor struct {
	session   *directCodingSession
	workspace *directCodingTypeScriptStageWorkspace
	progress  *directCodingTypeScriptCorrectionProgress
}

func newDirectCodingTypeScriptProjectStageExecutor(
	session *directCodingSession,
	program directCodingProgram,
) (directCodingProjectStageExecutor, error) {
	if session == nil {
		return nil, fmt.Errorf("TypeScript project stage requires one coding session")
	}
	workspace, err := newDirectCodingTypeScriptStageWorkspace(session.runtime.ctx, program)
	if err != nil {
		return nil, err
	}
	return &directCodingTypeScriptProjectStageExecutor{
		session: session, workspace: workspace,
		progress: newDirectCodingTypeScriptCorrectionProgress(),
	}, nil
}

func (executor *directCodingTypeScriptProjectStageExecutor) GenerateBlock(
	context assemblyline.ApplicationTaskContext,
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (string, error) {
	if ref.Document.AdapterID != "typescript" && ref.Document.AdapterID != "typescript_react" {
		return "", fmt.Errorf(
			"TypeScript project stage cannot generate adapter %q block %s",
			ref.Document.AdapterID, ref.Block.ID,
		)
	}
	return executor.session.generateDirectCodingApplicationTaskBlock(context, stage, ref)
}

func (executor *directCodingTypeScriptProjectStageExecutor) VerifyTask(
	context assemblyline.ApplicationTaskContext,
	stage *directCodingProgram,
) error {
	commands, err := directCodingApplicationTaskStageCommands(*stage, context)
	if err != nil {
		return err
	}
	return executor.session.stageTypeScriptProgramIn(
		executor.workspace.Root(), stage, commands, executor.progress,
	)
}

func (executor *directCodingTypeScriptProjectStageExecutor) VerifyFinal(
	program *directCodingProgram,
) error {
	return executor.session.stageTypeScriptProgramIn(
		executor.workspace.Root(), program, directCodingFullStageCommands(), executor.progress,
	)
}

func (executor *directCodingTypeScriptProjectStageExecutor) Close() error {
	if executor != nil && executor.workspace != nil {
		return executor.workspace.Close()
	}
	return nil
}
