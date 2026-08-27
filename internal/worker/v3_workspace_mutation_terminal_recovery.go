package worker

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func (runtime *nativeRuntimeV3) recoverTerminalWorkspaceMutation(
	root string,
	request directCodingRequest,
	snapshot *queue.WorkspaceMutationSnapshot,
	commands []testCommand,
) (string, bool, error) {
	if runtime == nil || runtime.claim == nil || snapshot == nil || snapshot.Terminal == nil {
		return "", true, fmt.Errorf("workspace mutation recovery lacks one exact terminal receipt")
	}
	terminal := snapshot.Terminal
	if snapshot.OperationID != terminal.Result.OperationID ||
		len(terminal.ReceiptSHA256) != 64 {
		return "", true, fmt.Errorf("workspace mutation terminal receipt identity differs")
	}
	current, err := workspacefacts.Capture(runtime.ctx, root)
	if err != nil {
		return "", true, fmt.Errorf("capture terminal workspace mutation reality: %w", err)
	}
	if err := snapshot.Command.Plan.VerifyExpected(current); err != nil {
		return "", true, fmt.Errorf("terminal workspace mutation differs from host reality: %w", err)
	}
	if !terminal.Result.VerificationSucceeded {
		failure := strings.TrimSpace(terminal.Failure)
		if failure == "" {
			failure = "unknown verification failure"
		}
		return "", true, fmt.Errorf(
			"workspace mutation %s has terminal failed verification: %s",
			snapshot.OperationID, failure,
		)
	}
	if snapshot.Command.Plan.GitSourceSnapshotID != "" {
		runtime.svc.emitStepEvent(
			runtime.claim.Authority, "workspace_mutation_recovered",
			fmt.Sprintf("operation=%s expected=%s", snapshot.OperationID, snapshot.Command.Plan.ExpectedStateID),
		)
		return fmt.Sprintf(
			"Recovered verified existing-repository workspace mutation: operation=%s files=%d verification=%s receipt_sha256=%s",
			snapshot.OperationID, len(snapshot.Command.Plan.Files),
			strings.Join(directCodingPrimaryCommandLabels(commands), " | "),
			terminal.ReceiptSHA256,
		), true, nil
	}
	verification, err := directCodingVerificationFromWorkspaceMutation(snapshot, commands)
	if err != nil {
		return "", true, err
	}
	cognition, err := newRecoveredDirectCodingTaskCognition(runtime, request)
	if err != nil {
		return "", true, err
	}
	if err := cognition.CompleteRecoveredWorkspaceMutation(
		root, current, snapshot, verification,
	); err != nil {
		return "", true, err
	}
	runtime.svc.emitStepEvent(
		runtime.claim.Authority, "workspace_mutation_recovered",
		fmt.Sprintf("operation=%s expected=%s", snapshot.OperationID, snapshot.Command.Plan.ExpectedStateID),
	)
	return fmt.Sprintf(
		"Recovered verified coding workspace: operation=%s files=%d verification=%s receipt_sha256=%s",
		snapshot.OperationID, len(snapshot.Command.Plan.Files),
		strings.Join(verification.Commands, " | "), terminal.ReceiptSHA256,
	), true, nil
}

func directCodingVerificationFromWorkspaceMutation(
	snapshot *queue.WorkspaceMutationSnapshot,
	commands []testCommand,
) (directCodingVerification, error) {
	if snapshot == nil || snapshot.Terminal == nil ||
		!snapshot.Terminal.Result.VerificationSucceeded {
		return directCodingVerification{}, fmt.Errorf(
			"restore coding verification requires one successful terminal workspace mutation",
		)
	}
	primary := make([]testCommand, 0, len(commands))
	for _, command := range commands {
		if command.WorkspaceRole == workspaceVerificationPrimary {
			primary = append(primary, command)
		}
	}
	if len(primary) == 0 || len(snapshot.Terminal.Result.CommandEvidenceIDs) < len(primary) {
		return directCodingVerification{}, fmt.Errorf(
			"terminal workspace mutation has incomplete primary verification evidence",
		)
	}
	verification := directCodingVerification{
		Passed:                true,
		TestsPassed:           directCodingCommandsContainTest(primary),
		Commands:              directCodingCommandLabels(primary),
		EvidenceIDs:           append([]int64(nil), snapshot.Terminal.Result.CommandEvidenceIDs[:len(primary)]...),
		MutationOperationID:   snapshot.OperationID,
		MutationReceiptSHA256: snapshot.Terminal.ReceiptSHA256,
	}
	if err := verification.validate(); err != nil {
		return directCodingVerification{}, fmt.Errorf(
			"restore terminal coding verification: %w", err,
		)
	}
	if !verification.TestsPassed {
		return directCodingVerification{}, fmt.Errorf(
			"terminal direct-coding workspace mutation has no test verification authority",
		)
	}
	return verification, nil
}
