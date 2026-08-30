package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingTypeScriptProjectStageExecutor struct {
	session               *directCodingSession
	workspace             *directCodingTypeScriptStageWorkspace
	progress              *directCodingTypeScriptCorrectionProgress
	publicSurfaceBindings map[string]directCodingBrowserPublicSurfaceBinding
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
		progress:              newDirectCodingTypeScriptCorrectionProgress(),
		publicSurfaceBindings: make(map[string]directCodingBrowserPublicSurfaceBinding),
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
	var publicSurface *assemblyline.FragmentPublicInteractionSurface
	var validateInitialCandidate func(string) error
	if ref.Block.Role == assemblyline.SourceBlockTaskImplementation {
		permittedRuntimeCalls, err := directCodingBrowserHostCallsForBlock(ref.Block)
		if err != nil {
			return "", err
		}
		validateInitialCandidate = func(candidate string) error {
			return validateDirectCodingBrowserPublicInteractionCandidateWithRuntimeCalls(
				candidate, permittedRuntimeCalls,
			)
		}
	} else if ref.Block.Role == assemblyline.SourceBlockTaskVerification {
		var err error
		publicSurface, validateInitialCandidate, err = executor.bindBrowserPublicSurface(
			context, stage, ref,
		)
		if err != nil {
			return "", err
		}
	}
	source, err := executor.session.generateDirectCodingApplicationTaskBlock(
		context, stage, ref, publicSurface, validateInitialCandidate,
	)
	if err != nil {
		return "", err
	}
	if ref.Block.Role == assemblyline.SourceBlockTaskImplementation {
		return executor.closeImplementationBeforeVerification(stage, ref, source)
	}
	return source, nil
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
		func(current *directCodingProgram) error {
			return executor.validateTaskBrowserPublicSurface(current, context.Task.TaskID)
		},
	)
}

func (executor *directCodingTypeScriptProjectStageExecutor) VerifyFinal(
	program *directCodingProgram,
) error {
	return executor.session.stageTypeScriptProgramIn(
		executor.workspace.Root(), program, directCodingFullStageCommands(), executor.progress,
		executor.validateAllBrowserPublicSurfaces,
	)
}

func (executor *directCodingTypeScriptProjectStageExecutor) Close() error {
	if executor != nil && executor.workspace != nil {
		return executor.workspace.Close()
	}
	return nil
}
