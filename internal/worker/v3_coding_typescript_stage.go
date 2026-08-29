package worker

import (
	"context"
	"fmt"
	"os"
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
	documents, err := composeDirectCodingSourceProgram(program)
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
		if structuredVitest {
			receipt, receiptErr := readDirectCodingVitestFailureReceipt(root)
			if receiptErr != nil {
				return nil, receiptErr
			}
			diagnosticOutput = strings.TrimSpace(receipt.Output + "\n" + output)
			diagnostic, mapped, mapErr := mapDirectCodingVitestFailureReceipt(root, documents, receipt)
			if mapErr != nil {
				return nil, mapErr
			}
			if mapped {
				diagnostic.VerificationStage = strings.Join(args, " ")
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
	origin, exists := directCodingSourceBlueprintBlock(program.Source, diagnostic.BlockID)
	if !exists {
		return nil, fmt.Errorf("route acceptance failure: unknown originating block %s", diagnostic.BlockID)
	}
	if origin.Role == assemblyline.SourceBlockTaskVerification {
		return nil, fmt.Errorf(
			"generated verification block %s failed staged execution and cannot authorize implementation mutation from dependency edges: %s",
			diagnostic.BlockID,
			safeLine(firstDirectCodingDiagnosticLine(diagnostic.Message), "unknown verification failure"),
		)
	}
	return diagnostic, nil
}

func runDirectCodingStageCommand(
	parent context.Context,
	root string,
	timeout time.Duration,
	name string,
	args ...string,
) (string, error) {
	execution, err := runValidatedV3Command(parent, root, codeCommand{
		Program: name, Args: append([]string(nil), args...), Timeout: timeout,
	})
	if err != nil {
		return "", fmt.Errorf("staged command is outside the code-owned verification boundary: %w", err)
	}
	rendered := renderV3CommandOutput(execution)
	if execution.ContextError != nil {
		return rendered, fmt.Errorf("command exceeded %s: %w", timeout, execution.ContextError)
	}
	if execution.RunError != nil {
		return rendered, fmt.Errorf("exit code %d", execution.ExitCode)
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

func directCodingTypeScriptBlockIsTSX(blueprint assemblyline.SourceBlueprint, blockID string) bool {
	for _, document := range blueprint.Documents {
		for _, block := range document.Blocks {
			if block.ID == blockID {
				return directCodingTypeScriptDocumentIsTSX(document)
			}
		}
	}
	return false
}

func directCodingTypeScriptDocumentIsTSX(document assemblyline.SourceDocument) bool {
	return strings.HasSuffix(strings.ToLower(document.Path), ".tsx")
}

func directCodingTypeScriptCorrectionBlock(
	blueprint assemblyline.SourceBlueprint,
	blockID string,
) (assemblyline.SourceBlock, error) {
	block, exists := directCodingSourceBlueprintBlock(blueprint, blockID)
	if !exists {
		return assemblyline.SourceBlock{}, fmt.Errorf("diagnostic names unknown block %s", blockID)
	}
	if !block.Generated() {
		return assemblyline.SourceBlock{}, fmt.Errorf(
			"code-owned browser adapter block %s failed validation and cannot be delegated",
			block.ID,
		)
	}
	if block.Role == assemblyline.SourceBlockTaskVerification {
		return assemblyline.SourceBlock{}, fmt.Errorf(
			"generated verification block %s is terminal staged evidence and cannot become repair model context",
			block.ID,
		)
	}
	return block, nil
}

func firstDirectCodingDiagnosticLine(message string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(message), "\n")
	return line
}
