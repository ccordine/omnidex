package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/operation"
	"github.com/gryph/omnidex/internal/queue"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func (s *directCodingSession) collectDirectCodingWorkspaceVerification(
	ctx context.Context,
	mutation queue.WorkspaceMutationCommand,
	commands []testCommand,
	sandbox *directCodingWorkspaceSandbox,
) (directCodingVerification, queue.WorkspaceMutationVerificationResult, error) {
	if s == nil || s.runtime == nil || s.runtime.claim == nil ||
		s.specification == nil || s.program == nil || sandbox == nil {
		return directCodingVerification{}, queue.WorkspaceMutationVerificationResult{},
			fmt.Errorf("direct-coding verification requires one complete journal authority")
	}
	if err := validateDirectCodingJournalCommands(mutation, commands); err != nil {
		return directCodingVerification{}, queue.WorkspaceMutationVerificationResult{}, err
	}
	if err := requireDirectCodingWorkspacePost(ctx, mutation); err != nil {
		return directCodingVerification{}, queue.WorkspaceMutationVerificationResult{}, err
	}
	records, diagnostic, testEvidence, err := s.executeDirectCodingVerificationJournal(
		ctx, commands, sandbox,
		func(index int, command testCommand, result operation.Result) (evidence.Record, error) {
			return directCodingVerificationEvidence(mutation, index, command, result)
		},
		func(index int, command testCommand, detail string) evidence.Record {
			return directCodingSkippedVerificationEvidence(mutation, index, command, detail)
		},
	)
	if err != nil {
		return directCodingVerification{}, queue.WorkspaceMutationVerificationResult{}, err
	}
	if err := requireDirectCodingWorkspacePost(ctx, mutation); err != nil {
		return directCodingVerification{}, queue.WorkspaceMutationVerificationResult{}, err
	}
	current, err := workspacefacts.Capture(ctx, mutation.Plan.WorkspaceRoot)
	if err != nil {
		return directCodingVerification{}, queue.WorkspaceMutationVerificationResult{}, err
	}
	verifiedRepositoryID := ""
	if current.Git != nil {
		verifiedRepositoryID = current.Git.RepositorySnapshotID
	}
	passed := diagnostic == nil
	verification := directCodingVerification{
		Passed: passed, TestsPassed: testEvidence,
		Commands: directCodingPrimaryCommandLabels(commands), Diagnostic: diagnostic,
	}
	if passed {
		sequence := s.nextSequence()
		s.completion.LatestCheckTurn = sequence
		if testEvidence {
			s.completion.LatestTestTurn = sequence
		}
		s.completion.LastTestHadNoTests = false
		s.lastCommands = append([]string(nil), verification.Commands...)
	}
	failure := ""
	if diagnostic != nil {
		failure = diagnostic.Stage + ": " + diagnostic.Detail
	}
	return verification, queue.WorkspaceMutationVerificationResult{
		Succeeded: passed, Failure: failure, CommandEvidence: records,
		VerifiedRepositorySnapshotID: verifiedRepositoryID,
	}, nil
}

type directCodingVerificationEvidenceBinder func(
	int,
	testCommand,
	operation.Result,
) (evidence.Record, error)

type directCodingSkippedVerificationBinder func(int, testCommand, string) evidence.Record

func (s *directCodingSession) executeDirectCodingVerificationJournal(
	ctx context.Context,
	commands []testCommand,
	sandbox *directCodingWorkspaceSandbox,
	bindEvidence directCodingVerificationEvidenceBinder,
	bindSkipped directCodingSkippedVerificationBinder,
) ([]evidence.Record, *directCodingDiagnostic, bool, error) {
	if sandbox == nil || bindEvidence == nil || bindSkipped == nil {
		return nil, nil, false, fmt.Errorf("direct-coding verification journal authority is incomplete")
	}
	diagnostic, err := s.directCodingWorkspaceDiagnostic()
	if err != nil {
		return nil, nil, false, err
	}
	records := make([]evidence.Record, 0, len(commands))
	primaryFailed := diagnostic != nil
	testEvidence := false
	lastTestIndex := -1
	var infrastructureErr error
	var cleanupErr error
	for index, command := range commands {
		if command.WorkspaceRole == workspaceVerificationPrimary &&
			(primaryFailed || infrastructureErr != nil) {
			records = append(records, bindSkipped(
				index, command, directCodingDiagnosticText(diagnostic, infrastructureErr),
			))
			continue
		}
		result, executeErr := sandbox.Execute(ctx, command)
		if executeErr != nil {
			infrastructureErr = errors.Join(infrastructureErr, fmt.Errorf(
				"execute journaled verification command %q: %w",
				directCodingCommandLabel(command), executeErr,
			))
			if command.WorkspaceRole == workspaceVerificationPrimary {
				primaryFailed = true
			}
			continue
		}
		record, recordErr := bindEvidence(index, command, result)
		if recordErr != nil {
			infrastructureErr = errors.Join(infrastructureErr, recordErr)
			if command.WorkspaceRole == workspaceVerificationPrimary {
				primaryFailed = true
			}
			continue
		}
		succeeded := directCodingCommandSucceeded(result)
		if command.WorkspaceRole == workspaceVerificationPrimary && command.Purpose == verificationTest {
			lastTestIndex = len(records)
			output := operationResultText(result.Output, "stdout") + "\n" +
				operationResultText(result.Output, "stderr")
			if succeeded && !verificationReportsNoTests(output) {
				testEvidence = true
			}
		}
		if !succeeded {
			detail := fmt.Sprintf(
				"verification command %q failed: %s",
				directCodingCommandLabel(command),
				trimForBudget(directCodingCommandResult(result), 1200),
			)
			if command.WorkspaceRole == workspaceVerificationCleanup {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("verification cleanup %s", detail))
				records = append(records, record)
				continue
			}
			if diagnostic == nil {
				diagnostic = &directCodingDiagnostic{
					Stage:   string(command.WorkspaceRole),
					Command: directCodingCommandLabel(command), Detail: detail,
				}
			} else {
				diagnostic.Detail = trimForBudget(diagnostic.Detail+"; "+detail, 64*1024)
			}
			if command.WorkspaceRole == workspaceVerificationPrimary {
				primaryFailed = true
			}
		}
		records = append(records, record)
	}
	if infrastructureErr != nil || cleanupErr != nil {
		return nil, nil, false, errors.Join(infrastructureErr, cleanupErr)
	}
	if len(records) != len(commands) {
		return nil, nil, false, fmt.Errorf(
			"direct-coding verification evidence differs from its journal plan",
		)
	}
	if s.completion.TestsRequired && !testEvidence && diagnostic == nil {
		diagnostic = &directCodingDiagnostic{
			Stage:   "verify",
			Command: strings.Join(directCodingPrimaryCommandLabels(commands), " && "),
			Detail:  "Verification commands succeeded but reported no executed tests. Add focused tests for the requested success and failure behavior.",
		}
		if lastTestIndex < 0 || lastTestIndex >= len(records) {
			return nil, nil, false, fmt.Errorf(
				"test-required verification journal has no test evidence command",
			)
		}
		records[lastTestIndex].Metadata["succeeded"] = false
		records[lastTestIndex].Warnings = append(records[lastTestIndex].Warnings, diagnostic.Detail)
	}
	return records, diagnostic, testEvidence, nil
}

