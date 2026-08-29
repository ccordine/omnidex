package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
	"github.com/gryph/omnidex/internal/taskstate"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

func (s *directCodingSession) ApplyAndVerify(
	prepared *directCodingPreparedMutation,
) (verification directCodingVerification, begin directCodingCompletionTaskDisposition, resultErr error) {
	if s == nil || s.runtime == nil || s.runtime.svc == nil || s.runtime.svc.repo == nil ||
		prepared == nil || prepared.stage == nil || prepared.mutationCount == 0 {
		return directCodingVerification{}, "", fmt.Errorf("apply direct-coding workspace mutation requires one prepared transaction")
	}
	defer func() {
		resultErr = errors.Join(resultErr, prepared.Cleanup())
	}()
	sandbox, err := newDirectCodingWorkspaceSandbox(
		s.runtime.ctx, prepared.projection, prepared.command.Plan.ID,
	)
	if err != nil {
		return directCodingVerification{}, "", err
	}
	defer func() {
		resultErr = errors.Join(resultErr, sandbox.Cleanup())
	}()
	callbackRan := false
	result, err := s.runtime.svc.repo.ExecuteWorkspaceMutation(
		s.runtime.ctx,
		s.runtime.claim.Authority,
		prepared.command,
		queue.WorkspaceMutationCallbacks{
			Observe: observeWorkspaceMutation,
			Apply: func(ctx context.Context, _ queue.WorkspaceMutationCommand) error {
				_, applyErr := prepared.stage.ApplyVerified(ctx)
				return applyErr
			},
			Verify: func(
				ctx context.Context,
				command queue.WorkspaceMutationCommand,
			) (queue.WorkspaceMutationVerificationResult, error) {
				callbackRan = true
				if err := requireDirectCodingWorkspacePost(ctx, command); err != nil {
					return queue.WorkspaceMutationVerificationResult{}, err
				}
				if err := sandbox.VerifyAuthority(ctx); err != nil {
					return queue.WorkspaceMutationVerificationResult{}, err
				}
				if err := s.completePreparedTreeCognition(prepared); err != nil {
					return queue.WorkspaceMutationVerificationResult{}, err
				}
				begin, err = s.BeginVerification()
				if err != nil {
					return queue.WorkspaceMutationVerificationResult{}, err
				}
				s.recordPreparedWorkspaceMutation(prepared)
				collected, result, collectErr := s.collectDirectCodingWorkspaceVerification(
					ctx, command, prepared.allCommands, sandbox,
				)
				if collectErr != nil {
					return queue.WorkspaceMutationVerificationResult{}, collectErr
				}
				verification = collected
				return result, nil
			},
		},
	)
	if err != nil {
		return directCodingVerification{}, "", err
	}
	if !callbackRan {
		if err := requireDirectCodingWorkspacePost(s.runtime.ctx, prepared.command); err != nil {
			return directCodingVerification{}, "", err
		}
		if err := sandbox.VerifyAuthority(s.runtime.ctx); err != nil {
			return directCodingVerification{}, "", err
		}
		if err := s.completePreparedTreeCognition(prepared); err != nil {
			return directCodingVerification{}, "", err
		}
		if begin, err = s.BeginVerification(); err != nil {
			return directCodingVerification{}, "", err
		}
		s.recordPreparedWorkspaceMutation(prepared)
		if !result.VerificationSucceeded {
			return directCodingVerification{}, "", fmt.Errorf(
				"terminal failed workspace verification replay lacks exact failure receipt authority",
			)
		}
		testEvidence := directCodingCommandsContainTest(prepared.primaryCommands)
		verification = directCodingVerification{
			Passed: true, TestsPassed: testEvidence,
			Commands: directCodingCommandLabels(prepared.primaryCommands),
		}
		sequence := s.nextSequence()
		s.completion.LatestCheckTurn = sequence
		if testEvidence {
			s.completion.LatestTestTurn = sequence
		}
		s.completion.LastTestHadNoTests = false
		s.lastCommands = append([]string(nil), verification.Commands...)
	}
	if len(result.CommandEvidenceIDs) != len(prepared.allCommands) ||
		len(prepared.primaryCommands) > len(result.CommandEvidenceIDs) {
		return directCodingVerification{}, "", fmt.Errorf("workspace mutation returned evidence outside its sealed command plan")
	}
	verification.EvidenceIDs = append(
		[]int64(nil), result.CommandEvidenceIDs[:len(prepared.primaryCommands)]...,
	)
	if err := s.bindWorkspaceMutationTerminal(prepared.command, result, &verification); err != nil {
		return directCodingVerification{}, "", err
	}
	if verification.Passed != result.VerificationSucceeded {
		return directCodingVerification{}, "", fmt.Errorf("direct-coding verification disagrees with journal terminal state")
	}
	return verification, begin, nil
}

