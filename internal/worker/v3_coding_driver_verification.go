package worker

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/operation"
)

func (s *directCodingSession) Verify() (directCodingVerification, error) {
	if s.specification == nil || s.program == nil {
		return directCodingVerification{}, fmt.Errorf("coding verification requires accepted typed semantics and a compiled deterministic program")
	}
	programDiagnostic, err := directCodingProgramWorkspaceDiagnostic(s.root, *s.program)
	if err != nil {
		return directCodingVerification{}, err
	}
	if programDiagnostic != nil {
		s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_static_validation_failed", "diagnostic="+safeLine(programDiagnostic.Detail, "unknown"))
		return directCodingVerification{Diagnostic: programDiagnostic}, nil
	}
	targetTreeDiagnostic, err := directCodingTargetTreeWorkspaceDiagnostic(s.root, s.program.TargetTree)
	if err != nil {
		return directCodingVerification{}, err
	}
	if targetTreeDiagnostic != nil {
		s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_target_tree_validation_failed", "diagnostic="+safeLine(targetTreeDiagnostic.Detail, "unknown"))
		return directCodingVerification{Diagnostic: targetTreeDiagnostic}, nil
	}
	commands, err := directCodingProgramVerificationCommands(*s.specification, *s.program)
	if err != nil {
		return directCodingVerification{}, err
	}
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_verification_started", fmt.Sprintf("commands=%d", len(commands)))
	executed := make([]string, 0, len(commands))
	testEvidence := false
	for _, command := range commands {
		label := directCodingCommandLabel(command)
		result, err := s.executeDirectCodingCommand(command)
		if err != nil {
			return directCodingVerification{}, err
		}
		executed = append(executed, label)
		if !directCodingCommandSucceeded(result) {
			diagnostic := &directCodingDiagnostic{
				Stage:   "verify",
				Command: label,
				Detail:  directCodingCommandResult(result),
			}
			s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_verification_failed", fmt.Sprintf(
				"command=%s diagnostic=%s",
				directCodingEventToken(label, "unknown"),
				safeLine(diagnostic.Detail, "unknown"),
			))
			return directCodingVerification{Commands: executed, Diagnostic: diagnostic}, nil
		}
		output := operationResultText(result.Output, "stdout") + "\n" + operationResultText(result.Output, "stderr")
		if isV3TestCommand(command.Name, command.Args) && !verificationReportsNoTests(output) {
			testEvidence = true
		}
		s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_verification_command_passed", "command="+directCodingEventToken(label, "unknown"))
	}
	if s.completion.TestsRequired && !testEvidence {
		diagnostic := &directCodingDiagnostic{
			Stage:   "verify",
			Command: strings.Join(executed, " && "),
			Detail:  "Verification commands succeeded but reported no executed tests. Add focused tests for the requested success and failure behavior.",
		}
		return directCodingVerification{Commands: executed, Diagnostic: diagnostic}, nil
	}
	if diagnostic := directCodingUnfinishedDiagnostic(s.completion); diagnostic != nil {
		return directCodingVerification{Commands: executed, Diagnostic: diagnostic}, nil
	}
	sequence := s.nextSequence()
	s.completion.LatestCheckTurn = sequence
	if testEvidence {
		s.completion.LatestTestTurn = sequence
	}
	s.completion.LastTestHadNoTests = s.completion.TestsRequired && !testEvidence
	s.lastCommands = append([]string(nil), executed...)
	return directCodingVerification{
		Passed:      true,
		TestsPassed: testEvidence,
		Commands:    executed,
	}, nil
}

func (s *directCodingSession) Complete(verification directCodingVerification) (string, error) {
	if !verification.Passed {
		return "", fmt.Errorf("cannot complete coding workflow from a failed verification result")
	}
	if err := validateDirectCodingCompletion(s.completion); err != nil {
		return "", err
	}
	if err := validateDirectCodingProtectedPaths(s.root, s.protectedPaths); err != nil {
		return "", err
	}
	if s.cognition == nil {
		return "", fmt.Errorf("coding completion requires persisted task cognition")
	}
	if err := s.cognition.CompleteObjective(verification); err != nil {
		return "", err
	}
	summary := fmt.Sprintf(
		"Completed deterministic coding workflow: planned_files=%d planned_deletes=%d accepted_mutations=%d %s verification=%s",
		s.plannedFiles,
		s.plannedDeletes,
		s.completion.MutationCount,
		renderDirectCodingMutationJournal(s.mutationJournal),
		strings.Join(verification.Commands, " | "),
	)
	return summary, nil
}

