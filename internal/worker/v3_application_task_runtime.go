package worker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func (s *directCodingSession) generateDirectCodingApplicationTaskBlock(
	_ assemblyline.ApplicationTaskContext,
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (string, error) {
	job, err := directCodingApplicationTaskFragmentJob(stage, ref)
	if err != nil {
		return "", err
	}
	modelName, err := s.workerModel(station.CodingFragment)
	if err != nil {
		return "", err
	}
	runtime := directCodingWorkerRuntime(s)
	return generateDirectCodingTypeScriptBlockWithRuntime(
		runtime, modelName, s.typeScriptRepairModels, s.typeScriptRepairEvents(), job,
	)
}

func directCodingApplicationTaskFragmentJob(
	stage *directCodingProgram,
	ref assemblyline.SourceBlockRef,
) (directCodingTypeScriptFragmentJob, error) {
	if stage == nil {
		return directCodingTypeScriptFragmentJob{}, fmt.Errorf("application task generation requires one isolated stage")
	}
	profile, err := directCodingVersionProfileForProgram(*stage)
	if err != nil {
		return directCodingTypeScriptFragmentJob{}, err
	}
	declarations := make(map[string]string)
	block := ref.Block
	tsx := directCodingTypeScriptDocumentIsTSX(ref.Document)
	found := false
	for _, document := range stage.Source.Documents {
		for _, candidate := range document.Blocks {
			if !candidate.Generated() || strings.TrimSpace(stage.Generated[candidate.ID]) != "" {
				declarations[candidate.ID] = strings.TrimSpace(candidate.API)
			}
			if candidate.ID == block.ID {
				if document.ID != ref.Document.ID || document.AdapterID != ref.Document.AdapterID {
					return directCodingTypeScriptFragmentJob{}, fmt.Errorf("source block %s reference differs from isolated stage", block.ID)
				}
				found = true
			}
		}
	}
	if !found || !block.Generated() {
		return directCodingTypeScriptFragmentJob{}, fmt.Errorf("application task generation block %s is not an isolated generated block", block.ID)
	}
	available, err := directCodingTypeScriptAvailableDeclarations(block, declarations)
	if err != nil {
		return directCodingTypeScriptFragmentJob{}, err
	}
	return directCodingTypeScriptFragmentJob{
		block: block, dialect: profile.SourceDialect, tsx: tsx, available: available,
	}, nil
}

func directCodingApplicationTaskStageCommands(
	stage directCodingProgram,
	context assemblyline.ApplicationTaskContext,
) ([][]string, error) {
	acceptanceID, err := directCodingTaskBlockIDByRole(
		stage.Source, context.Task.TaskID, assemblyline.SourceBlockTaskVerification,
	)
	if err != nil {
		return nil, err
	}
	path := ""
	for _, document := range stage.Source.Documents {
		for _, block := range document.Blocks {
			if block.ID == acceptanceID {
				if path != "" {
					return nil, fmt.Errorf("application task stage repeats acceptance block %s", acceptanceID)
				}
				path = document.Path
			}
		}
	}
	if path == "" {
		return nil, fmt.Errorf("application task stage lacks acceptance block %s", acceptanceID)
	}
	return [][]string{{"run", "typecheck"}, directCodingStructuredVitestCommand(path)}, nil
}

func (s *directCodingSession) runDirectCodingApplicationTaskLifecycle(
	input assemblyline.ApplicationWorkloadDraftInput,
	frozen assemblyline.FrozenApplicationWorkload,
	program *directCodingProgram,
) error {
	if program == nil {
		return fmt.Errorf("application task lifecycle requires one program")
	}
	if _, err := directCodingVersionProfileForProgram(*program); err != nil {
		return err
	}
	stack, err := directCodingProjectStackByID(program.StackID)
	if err != nil {
		return err
	}
	executor, err := stack.NewStageExecutor(s, *program)
	if err != nil {
		return err
	}
	lifecycleErr := runDirectCodingApplicationTaskLifecycle(
		input, frozen, program,
		directCodingApplicationTaskLifecycleHooks{
			BeginTask: func(context assemblyline.ApplicationTaskContext) error {
				if s.cognition == nil {
					return fmt.Errorf("application task lifecycle requires persisted task cognition")
				}
				return s.cognition.Begin(context.Task.TaskID)
			},
			BuildBlock: executor.GenerateBlock,
			VerifyTask: executor.VerifyTask,
			CompleteTask: func(context assemblyline.ApplicationTaskContext, generated map[string]string) error {
				return s.cognition.CompleteTask(context.Task.TaskID, generated)
			},
			FinalStage: func(complete *directCodingProgram) error {
				return executor.VerifyFinal(complete)
			},
		},
	)
	closeErr := executor.Close()
	if lifecycleErr != nil || closeErr != nil {
		return errors.Join(lifecycleErr, closeErr)
	}
	return nil
}
