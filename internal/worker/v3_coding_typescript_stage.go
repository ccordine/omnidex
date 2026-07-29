package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/specialist"
)

const directCodingTypeScriptInstallTimeout = 3 * time.Minute

var (
	directCodingTypeScriptColonIssuePattern = regexp.MustCompile(`(?m)(?:^|[ \t])((?:\./)?[A-Za-z0-9_./-]+\.tsx?):([0-9]+):([0-9]+)`)
	directCodingTypeScriptParenIssuePattern = regexp.MustCompile(`(?m)(?:^|[ \t])((?:\./)?[A-Za-z0-9_./-]+\.tsx?)\(([0-9]+),([0-9]+)\)`)
)

func (s *directCodingSession) stageTypeScriptProgram(program *directCodingProgram) error {
	root, err := os.MkdirTemp("", "omnidex-charmander-typescript-stage-")
	if err != nil {
		return fmt.Errorf("create isolated TypeScript coding stage: %w", err)
	}
	defer os.RemoveAll(root)
	if err := writeDirectCodingTypeScriptStage(root, *program); err != nil {
		return err
	}
	if output, err := runDirectCodingStageCommand(
		s.runtime.ctx, root, directCodingTypeScriptInstallTimeout, "npm", directCodingNPMInstallArgs()...,
	); err != nil {
		return fmt.Errorf("staged TypeScript dependency installation failed: %w\n%s", err, trimForBudget(output, 12_000))
	}

	repeated := make(map[string]int)
	for correction := 0; correction <= maxDirectCodingStageCorrections; correction++ {
		s.runtime.svc.emitStepEvent(s.runtime.claim.Step.ID, "coding_stage_started", fmt.Sprintf(
			"attempt=%d generated_blocks=%d", correction+1, len(program.Generated),
		))
		if err := writeDirectCodingTypeScriptStage(root, *program); err != nil {
			return err
		}
		diagnostic, err := verifyDirectCodingTypeScriptStage(s.runtime.ctx, root, *program)
		if err != nil {
			return err
		}
		if diagnostic == nil {
			s.runtime.svc.emitStepEvent(s.runtime.claim.Step.ID, "coding_stage_passed", fmt.Sprintf(
				"attempt=%d generated_blocks=%d", correction+1, len(program.Generated),
			))
			return nil
		}
		if correction == maxDirectCodingStageCorrections {
			return fmt.Errorf("staged TypeScript program exhausted %d node corrections: %s", maxDirectCodingStageCorrections, diagnostic.Message)
		}
		target, err := directCodingTypeScriptCorrectionBlock(program.TypeScript, diagnostic.BlockID)
		if err != nil {
			return fmt.Errorf("route staged TypeScript diagnostic: %w: %s", err, diagnostic.Message)
		}
		fingerprint := target.ID + "\x00" + firstDirectCodingDiagnosticLine(diagnostic.Message)
		repeated[fingerprint]++
		if repeated[fingerprint] > maxDirectCodingStageRepeatedCorrections {
			return fmt.Errorf("block %s repeated the same staged failure %d times: %s", target.ID, maxDirectCodingStageRepeatedCorrections, diagnostic.Message)
		}
		declarations, err := directCodingTypeScriptAcceptedDeclarations(program.TypeScript, program.Generated)
		if err != nil {
			return err
		}
		available, err := directCodingTypeScriptAvailableDeclarations(target, declarations)
		if err != nil {
			return err
		}
		modelName, err := s.workerModel("coding_fragment_correction", specialist.RoleCodingFragmentCorrectionStation)
		if err != nil {
			return err
		}
		failure := directCodingTypeScriptModelFailure(diagnostic.Output)
		s.runtime.svc.emitStepEvent(s.runtime.claim.Step.ID, "coding_fragment_correction_started", fmt.Sprintf(
			"block=%s correction=%d exact_failure=%s", target.ID, correction+1,
			safeLine(trimForBudget(failure, 500), "unknown"),
		))
		workerRuntime := directCodingWorkerRuntime(s)
		workerRuntime.CorrectionModel = modelName
		source, err := runDirectCodingTypeScriptFragmentWorker(
			workerRuntime, modelName,
			directCodingTypeScriptFragmentJob{
				block: target, tsx: directCodingTypeScriptBlockIsTSX(program.TypeScript, target.ID),
				available: available, current: program.Generated[target.ID],
				failure: failure,
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
	}
	return fmt.Errorf("staged TypeScript program correction loop ended without a result")
}

func writeDirectCodingTypeScriptStage(root string, program directCodingProgram) error {
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		return err
	}
	for _, file := range assembly.Files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return fmt.Errorf("create staged directory for %s: %w", file.Path, err)
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o600); err != nil {
			return fmt.Errorf("write staged source %s: %w", file.Path, err)
		}
	}
	return nil
}