func renderDirectCodingMutationJournal(entries []directCodingMutationJournalEntry) string {
	groups := map[workspaceFileOperation][]string{
		workspaceFileCreate: {}, workspaceFileReplace: {}, workspaceFileDelete: {},
	}
	for _, entry := range entries {
		if _, registered := groups[entry.Operation]; !registered {
			continue
		}
		groups[entry.Operation] = append(groups[entry.Operation], entry.Path)
	}
	for operation := range groups {
		sort.Strings(groups[operation])
	}
	return fmt.Sprintf(
		"created=[%s] replaced=[%s] deleted=[%s]",
		strings.Join(groups[workspaceFileCreate], ","),
		strings.Join(groups[workspaceFileReplace], ","),
		strings.Join(groups[workspaceFileDelete], ","),
	)
}

func (s *directCodingSession) executeDirectCodingCommand(command testCommand) (operation.Result, error) {
	label := directCodingCommandLabel(command)
	if err := validateV3Command(command.Name, command.Args); err != nil {
		return operation.Result{}, fmt.Errorf("server-selected coding command %s is invalid: %w", label, err)
	}
	result, err := executeCodeCommandAtRoot(s.runtime.ctx, s.root, codeCommand{
		Program: command.Name, Args: append([]string(nil), command.Args...),
	})
	if err != nil {
		return operation.Result{}, fmt.Errorf("execute server-selected coding command %s: %w", label, err)
	}
	if err := s.persistCodeOwnedEvidence(result); err != nil {
		return operation.Result{}, fmt.Errorf("persist server-selected coding command %s: %w", label, err)
	}
	return result, nil
}

func directCodingCommandSucceeded(result operation.Result) bool {
	succeeded, ok := result.Output["succeeded"].(bool)
	return ok && succeeded
}

func directCodingCommandLabel(command testCommand) string {
	return strings.Join(append([]string{command.Name}, command.Args...), " ")
}

func directCodingVerificationCommands(root string) []testCommand {
	exists := func(path string) bool {
		info, err := os.Stat(filepath.Join(root, path))
		return err == nil && !info.IsDir()
	}
	commands := make([]testCommand, 0, 8)
	if exists("go.mod") {
		commands = append(commands, testCommand{Family: "go", Name: "go", Args: []string{"test", "./..."}})
	}
	if exists("Cargo.toml") {
		commands = append(commands, testCommand{Family: "rust", Name: "cargo", Args: []string{"test"}})
	}
	if exists("package.json") {
		command := testCommand{Family: "node", Name: "npm", Args: []string{"test"}}
		if exists("pnpm-lock.yaml") {
			command.Name = "pnpm"
		} else if exists("yarn.lock") {
			command.Name = "yarn"
		}
		commands = append(commands, command)
	}
	if exists("pyproject.toml") || exists("requirements.txt") {
		commands = append(commands, testCommand{Family: "python", Name: "python3", Args: []string{"-m", "pytest", "-q"}})
	}
	if exists("composer.json") {
		commands = append(commands, testCommand{Family: "php", Name: "composer", Args: []string{"test"}})
	}
	if exists("pom.xml") {
		commands = append(commands, testCommand{Family: "java", Name: "mvn", Args: []string{"test"}})
	}
	if exists("build.gradle") || exists("build.gradle.kts") {
		commands = append(commands, testCommand{Family: "java", Name: "gradle", Args: []string{"test"}})
	}
	if matches, _ := filepath.Glob(filepath.Join(root, "*.csproj")); len(matches) > 0 {
		commands = append(commands, testCommand{Family: "dotnet", Name: "dotnet", Args: []string{"test"}})
	}
	return commands
}

func directCodingCommandResult(result operation.Result) string {
	parts := make([]string, 0, 3)
	if exitCode, ok := result.Output["exit_code"]; ok {
		parts = append(parts, fmt.Sprintf("exit_code=%v", exitCode))
	}
	if stdout, _ := result.Output["stdout"].(string); strings.TrimSpace(stdout) != "" {
		parts = append(parts, "stdout:\n"+trimForBudget(stdout, 5000))
	}
	if stderr, _ := result.Output["stderr"].(string); strings.TrimSpace(stderr) != "" {
		parts = append(parts, "stderr:\n"+trimForBudget(stderr, 5000))
	}
	return strings.Join(parts, "\n")
}