func requireDirectCodingWorkspacePost(
	ctx context.Context,
	command queue.WorkspaceMutationCommand,
) error {
	current, err := workspacefacts.Capture(ctx, command.Plan.WorkspaceRoot)
	if err != nil {
		return err
	}
	return command.Plan.VerifyExpected(current)
}

func (s *directCodingSession) completePreparedTreeCognition(
	prepared *directCodingPreparedMutation,
) error {
	for _, path := range prepared.assembly.Directories {
		if err := s.completePreparedTreeLeaf(path, true, prepared.command.Plan.ExpectedStateID); err != nil {
			return err
		}
	}
	for _, file := range prepared.assembly.Files {
		evidence := "file=" + file.Path + " sha256=" + directCodingDigest(file.Content) +
			" workspace_state=" + prepared.command.Plan.ExpectedStateID
		if err := s.completePreparedTreeLeaf(file.Path, false, evidence); err != nil {
			return err
		}
	}
	for _, path := range prepared.assembly.DeletePaths {
		evidence := "file=" + path + " absent=true workspace_state=" +
			prepared.command.Plan.ExpectedStateID
		if err := s.completePreparedTreeLeaf(path, false, evidence); err != nil {
			return err
		}
	}
	return nil
}

func (s *directCodingSession) completePreparedTreeLeaf(
	path string,
	directory bool,
	evidence string,
) error {
	if s.cognition == nil {
		return fmt.Errorf("direct-coding tree completion requires persisted cognition")
	}
	var id taskstate.NodeID
	if directory {
		transition, ok := s.cognition.treeDirs[path]
		if !ok {
			return fmt.Errorf("direct-coding directory %q is not planned", path)
		}
		key, err := directCodingTreeTaskKey(transition)
		if err != nil {
			return err
		}
		id = s.cognition.treeTaskIDs[key]
	} else {
		transition, ok := s.cognition.treeFiles[path]
		if !ok {
			return fmt.Errorf("direct-coding file %q is not planned", path)
		}
		key, err := directCodingTreeTaskKey(transition)
		if err != nil {
			return err
		}
		id = s.cognition.treeTaskIDs[key]
	}
	ledger, err := s.cognition.ledger()
	if err != nil {
		return err
	}
	node, exists := ledger.Node(id)
	if !exists {
		return fmt.Errorf("direct-coding tree leaf %q disappeared", path)
	}
	if node.Status == taskstate.NodeDone {
		return nil
	}
	if node.Status == taskstate.NodeReady {
		if directory {
			err = s.cognition.BeginTreeDirectory(path)
		} else {
			err = s.cognition.BeginTreeFile(path)
		}
		if err != nil {
			return err
		}
	} else if node.Status != taskstate.NodeActive {
		return fmt.Errorf("direct-coding tree leaf %q cannot resume from %q", path, node.Status)
	}
	if directory {
		return s.cognition.CompleteTreeDirectory(path, evidence)
	}
	return s.cognition.CompleteTreeFile(path, evidence)
}

func (s *directCodingSession) recordPreparedWorkspaceMutation(
	prepared *directCodingPreparedMutation,
) {
	if prepared.recorded {
		return
	}
	content := make(map[string]string, len(prepared.assembly.Files))
	for _, file := range prepared.assembly.Files {
		content[file.Path] = file.Content
		s.completion.WrittenSource[file.Path] = file.Content
	}
	for _, transition := range prepared.command.Plan.Files {
		operation := workspaceFileReplace
		switch {
		case !transition.Source.Present:
			operation = workspaceFileCreate
		case !transition.Expected.Present:
			operation = workspaceFileDelete
			delete(s.completion.WrittenSource, transition.Path)
		case content[transition.Path] == "":
			operation = workspaceFileReplace
		}
		s.completion.MutationCount++
		s.completion.LatestMutationTurn = s.nextSequence()
		s.mutationJournal = append(s.mutationJournal, directCodingMutationJournalEntry{
			Path: transition.Path, Operation: operation,
		})
	}
	prepared.recorded = true
}

func directCodingCommandLabels(commands []testCommand) []string {
	labels := make([]string, len(commands))
	for index, command := range commands {
		labels[index] = directCodingCommandLabel(command)
	}
	return labels
}

func directCodingCommandsContainTest(commands []testCommand) bool {
	for _, command := range commands {
		if command.Purpose == verificationTest {
			return true
		}
	}
	return false
}
