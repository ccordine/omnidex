package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/evidence"
	"github.com/gryph/omnidex/internal/queue"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func (session *directCodingSession) verifyRecoveredPlainWorkspaceMutation(
	ctx context.Context,
	command queue.WorkspaceMutationCommand,
	commands []testCommand,
) (result queue.WorkspaceMutationVerificationResult, resultErr error) {
	if session == nil || session.runtime == nil || command.Plan.GitSourceSnapshotID != "" {
		return result, fmt.Errorf("plain workspace mutation recovery requires exact non-Git authority")
	}
	current, err := workspacefacts.Capture(ctx, command.Plan.WorkspaceRoot)
	if err != nil {
		return result, err
	}
	if err := command.Plan.VerifyExpected(current); err != nil {
		return result, err
	}
	projection, err := newWorkspaceSnapshotProjection(current)
	if err != nil {
		return result, err
	}
	sandbox, err := newDirectCodingWorkspaceSandbox(ctx, projection, command.Plan.ID)
	if err != nil {
		return result, err
	}
	defer func() {
		resultErr = errors.Join(resultErr, sandbox.Cleanup())
	}()
	if err := sandbox.VerifyAuthority(ctx); err != nil {
		return result, err
	}
	return collectRecoveredWorkspaceVerification(ctx, sandbox, command, commands)
}

func collectRecoveredWorkspaceVerification(
	ctx context.Context,
	sandbox *directCodingWorkspaceSandbox,
	command queue.WorkspaceMutationCommand,
	commands []testCommand,
) (queue.WorkspaceMutationVerificationResult, error) {
	if sandbox == nil {
		return queue.WorkspaceMutationVerificationResult{}, fmt.Errorf("recovered workspace verification requires one sandbox")
	}
	if err := validateDirectCodingJournalCommands(command, commands); err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	records := make([]evidence.Record, 0, len(commands))
	primaryFailed := false
	testEvidence := false
	lastTestIndex := -1
	failure := ""
	var cleanupErr error
	for index, commandItem := range commands {
		if commandItem.WorkspaceRole == workspaceVerificationPrimary && primaryFailed {
			records = append(records, directCodingSkippedVerificationEvidence(
				command, index, commandItem, failure,
			))
			continue
		}
		execution, err := sandbox.Execute(ctx, commandItem)
		if err != nil {
			return queue.WorkspaceMutationVerificationResult{}, fmt.Errorf(
				"execute recovered verification command %q: %w",
				directCodingCommandLabel(commandItem), err,
			)
		}
		record, err := directCodingVerificationEvidence(command, index, commandItem, execution)
		if err != nil {
			return queue.WorkspaceMutationVerificationResult{}, err
		}
		succeeded := directCodingCommandSucceeded(execution)
		if commandItem.WorkspaceRole == workspaceVerificationPrimary &&
			commandItem.Purpose == verificationTest {
			lastTestIndex = len(records)
			output := operationResultText(execution.Output, "stdout") + "\n" +
				operationResultText(execution.Output, "stderr")
			if succeeded && !verificationReportsNoTests(output) {
				testEvidence = true
			}
		}
		if !succeeded {
			detail := fmt.Sprintf(
				"verification command %q failed: %s",
				directCodingCommandLabel(commandItem),
				trimForBudget(directCodingCommandResult(execution), 1200),
			)
			if commandItem.WorkspaceRole == workspaceVerificationCleanup {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("verification cleanup %s", detail))
				records = append(records, record)
				continue
			}
			primaryFailed = true
			if failure == "" {
				failure = detail
			} else {
				failure = trimForBudget(failure+"; "+detail, 64*1024)
			}
		}
		records = append(records, record)
	}
	if cleanupErr != nil {
		return queue.WorkspaceMutationVerificationResult{}, cleanupErr
	}
	if len(records) != len(commands) {
		return queue.WorkspaceMutationVerificationResult{}, fmt.Errorf(
			"recovered workspace verification evidence differs from its command plan",
		)
	}
	if !testEvidence && failure == "" {
		if lastTestIndex < 0 || lastTestIndex >= len(records) {
			return queue.WorkspaceMutationVerificationResult{}, fmt.Errorf(
				"recovered direct-coding verification plan has no test command",
			)
		}
		failure = "verification commands succeeded but reported no executed tests"
		records[lastTestIndex].Metadata["succeeded"] = false
		records[lastTestIndex].Warnings = append(
			records[lastTestIndex].Warnings, strings.TrimSpace(failure),
		)
	}
	if err := sandbox.VerifyAuthority(ctx); err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	current, err := workspacefacts.Capture(ctx, command.Plan.WorkspaceRoot)
	if err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	if err := command.Plan.VerifyExpected(current); err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	return queue.WorkspaceMutationVerificationResult{
		Succeeded: failure == "", Failure: failure, CommandEvidence: records,
	}, nil
}
