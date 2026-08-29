package worker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

type directCodingTypeScriptStageWorkspace struct {
	root string
}

func newDirectCodingTypeScriptStageWorkspace(
	ctx context.Context,
	program directCodingProgram,
) (*directCodingTypeScriptStageWorkspace, error) {
	packageFiles, err := directCodingStagePackageFiles(program.StaticFiles)
	if err != nil {
		return nil, err
	}
	root, err := os.MkdirTemp("", "omnidex-typescript-stage-")
	if err != nil {
		return nil, fmt.Errorf("create isolated TypeScript coding stage: %w", err)
	}
	workspace := &directCodingTypeScriptStageWorkspace{root: root}
	if err := writeDirectCodingVitestReporter(root); err != nil {
		_ = workspace.Close()
		return nil, err
	}
	if err := writeDirectCodingTypeScriptScopeInspector(root); err != nil {
		_ = workspace.Close()
		return nil, err
	}
	for _, packageFile := range packageFiles {
		if err := os.WriteFile(filepath.Join(root, packageFile.Path), []byte(packageFile.Content), 0o600); err != nil {
			_ = workspace.Close()
			return nil, fmt.Errorf("write staged TypeScript package authority %s: %w", packageFile.Path, err)
		}
	}
	output, err := runDirectCodingStageCommand(
		ctx, root, directCodingTypeScriptInstallTimeout, "npm", directCodingNPMInstallArgs()...,
	)
	if err != nil {
		_ = workspace.Close()
		return nil, fmt.Errorf(
			"staged TypeScript dependency installation failed: %w\n%s",
			err, trimForBudget(output, 12_000),
		)
	}
	return workspace, nil
}

func directCodingStagePackageFiles(files []directCodingFileTask) ([]directCodingFileTask, error) {
	required := map[string]directCodingFileTask{
		"package.json":      {},
		"package-lock.json": {},
	}
	counts := map[string]int{}
	for _, file := range files {
		if _, exists := required[file.Path]; exists {
			required[file.Path] = file
			counts[file.Path]++
		}
	}
	ordered := make([]directCodingFileTask, 0, len(required))
	for _, artifactPath := range []string{"package.json", "package-lock.json"} {
		file := required[artifactPath]
		if counts[artifactPath] != 1 || strings.TrimSpace(file.Content) == "" {
			return nil, fmt.Errorf("TypeScript stage requires one non-empty %s", artifactPath)
		}
		ordered = append(ordered, file)
	}
	return ordered, nil
}

func (workspace *directCodingTypeScriptStageWorkspace) Root() string {
	if workspace == nil {
		return ""
	}
	return workspace.root
}

func (workspace *directCodingTypeScriptStageWorkspace) Close() error {
	if workspace == nil || workspace.root == "" {
		return nil
	}
	err := os.RemoveAll(workspace.root)
	workspace.root = ""
	if err != nil {
		return fmt.Errorf("remove isolated TypeScript stage: %w", err)
	}
	return nil
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
	progress *directCodingTypeScriptCorrectionProgress,
) error {
	if program == nil || progress == nil || progress.seen == nil {
		return fmt.Errorf("staged TypeScript verification requires a program and correction progress authority")
	}
	if err := resetDirectCodingTypeScriptStage(root); err != nil {
		return err
	}
	if err := progress.beginStage(); err != nil {
		return err
	}
	for attempt := 1; ; attempt++ {
		if err := s.runtime.ctx.Err(); err != nil {
			return fmt.Errorf("staged TypeScript correction stopped by context authority: %w", err)
		}
		s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_stage_started", fmt.Sprintf(
			"attempt=%d generated_blocks=%d", attempt, len(program.Generated),
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
		if err := s.correctDirectCodingTypeScriptStage(root, program, diagnostic, progress); err != nil {
			return err
		}
	}
}

