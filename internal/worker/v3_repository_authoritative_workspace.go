package worker

import (
	"context"
	"fmt"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

type repositoryAuthoritativeVerificationAuthority struct {
	base                 repositoryVerificationAuthority
	projectionSnapshotID string
}

func newRepositoryAuthoritativeVerificationAuthority(
	base repositoryVerificationAuthority,
	projectionSnapshotID string,
	commands []testCommand,
) (repositoryAuthoritativeVerificationAuthority, error) {
	authority := repositoryAuthoritativeVerificationAuthority{
		base: base, projectionSnapshotID: projectionSnapshotID,
	}
	if err := authority.validate(commands); err != nil {
		return repositoryAuthoritativeVerificationAuthority{}, err
	}
	return authority, nil
}

func (authority repositoryAuthoritativeVerificationAuthority) validate(commands []testCommand) error {
	if err := authority.base.validate(commands); err != nil {
		return err
	}
	if !validRepositoryVerificationOpaqueID(authority.projectionSnapshotID, "snapshot_") {
		return fmt.Errorf("authoritative repository verification projection identity is malformed")
	}
	return nil
}

func (authority repositoryAuthoritativeVerificationAuthority) metadata() map[string]any {
	metadata := authority.base.metadata()
	metadata["repository_verification_snapshot_id"] = authority.projectionSnapshotID
	return metadata
}

func (authority repositoryAuthoritativeVerificationAuthority) planIdentity() string {
	return authority.base.planIdentity()
}

func (authority repositoryAuthoritativeVerificationAuthority) allowsScope(
	scope repositoryVerificationScope,
) bool {
	return scope == repositoryVerificationAuthoritative
}

func newExactAuthoritativeRepositoryVerificationProjection(
	ctx context.Context,
	root string,
	contractID string,
	commands []testCommand,
	prepared *verifiedRepositoryChangeStage,
	source repositoryfacts.Snapshot,
) (repositoryWorkspaceProjection, repositoryAuthoritativeVerificationAuthority, error) {
	if prepared == nil {
		return repositoryWorkspaceProjection{}, repositoryAuthoritativeVerificationAuthority{}, fmt.Errorf(
			"authoritative repository verification requires one verified stage",
		)
	}
	post, err := exactAuthoritativeRepositoryPostSnapshot(
		ctx, root, source, contractID, commands, prepared,
	)
	if err != nil {
		return repositoryWorkspaceProjection{}, repositoryAuthoritativeVerificationAuthority{}, err
	}
	projection, err := newRepositorySnapshotProjection(post)
	if err != nil {
		return repositoryWorkspaceProjection{}, repositoryAuthoritativeVerificationAuthority{}, fmt.Errorf(
			"construct authoritative repository post projection: %w", err,
		)
	}
	base, err := prepared.verificationAuthority(source.ID, contractID, commands)
	if err != nil {
		return repositoryWorkspaceProjection{}, repositoryAuthoritativeVerificationAuthority{}, err
	}
	authority, err := newRepositoryAuthoritativeVerificationAuthority(base, post.ID, commands)
	if err != nil {
		return repositoryWorkspaceProjection{}, repositoryAuthoritativeVerificationAuthority{}, err
	}
	return projection, authority, nil
}