func (s *directCodingSession) directCodingWorkspaceDiagnostic() (*directCodingDiagnostic, error) {
	diagnostic, err := directCodingProgramWorkspaceDiagnostic(s.root, *s.program, s.initialPaths)
	if err != nil || diagnostic != nil {
		return diagnostic, err
	}
	diagnostic, err = directCodingTargetTreeWorkspaceDiagnostic(s.root, s.program.TargetTree)
	if diagnostic != nil {
		s.runtime.svc.emitStepEvent(
			s.runtime.claim.Authority, "coding_target_tree_validation_failed",
			"diagnostic="+safeLine(diagnostic.Detail, "unknown"),
		)
	}
	return diagnostic, err
}

func validateDirectCodingJournalCommands(
	mutation queue.WorkspaceMutationCommand,
	commands []testCommand,
) error {
	recovered, err := workspaceVerificationCommandsFromPlan(mutation.Verification)
	if err != nil {
		return err
	}
	if len(commands) != len(recovered) {
		return fmt.Errorf("direct-coding commands differ from journal authority")
	}
	for index, command := range commands {
		encoded, err := encodeWorkspaceVerificationCommand(command)
		if err != nil || encoded != mutation.Verification.Commands[index].Command {
			return fmt.Errorf("direct-coding command %d differs from journal authority", index+1)
		}
	}
	return nil
}

func directCodingVerificationEvidence(
	mutation queue.WorkspaceMutationCommand,
	index int,
	command testCommand,
	result operation.Result,
) (evidence.Record, error) {
	if len(result.Evidence) != 1 || index < 0 || index >= len(mutation.Verification.Commands) {
		return evidence.Record{}, fmt.Errorf("verification command %d produced invalid evidence", index+1)
	}
	planned := mutation.Verification.Commands[index]
	record := result.Evidence[0]
	record.ID, record.JobID, record.StepID = 0, mutation.JobID, mutation.StepID
	record.Kind, record.Command = planned.Kind, planned.Command
	record.SourceType, record.SourceRef = "", ""
	record.Metadata = cloneDirectCodingEvidenceMetadata(record.Metadata)
	record.Metadata["succeeded"] = directCodingCommandSucceeded(result)
	record.Metadata["workspace_verification_role"] = string(command.WorkspaceRole)
	return record, nil
}

func cloneDirectCodingEvidenceMetadata(source map[string]any) map[string]any {
	cloned := make(map[string]any, len(source)+2)
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func directCodingSkippedVerificationEvidence(
	mutation queue.WorkspaceMutationCommand,
	index int,
	command testCommand,
	detail string,
) evidence.Record {
	planned := mutation.Verification.Commands[index]
	return evidence.Record{
		JobID: mutation.JobID, StepID: mutation.StepID,
		Kind: planned.Kind, ToolName: "command.run", Command: planned.Command,
		Summary:  "verification command not executed after an earlier authoritative failure",
		Warnings: []string{trimForBudget(detail, 1200)}, Confidence: 1,
		Metadata: map[string]any{
			"execution": false, "succeeded": false,
			"skipped_after_authoritative_failure": true,
			"workspace_verification_role":         string(command.WorkspaceRole),
		},
	}
}

func directCodingPrimaryCommandLabels(commands []testCommand) []string {
	labels := make([]string, 0, len(commands))
	for _, command := range commands {
		if command.WorkspaceRole == workspaceVerificationPrimary {
			labels = append(labels, directCodingCommandLabel(command))
		}
	}
	return labels
}

func directCodingDiagnosticText(diagnostic *directCodingDiagnostic, err error) string {
	if diagnostic != nil {
		return diagnostic.Stage + ": " + diagnostic.Detail
	}
	if err != nil {
		return err.Error()
	}
	return "earlier authoritative verification failure"
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