func (s *directCodingSession) correctDirectCodingTypeScriptStage(
	root string,
	program *directCodingProgram,
	diagnostic *directCodingStageDiagnostic,
	progress *directCodingTypeScriptCorrectionProgress,
) error {
	if diagnostic == nil {
		return fmt.Errorf("correct staged TypeScript program requires one diagnostic")
	}
	target, err := directCodingTypeScriptCorrectionBlock(program.Source, diagnostic.BlockID)
	if err != nil {
		return fmt.Errorf("route staged TypeScript diagnostic: %w: %s", err, diagnostic.Message)
	}
	current, exists := program.Generated[target.ID]
	if !exists || strings.TrimSpace(current) == "" {
		return fmt.Errorf("staged TypeScript diagnostic target %s has no accepted declaration", target.ID)
	}
	failure, err := directCodingTypeScriptStageModelFeedback(diagnostic)
	if err != nil {
		return err
	}
	tsx := directCodingTypeScriptBlockIsTSX(program.Source, target.ID)
	var repairRegion *assemblyline.TypeScriptFragmentRepairRegion
	if diagnostic.CompilerIssue {
		scope, bindingErr := inspectDirectCodingTypeScriptScope(s.runtime.ctx, root, *diagnostic)
		if bindingErr != nil {
			return fmt.Errorf("derive staged TypeScript compiler scope for block %s: %w", target.ID, bindingErr)
		}
		candidate, repaired, deterministicErr := applyDirectCodingTypeScriptDeterministicRepair(current, scope)
		if deterministicErr != nil {
			return fmt.Errorf(
				"apply deterministic TypeScript compiler repair for block %s: %w",
				target.ID, deterministicErr,
			)
		}
		if repaired {
			if _, parseErr := assemblyline.ParseTypeScriptFunction(
				assemblyline.TypeScriptFunctionContract{
					Signature: target.Signature, TSX: tsx, Policy: target.Policy,
				},
				candidate,
			); parseErr != nil {
				return fmt.Errorf(
					"validate deterministic TypeScript compiler repair for block %s: %w",
					target.ID, parseErr,
				)
			}
			if err := progress.observeDeterministic(
				target.ID, diagnostic.VerificationStage, failure,
			); err != nil {
				return err
			}
			program.Generated[target.ID] = candidate
			s.runtime.svc.emitStepEvent(
				s.runtime.claim.Authority,
				"coding_compiler_repair_applied",
				fmt.Sprintf("block=%s mechanism=deterministic_primitive_nullish_narrowing", target.ID),
			)
			return nil
		}
		localized, regionErr := assemblyline.NewTypeScriptCompilerRepairRegionWithEvidence(
			current, tsx, diagnostic.DeclarationLine, diagnostic.DeclarationColumn,
			scope.Bindings, scope.ExpressionEvidence, scope.UnavailableBindings,
		)
		if regionErr != nil {
			return fmt.Errorf("localize staged TypeScript compiler failure for block %s: %w", target.ID, regionErr)
		}
		repairRegion = &localized
	}
	if err := progress.observeSemantic(
		target.ID, diagnostic.VerificationStage, failure,
	); err != nil {
		return err
	}
	declarations, err := directCodingTypeScriptAcceptedDeclarations(program.Source, program.Generated)
	if err != nil {
		return err
	}
	available, err := directCodingTypeScriptAvailableDeclarations(target, declarations)
	if err != nil {
		return err
	}
	if directCodingTypeScriptRepairRegionHasExactIncompatibility(repairRegion) {
		// The checker has already projected the exact expression, its complete
		// inferred/contextual types, incompatible constituents, and referenced
		// local bindings. Broader capability declarations are unrelated to this
		// one type-narrowing uncertainty.
		available = ""
	}
	profile, err := directCodingVersionProfileForProgram(*program)
	if err != nil {
		return err
	}
	source, err := s.convergeDirectCodingTypeScriptGuidedRepair(
		target, tsx, profile.SourceDialect, available, current, repairRegion, failure,
	)
	if err != nil {
		return fmt.Errorf(
			"correct block %s for staged failure %s: %w",
			target.ID, safeLine(firstDirectCodingDiagnosticLine(diagnostic.Message), "unknown"), err,
		)
	}
	if strings.TrimSpace(source) == strings.TrimSpace(current) {
		return fmt.Errorf("block %s returned an unchanged declaration for staged failure: %s", target.ID, diagnostic.Message)
	}
	program.Generated[target.ID] = source
	return nil
}
