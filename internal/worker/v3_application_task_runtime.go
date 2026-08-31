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
	ref assemblyline.SourceBlockRef,
	validateInitialCandidate func(string) error,
) (string, error) {
	job, err := directCodingApplicationTaskFragmentJob(
		stage, ref, validateInitialCandidate,
	)
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
	validateInitialCandidate func(string) error,
) (directCodingTypeScriptFragmentJob, error) {
	if stage == nil {
		return directCodingTypeScriptFragmentJob{}, fmt.Errorf("application task generation requires one isolated stage")
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
		block: block, dialect: stage.Project.Dialect, tsx: tsx, available: available,
		validateInitialCandidate: validateInitialCandidate,
	}, nil
}

func directCodingTypeScriptDocumentIsTSX(document assemblyline.SourceDocument) bool {
	return strings.HasSuffix(strings.ToLower(document.Path), ".tsx")
}

func (s *directCodingSession) runDirectCodingApplicationTaskLifecycle(
	frozen assemblyline.FrozenApplicationWorkload,
	program *directCodingProgram,
) error {
	if program == nil {
		return fmt.Errorf("application task lifecycle requires one program")
	}
	stack := program.Project.Stack
	if stack.NewSourceGenerator == nil {
		return fmt.Errorf("project stack %s has no source generator", stack.ID)
	}
	generator, err := stack.NewSourceGenerator(s, *program)
	if err != nil {
		return err
	}
	lifecycleErr := runDirectCodingApplicationTaskLifecycle(
		frozen, program,
		directCodingApplicationTaskLifecycleHooks{
			BuildBlock: generator.GenerateBlock,
		},
	)
	return lifecycleErr
}
