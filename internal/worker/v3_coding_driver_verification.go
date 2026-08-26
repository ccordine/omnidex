package worker

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/operation"
)

func (s *directCodingSession) Verify() (
	verification directCodingVerification,
	returnErr error,
) {
	if s.specification == nil || s.program == nil {
		return directCodingVerification{}, fmt.Errorf("coding verification requires accepted typed semantics and a compiled deterministic program")
	}
	programDiagnostic, err := directCodingProgramWorkspaceDiagnostic(
		s.root, *s.program, s.initialPaths,
	)
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
	stack, err := directCodingProjectStackByID(s.program.StackID)
	if err != nil {
		return directCodingVerification{}, err
	}
	if len(stack.CleanupCommands) != 0 {
		defer func() {
			if cleanupErr := s.executeDirectCodingCleanup(stack); cleanupErr != nil {
				verification = directCodingVerification{}
				returnErr = errors.Join(returnErr, cleanupErr)
			}
		}()
	}
	s.runtime.svc.emitStepEvent(s.runtime.claim.Authority, "coding_verification_started", fmt.Sprintf("commands=%d", len(commands)))
	executed := make([]string, 0, len(commands))
	evidenceIDs := make([]int64, 0, len(commands))
	testEvidence := false
	for _, command := range commands {
		label := directCodingCommandLabel(command)
		result, evidenceID, err := s.executeDirectCodingCommand(command)
		if err != nil {
			return directCodingVerification{}, err
		}
		executed = append(executed, label)
		evidenceIDs = append(evidenceIDs, evidenceID)
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
			return directCodingVerification{Commands: executed, EvidenceIDs: evidenceIDs, Diagnostic: diagnostic}, nil
		}
		output := operationResultText(result.Output, "stdout") + "\n" + operationResultText(result.Output, "stderr")
		if command.Purpose == verificationTest && !verificationReportsNoTests(output) {
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
		return directCodingVerification{Commands: executed, EvidenceIDs: evidenceIDs, Diagnostic: diagnostic}, nil
	}
	if diagnostic := directCodingUnfinishedDiagnostic(s.completion); diagnostic != nil {
		return directCodingVerification{Commands: executed, EvidenceIDs: evidenceIDs, Diagnostic: diagnostic}, nil
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
		EvidenceIDs: evidenceIDs,
	}, nil
}

func (s *directCodingSession) executeDirectCodingCleanup(stack directCodingProjectStack) error {
	for _, command := range stack.CleanupCommands {
		result, _, err := s.executeDirectCodingCommand(command)
		if err != nil {
			return fmt.Errorf("execute %s cleanup: %w", stack.ID, err)
		}
		if !directCodingCommandSucceeded(result) {
			return fmt.Errorf(
				"execute %s cleanup command %s: %s",
				stack.ID, directCodingCommandLabel(command), directCodingCommandResult(result),
			)
		}
	}
	return nil
}

func (s *directCodingSession) Complete(verification directCodingVerification) (string, error) {
	if !verification.Passed {
		return "", fmt.Errorf("cannot complete coding workflow from a failed verification result")
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
	if s.deploymentDisposition == assemblyline.ApplicationServiceDeploymentPersistCurrentHost {
		serviceURL, healthURL, err := directCodingDeploymentURLs(s.deployedEndpoint)
		if err != nil || s.deploymentOperationID == "" || len(s.deploymentReceiptSHA) != 64 {
			return "", fmt.Errorf("coding completion requires one canonical persisted deployment outcome")
		}
		summary += fmt.Sprintf(
			" deployment_operation=%s service_url=%s health_url=%s receipt_sha256=%s",
			s.deploymentOperationID, serviceURL, healthURL, s.deploymentReceiptSHA,
		)
	}
	return summary, nil
}

func renderDirectCodingMutationJournal(entries []directCodingMutationJournalEntry) string {
	groups := map[workspaceFileOperation][]string{
		workspaceDirectoryEnsure: {}, workspaceFileCreate: {}, workspaceFileReplace: {}, workspaceFileDelete: {},
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
		"directories=[%s] created=[%s] replaced=[%s] deleted=[%s]",
		strings.Join(groups[workspaceDirectoryEnsure], ","),
		strings.Join(groups[workspaceFileCreate], ","),
		strings.Join(groups[workspaceFileReplace], ","),
		strings.Join(groups[workspaceFileDelete], ","),
	)
}

func (s *directCodingSession) executeDirectCodingCommand(command testCommand) (operation.Result, int64, error) {
	label := directCodingCommandLabel(command)
	if err := validateV3Command(command.Name, command.Args); err != nil {
		return operation.Result{}, 0, fmt.Errorf("server-selected coding command %s is invalid: %w", label, err)
	}
	result, err := executeCodeCommandAtRoot(s.runtime.ctx, s.root, codeCommand{
		Program: command.Name, Args: append([]string(nil), command.Args...), Timeout: command.Timeout,
	})
	if err != nil {
		return operation.Result{}, 0, fmt.Errorf("execute server-selected coding command %s: %w", label, err)
	}
	if command.Purpose == verificationTest {
		for index := range result.Evidence {
			result.Evidence[index].Kind = evidence.KindTestResult
		}
	}
	if len(result.Evidence) != 1 {
		return operation.Result{}, 0, fmt.Errorf("server-selected coding command %s produced %d evidence rows, expected one", label, len(result.Evidence))
	}
	ids, err := s.persistCodeOwnedEvidenceIDs(result)
	if err != nil {
		return operation.Result{}, 0, fmt.Errorf("persist server-selected coding command %s: %w", label, err)
	}
	if len(ids) != 1 {
		return operation.Result{}, 0, fmt.Errorf("server-selected coding command %s persisted an invalid evidence set", label)
	}
	return result, ids[0], nil
}

func directCodingCommandSucceeded(result operation.Result) bool {
	succeeded, ok := result.Output["succeeded"].(bool)
	return ok && succeeded
}

func directCodingCommandLabel(command testCommand) string {
	return strings.Join(append([]string{command.Name}, command.Args...), " ")
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
