package worker

import (
	"context"
	"errors"
	"fmt"

	"github.com/gryph/omnidex/internal/queue"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

type workspaceRepositoryVerificationAuthority struct {
	ownerID             string
	sourceSnapshotID    string
	stageID             string
	patchSHA256         string
	expectedStateID     string
	verificationPlanID  string
	postRepositoryState string
}

func newWorkspaceRepositoryVerificationAuthority(
	command queue.WorkspaceMutationCommand,
	commands []testCommand,
	postRepositorySnapshotID string,
) (workspaceRepositoryVerificationAuthority, error) {
	if err := command.Plan.Validate(); err != nil {
		return workspaceRepositoryVerificationAuthority{}, err
	}
	if !validRepositoryVerificationOwnerID(command.Plan.OwnerID) ||
		!validRepositoryVerificationOpaqueID(command.Plan.GitSourceSnapshotID, "snapshot_") ||
		!validRepositoryVerificationOpaqueID(postRepositorySnapshotID, "snapshot_") {
		return workspaceRepositoryVerificationAuthority{}, fmt.Errorf("workspace repository verification authority is incomplete")
	}
	if len(commands) != len(command.Verification.Commands) {
		return workspaceRepositoryVerificationAuthority{}, fmt.Errorf("workspace repository verification commands differ from journal authority")
	}
	for index, item := range commands {
		encoded, err := encodeWorkspaceVerificationCommand(item)
		if err != nil || encoded != command.Verification.Commands[index].Command {
			return workspaceRepositoryVerificationAuthority{}, fmt.Errorf(
				"workspace repository verification command %d differs from journal authority", index+1,
			)
		}
	}
	planID, err := repositoryVerificationPlanID(commands)
	if err != nil {
		return workspaceRepositoryVerificationAuthority{}, err
	}
	authority := workspaceRepositoryVerificationAuthority{
		ownerID:          command.Plan.OwnerID,
		sourceSnapshotID: command.Plan.GitSourceSnapshotID,
		stageID:          command.Plan.ID, patchSHA256: command.Plan.PatchSHA256,
		expectedStateID:    command.Plan.ExpectedStateID,
		verificationPlanID: planID, postRepositoryState: postRepositorySnapshotID,
	}
	if err := authority.validate(commands); err != nil {
		return workspaceRepositoryVerificationAuthority{}, err
	}
	return authority, nil
}

func (authority workspaceRepositoryVerificationAuthority) validate(commands []testCommand) error {
	if !validRepositoryVerificationOwnerID(authority.ownerID) ||
		!validRepositoryVerificationOpaqueID(authority.sourceSnapshotID, "snapshot_") ||
		!validRepositoryVerificationOpaqueID(authority.stageID, "workspace_stage_") ||
		!validRepositoryVerificationSHA256(authority.patchSHA256) ||
		!validRepositoryVerificationOpaqueID(authority.expectedStateID, "workspace_state_") ||
		!validRepositoryVerificationSHA256(authority.verificationPlanID) ||
		!validRepositoryVerificationOpaqueID(authority.postRepositoryState, "snapshot_") {
		return fmt.Errorf("workspace repository verification authority is malformed")
	}
	planID, err := repositoryVerificationPlanID(commands)
	if err != nil {
		return err
	}
	if planID != authority.verificationPlanID {
		return fmt.Errorf("workspace repository verification belongs to a different command plan")
	}
	return nil
}

func (authority workspaceRepositoryVerificationAuthority) metadata() map[string]any {
	metadata := map[string]any{
		"workspace_mutation_owner_id":         authority.ownerID,
		"workspace_source_snapshot_id":        authority.sourceSnapshotID,
		"workspace_mutation_stage_id":         authority.stageID,
		"workspace_mutation_patch_sha256":     authority.patchSHA256,
		"workspace_expected_state_id":         authority.expectedStateID,
		"repository_verification_plan_id":     authority.verificationPlanID,
		"repository_verification_snapshot_id": authority.postRepositoryState,
		"repository_mutation_owner_id":        authority.ownerID,
		"repository_source_snapshot_id":       authority.sourceSnapshotID,
	}
	setRepositoryOwnerMetadata(metadata, authority.ownerID)
	return metadata
}

func (authority workspaceRepositoryVerificationAuthority) planIdentity() string {
	return authority.verificationPlanID
}

func (authority workspaceRepositoryVerificationAuthority) allowsScope(
	scope repositoryVerificationScope,
) bool {
	return scope == repositoryVerificationAuthoritative
}

func (session *directCodingSession) verifyAppliedRepositoryWorkspaceMutation(
	ctx context.Context,
	command queue.WorkspaceMutationCommand,
	commands []testCommand,
) (queue.WorkspaceMutationVerificationResult, error) {
	current, err := workspacefacts.Capture(ctx, command.Plan.WorkspaceRoot)
	if err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	if err := command.Plan.VerifyExpected(current); err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	if current.Git == nil {
		return queue.WorkspaceMutationVerificationResult{}, fmt.Errorf("repository verification requires exact Git post authority")
	}
	projection, err := newWorkspaceSnapshotProjection(current)
	if err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	authority, err := newWorkspaceRepositoryVerificationAuthority(
		command, commands, current.Git.RepositorySnapshotID,
	)
	if err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	records, failure, err := session.collectExistingRepositoryVerification(
		projection, repositoryVerificationAuthoritative, commands, authority,
		func(assertCtx context.Context) error {
			observed, captureErr := workspacefacts.Capture(assertCtx, command.Plan.WorkspaceRoot)
			return errors.Join(
				projection.VerifyExact(assertCtx), captureErr,
				command.Plan.VerifyExpected(observed),
			)
		},
	)
	if err != nil {
		return queue.WorkspaceMutationVerificationResult{}, err
	}
	return queue.WorkspaceMutationVerificationResult{
		Succeeded: failure == "", Failure: failure,
		CommandEvidence:              records,
		VerifiedRepositorySnapshotID: current.Git.RepositorySnapshotID,
	}, nil
}
