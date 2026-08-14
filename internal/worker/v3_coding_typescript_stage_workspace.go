package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/station"
)

type directCodingTypeScriptStageWorkspace struct {
	root string
}

func newDirectCodingTypeScriptStageWorkspace(
	ctx context.Context,
	program directCodingProgram,
) (*directCodingTypeScriptStageWorkspace, error) {
	packageFile, err := directCodingStagePackageFile(program.StaticFiles)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("", "omnidex-typescript-stage-")
	if err != nil {
		return nil, fmt.Errorf("create isolated TypeScript coding stage: %w", err)
	}
	workspace := &directCodingTypeScriptStageWorkspace{root: root}
	if err := writeDirectCodingVitestReporter(root); err != nil {
		workspace.Close()
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(root, packageFile.Path), []byte(packageFile.Content), 0o600); err != nil {
		workspace.Close()
		return nil, fmt.Errorf("write staged TypeScript package manifest: %w", err)
	}
	output, err := runDirectCodingStageCommand(
		ctx, root, directCodingTypeScriptInstallTimeout, "npm", directCodingNPMInstallArgs()...,
	)
	if err != nil {
		workspace.Close()
		return nil, fmt.Errorf(
			"staged TypeScript dependency installation failed: %w\n%s",
			err, trimForBudget(output, 12_000),
		)
	}
	return workspace, nil
}

func directCodingStagePackageFile(files []directCodingFileTask) (directCodingFileTask, error) {
	var packageFile directCodingFileTask
	count := 0
	for _, file := range files {
		if file.Path == "package.json" {
			packageFile = file
			count++
		}
	}
	if count != 1 || strings.TrimSpace(packageFile.Content) == "" {
		return directCodingFileTask{}, fmt.Errorf("TypeScript stage requires one non-empty package.json")
	}
	return packageFile, nil
}

func (workspace *directCodingTypeScriptStageWorkspace) Root() string {
	if workspace == nil {
		return ""
	}
	return workspace.root
}

func (workspace *directCodingTypeScriptStageWorkspace) Close() {
	if workspace == nil || workspace.root == "" {
		return
	}
	_ = os.RemoveAll(workspace.root)
	workspace.root = ""
}

func resetDirectCodingTypeScriptStage(root string) error {
	if root == "" || !filepath.IsAbs(root) {
		return fmt.Errorf("reset TypeScript stage requires one absolute temporary root")
	}
	for _, relative := range []string{
		"src", "dist", ".vite", "index.html", "tsconfig.json", "vite.config.ts", ".gitignore",
		directCodingVitestReportFile,
	} {
		if err := os.RemoveAll(filepath.Join(root, relative)); err != nil {
			return fmt.Errorf("reset staged TypeScript path %s: %w", relative, err)
		}
	}
	return nil
}

func (s *directCodingSession) stageTypeScriptProgramIn(
	root string,
	program *directCodingProgram,
	commands [][]string,
	remainingCorrections *int,
) error {
	if program == nil || remainingCorrections == nil || *remainingCorrections < 0 {
		return fmt.Errorf("staged TypeScript verification requires a program and correction budget")
	}
	if err := resetDirectCodingTypeScriptStage(root); err != nil {
		return err
	}
	repeated := make(map[string]int)
	for attempt := 1; ; attempt++ {
		s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_stage_started", fmt.Sprintf(
			"attempt=%d generated_blocks=%d corrections_remaining=%d",
			attempt, len(program.Generated), *remainingCorrections,
		))
		if err := writeDirectCodingTypeScriptStage(root, *program); err != nil {
			return err
		}
		diagnostic, err := verifyDirectCodingTypeScriptStageCommands(
			s.runtime.ctx, root, *program, commands,
		)
		if err != nil {
			return err
		}
		if diagnostic == nil {
			s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_stage_passed", fmt.Sprintf(
				"attempt=%d generated_blocks=%d", attempt, len(program.Generated),
			))
			return nil
		}
		if *remainingCorrections == 0 {
			return fmt.Errorf("staged TypeScript program exhausted its node-correction budget: %s", diagnostic.Message)
		}
		if err := s.correctDirectCodingTypeScriptStage(program, diagnostic, repeated); err != nil {
			return err
		}
		(*remainingCorrections)--
	}
}

func (s *directCodingSession) correctDirectCodingTypeScriptStage(
	program *directCodingProgram,
	diagnostic *directCodingStageDiagnostic,
	repeated map[string]int,
) error {
	target, err := directCodingTypeScriptCorrectionBlock(program.TypeScript, diagnostic.BlockID)
	if err != nil {
		return fmt.Errorf("route staged TypeScript diagnostic: %w: %s", err, diagnostic.Message)
	}
	fingerprint := target.ID + "\x00" + firstDirectCodingDiagnosticLine(diagnostic.Message)
	repeated[fingerprint]++
	if repeated[fingerprint] > maxDirectCodingStageRepeatedCorrections {
		return fmt.Errorf(
			"block %s repeated the same staged failure %d times: %s",
			target.ID, maxDirectCodingStageRepeatedCorrections, diagnostic.Message,
		)
	}
	declarations, err := directCodingTypeScriptAcceptedDeclarations(program.TypeScript, program.Generated)
	if err != nil {
		return err
	}
	available, err := directCodingTypeScriptAvailableDeclarations(target, declarations)
	if err != nil {
		return err
	}
	modelName, err := s.workerModel(station.CodingFragmentCorrection)
	if err != nil {
		return err
	}
	failure := directCodingTypeScriptModelFailure(diagnostic.Output)
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_fragment_correction_started", fmt.Sprintf(
		"block=%s exact_failure=%s", target.ID,
		safeLine(trimForBudget(failure, 500), "unknown"),
	))
	workerRuntime := directCodingWorkerRuntime(s)
	workerRuntime.CorrectionModel = modelName
	source, err := runDirectCodingTypeScriptFragmentWorker(
		workerRuntime, modelName,
		directCodingTypeScriptFragmentJob{
			block: target, tsx: directCodingTypeScriptBlockIsTSX(program.TypeScript, target.ID),
			available: available, current: program.Generated[target.ID], failure: failure,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"correct block %s for staged failure %s: %w",
			target.ID, safeLine(firstDirectCodingDiagnosticLine(diagnostic.Message), "unknown"), err,
		)
	}
	if source == program.Generated[target.ID] {
		return fmt.Errorf("block %s returned an unchanged declaration for staged failure: %s", target.ID, diagnostic.Message)
	}
	program.Generated[target.ID] = source
	return nil
}
