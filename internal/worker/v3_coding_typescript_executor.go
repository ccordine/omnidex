package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingTypeScriptProjectStageExecutor struct {
	session               *directCodingSession
	workspace             *directCodingTypeScriptStageWorkspace
	publicSurfaceBindings map[string]directCodingBrowserPublicSurfaceBinding
}

func newDirectCodingTypeScriptSourceGenerator(
	session *directCodingSession,
	program directCodingProgram,
) (directCodingProjectSourceGenerator, error) {
	if session == nil {
		return nil, fmt.Errorf("TypeScript source generation requires one coding session")
	}
	workspace, err := newDirectCodingTypeScriptStageWorkspace(session, program)
	if err != nil {
		return nil, err
	}
	return &directCodingTypeScriptProjectStageExecutor{
		session: session, workspace: workspace,
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
			"TypeScript source generator cannot generate adapter %q block %s",
			ref.Document.AdapterID, ref.Block.ID,
		)
	}
	var publicSurface *assemblyline.FragmentPublicInteractionSurface
	var validateInitialCandidate func(string) error
	switch ref.Block.Role {
	case assemblyline.SourceBlockTaskImplementation:
		validateInitialCandidate = validateDirectCodingBrowserPublicInteractionCandidate
	case assemblyline.SourceBlockTaskVerification:
		var err error
		publicSurface, validateInitialCandidate, err = executor.bindBrowserPublicSurface(
			context, stage, ref,
		)
		if err != nil {
			return "", err
		}
	case assemblyline.SourceBlockTaskRepresentation:
	default:
		return "", fmt.Errorf("TypeScript source generator cannot build task role %q", ref.Block.Role)
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
	return executor.workspace.Verify(
		stage, directCodingVerificationPhaseIsolatedTask, commands,
		func(current *directCodingProgram) error {
			return executor.validateTaskBrowserPublicSurface(current, context.Task.TaskID)
		},
	)
}

func (executor *directCodingTypeScriptProjectStageExecutor) VerifyFinal(
	program *directCodingProgram,
) error {
	return executor.workspace.Verify(
		program, directCodingVerificationPhaseIsolatedFinal,
		directCodingFullTypeScriptStageCommands(),
		executor.validateAllBrowserPublicSurfaces,
	)
}

func (executor *directCodingTypeScriptProjectStageExecutor) Close() error {
	if executor == nil || executor.workspace == nil {
		return nil
	}
	return executor.workspace.Close()
}
