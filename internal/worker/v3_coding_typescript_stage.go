package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const directCodingTypeScriptInstallTimeout = 3 * time.Minute

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
	return verifyDirectCodingTypeScriptStageCommands(parent, root, program, directCodingFullStageCommands())
}

func verifyDirectCodingTypeScriptStageCommands(
	parent context.Context,
	root string,
	program directCodingProgram,
	commands [][]string,
) (*directCodingStageDiagnostic, error) {
	documents, err := composeDirectCodingTypeScriptProgram(program)
	if err != nil {
		return nil, err
	}
	for _, args := range commands {
		if len(args) == 0 {
			return nil, fmt.Errorf("staged TypeScript command is empty")
		}
		structuredVitest := directCodingStageCommandUsesVitestReport(args)
		if structuredVitest {
			if err := clearDirectCodingVitestReport(root); err != nil {
				return nil, err
			}
		}
		output, commandErr := runDirectCodingStageCommand(parent, root, directCodingStageTimeout, "npm", args...)
		if commandErr == nil {
			continue
		}
		diagnosticOutput := output
		failureClass := directCodingStageFailureUnclassified
		if structuredVitest {
			receipt, receiptErr := readDirectCodingVitestFailureReceipt(root)
			if receiptErr != nil {
				return nil, receiptErr
			}
			failureClass = receipt.FailureClass
			diagnosticOutput = strings.TrimSpace(receipt.Output + "\n" + output)
			if diagnostic, mapped := mapDirectCodingVitestFailureReceipt(root, documents, receipt); mapped {
				diagnostic.VerificationStage = strings.Join(args, " ")
				diagnostic.FailureClass = failureClass
				diagnostic, err = routeDirectCodingAcceptanceFailure(program, diagnostic)
				if err != nil {
					return nil, err
				}
				return diagnostic, nil
			}
		}
		if !structuredVitest {
			diagnostic, mapped := mapDirectCodingTypeScriptStageDiagnostic(documents, diagnosticOutput)
			if !mapped {
				return nil, directCodingUnmappedStageFailure(args, commandErr, output, diagnosticOutput)
			}
			diagnostic.VerificationStage = strings.Join(args, " ")
			diagnostic.FailureClass = failureClass
			return diagnostic, nil
		}
		return nil, directCodingUnmappedStageFailure(args, commandErr, output, diagnosticOutput)
	}
	return nil, nil
}

func directCodingFullStageCommands() [][]string {
	return [][]string{{"run", "typecheck"}, directCodingStructuredVitestCommand(""), {"run", "build"}}
}

func routeDirectCodingAcceptanceFailure(
	program directCodingProgram,
	diagnostic *directCodingStageDiagnostic,
) (*directCodingStageDiagnostic, error) {
	if diagnostic == nil {
		return nil, fmt.Errorf("route acceptance failure: diagnostic is nil")
	}
	_, exists := directCodingTypeScriptBlueprintBlock(program.TypeScript, diagnostic.BlockID)
	if !exists {
		return nil, fmt.Errorf("route acceptance failure: unknown originating block %s", diagnostic.BlockID)
	}
	if diagnostic.FailureClass != directCodingStageFailureVitestBehavior {
		return diagnostic, nil
	}
	receipt, reviewed := program.AcceptanceGrounding[diagnostic.BlockID]
	if !reviewed {
		return diagnostic, nil
	}
	context, featureID, recognized, err := directCodingAcceptanceTaskAuthority(program, diagnostic.BlockID)
	if err != nil {
		return nil, err
	}
	if !recognized {
		return diagnostic, nil
	}
	source := strings.TrimSpace(program.Generated[diagnostic.BlockID])
	tsx := directCodingTypeScriptBlockIsTSX(program.TypeScript, diagnostic.BlockID)
	input, err := assemblyline.NewApplicationAcceptanceGroundingReviewInput(
		context, source, tsx,
		directCodingBrowserAcceptancePlatformAuthorities(),
	)
	if err != nil {
		return diagnostic, nil
	}
	authorized, err := receipt.AuthorizesFeatureFailureAt(
		input, source, tsx, diagnostic.DeclarationLine, diagnostic.DeclarationColumn,
	)
	if err != nil || !authorized {
		return diagnostic, nil
	}
	feature, exists := directCodingTypeScriptBlueprintBlock(program.TypeScript, featureID)
	if !exists || !feature.Generated() || strings.TrimSpace(program.Generated[featureID]) == "" {
		return nil, fmt.Errorf("grounded acceptance %s targets unavailable implementation %s", diagnostic.BlockID, featureID)
	}
	routed := *diagnostic
	routed.BlockID = featureID
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

func directCodingUnmappedStageFailure(
	args []string,
	commandErr error,
	commandOutput string,
	diagnosticOutput string,
) error {
	evidence := strings.TrimSpace(diagnosticOutput)
	if evidence == "" {
		evidence = strings.TrimSpace(commandOutput)
	}
	return fmt.Errorf(
		"staged TypeScript command npm %s failed without one block-owned diagnostic: %w\n%s",
		strings.Join(args, " "), commandErr, trimForBudget(evidence, 12_000),
	)
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
