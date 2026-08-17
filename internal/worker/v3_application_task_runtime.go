package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

func (s *directCodingSession) generateDirectCodingApplicationTaskBlock(
	_ assemblyline.ApplicationTaskContext,
	stage *directCodingProgram,
	block assemblyline.TypeScriptBlock,
) (string, error) {
	job, err := directCodingApplicationTaskFragmentJob(stage, block)
	if err != nil {
		return "", err
	}
	modelName, err := s.workerModel(station.CodingFragment)
	if err != nil {
		return "", err
	}
	correctionModel, err := s.workerModel(station.CodingFragmentCorrection)
	if err != nil {
		return "", err
	}
	runtime := directCodingWorkerRuntime(s)
	runtime.CorrectionModel = correctionModel
	return runDirectCodingTypeScriptFragmentWorker(runtime, modelName, job)
}

func directCodingApplicationTaskFragmentJob(
	stage *directCodingProgram,
	block assemblyline.TypeScriptBlock,
) (directCodingTypeScriptFragmentJob, error) {
	if stage == nil {
		return directCodingTypeScriptFragmentJob{}, fmt.Errorf("application task generation requires one isolated stage")
	}
	declarations := make(map[string]string)
	tsx := false
	found := false
	for _, document := range stage.TypeScript.Documents {
		for _, candidate := range document.Blocks {
			if !candidate.Generated() || strings.TrimSpace(stage.Generated[candidate.ID]) != "" {
				declarations[candidate.ID] = strings.TrimSpace(candidate.API)
			}
			if candidate.ID == block.ID {
				tsx = document.TSX()
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
	return directCodingTypeScriptFragmentJob{block: block, tsx: tsx, available: available}, nil
}

func directCodingApplicationTaskStageCommands(
	stage directCodingProgram,
	context assemblyline.ApplicationTaskContext,
) ([][]string, error) {
	_, acceptanceID, err := applicationTaskBlockIDs(context.Task.TaskID)
	if err != nil {
		return nil, err
	}
	path := ""
	for _, document := range stage.TypeScript.Documents {
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
	workspace, err := newDirectCodingTypeScriptStageWorkspace(s.runtime.ctx, *program)
	if err != nil {
		return err
	}
	defer workspace.Close()
	correctionProgress := newDirectCodingTypeScriptCorrectionProgress()
	return runDirectCodingApplicationTaskLifecycle(
		input, frozen, program,
		directCodingApplicationTaskLifecycleHooks{
			BeginTask: func(context assemblyline.ApplicationTaskContext) error {
				if s.cognition == nil {
					return fmt.Errorf("application task lifecycle requires persisted task cognition")
				}
				return s.cognition.Begin(context.Task.TaskID)
			},
			BuildBlock: s.generateDirectCodingApplicationTaskBlock,
			CompleteTask: func(context assemblyline.ApplicationTaskContext, generated map[string]string) error {
				return s.cognition.CompleteTask(context.Task.TaskID, generated)
			},
			FinalStage: func(complete *directCodingProgram) error {
				return s.stageTypeScriptProgramIn(
					workspace.Root(), complete, directCodingFullStageCommands(), correctionProgress,
				)
			},
		},
	)
}