func verifyDirectCodingTypeScriptStage(
	parent context.Context,
	root string,
	program directCodingProgram,
) (*directCodingStageDiagnostic, error) {
	documents, err := composeDirectCodingTypeScriptProgram(program)
	if err != nil {
		return nil, err
	}
	commands := [][]string{{"test"}, {"run", "typecheck"}, {"run", "build"}}
	for _, args := range commands {
		output, commandErr := runDirectCodingStageCommand(parent, root, directCodingStageTimeout, "npm", args...)
		if commandErr == nil {
			continue
		}
		if diagnostic, mapped := mapDirectCodingTypeScriptStageDiagnostic(documents, output); mapped {
			if args[0] == "test" {
				diagnostic, err = routeDirectCodingAcceptanceFailure(program.TypeScript, diagnostic)
				if err != nil {
					return nil, err
				}
			}
			return diagnostic, nil
		}
		return nil, fmt.Errorf(
			"staged TypeScript command npm %s failed without one block-owned diagnostic: %w\n%s",
			strings.Join(args, " "), commandErr, trimForBudget(output, 12_000),
		)
	}
	return nil, nil
}

func routeDirectCodingAcceptanceFailure(
	blueprint assemblyline.TypeScriptBlueprint,
	diagnostic *directCodingStageDiagnostic,
) (*directCodingStageDiagnostic, error) {
	if diagnostic == nil {
		return nil, fmt.Errorf("route acceptance failure: diagnostic is nil")
	}
	block, exists := directCodingTypeScriptBlueprintBlock(blueprint, diagnostic.BlockID)
	if !exists {
		return nil, fmt.Errorf("route acceptance failure: unknown originating block %s", diagnostic.BlockID)
	}
	if block.FailureTarget == "" {
		return diagnostic, nil
	}
	if _, exists := directCodingTypeScriptBlueprintBlock(blueprint, block.FailureTarget); !exists {
		return nil, fmt.Errorf("route acceptance failure: unknown target block %s", block.FailureTarget)
	}
	routed := *diagnostic
	routed.BlockID = block.FailureTarget
	return &routed, nil
}

func runDirectCodingStageCommand(
	parent context.Context,
	root string,
	timeout time.Duration,
	name string,
	args ...string,
) (string, error) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = root
	command.Env = os.Environ()
	output, err := command.CombinedOutput()
	rendered := strings.TrimSpace(string(output))
	if ctx.Err() != nil {
		return rendered, fmt.Errorf("command exceeded %s: %w", timeout, ctx.Err())
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return rendered, fmt.Errorf("exit code %d", exitErr.ExitCode())
		}
		return rendered, err
	}
	return rendered, nil
}

func mapDirectCodingTypeScriptStageDiagnostic(
	documents []assemblyline.ComposedTypeScriptDocument,
	output string,
) (*directCodingStageDiagnostic, bool) {
	searchable := directCodingANSISequencePattern.ReplaceAllString(output, "")
	byPath := make(map[string]assemblyline.ComposedTypeScriptDocument, len(documents))
	for _, document := range documents {
		byPath[filepath.ToSlash(document.Path)] = document
	}
	patterns := []*regexp.Regexp{directCodingTypeScriptColonIssuePattern, directCodingTypeScriptParenIssuePattern}
	for _, pattern := range patterns {
		for _, match := range pattern.FindAllStringSubmatch(searchable, -1) {
			path := filepath.ToSlash(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(match[1]), "./"), "/"))
			document, exists := byPath[path]
			if !exists {
				continue
			}
			line, err := strconv.Atoi(match[2])
			if err != nil {
				continue
			}
			for blockID, span := range document.Spans {
				if span.Contains(line) {
					location := fmt.Sprintf("%s:%s:%s", path, match[2], match[3])
					return &directCodingStageDiagnostic{
						BlockID: blockID, Message: location + "\n" + trimForBudget(searchable, 5000), Output: searchable,
					}, true
				}
			}
		}
	}
	return nil, false
}

func directCodingTypeScriptBlockIsTSX(blueprint assemblyline.TypeScriptBlueprint, blockID string) bool {
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.ID == blockID {
				return document.TSX()
			}
		}
	}
	return false
}

func directCodingTypeScriptCorrectionBlock(
	blueprint assemblyline.TypeScriptBlueprint,
	blockID string,
) (assemblyline.TypeScriptBlock, error) {
	block, exists := directCodingTypeScriptBlueprintBlock(blueprint, blockID)
	if !exists {
		return assemblyline.TypeScriptBlock{}, fmt.Errorf("diagnostic names unknown block %s", blockID)
	}
	if !block.Generated() {
		return assemblyline.TypeScriptBlock{}, fmt.Errorf(
			"code-owned browser adapter block %s failed validation and cannot be delegated",
			block.ID,
		)
	}
	return block, nil
}

func firstDirectCodingDiagnosticLine(message string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(message), "\n")
	return line
}
