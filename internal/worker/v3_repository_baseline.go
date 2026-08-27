package worker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

const repositoryBaselineSchemaV1 = "omnidex.repository-verification-baseline.v1"

type repositoryBaselineVerificationAuthority struct {
	baselineID       string
	sourceSnapshotID string
	contractID       string
	planID           string
}

type repositoryBaselineIdentity struct {
	Schema           string `json:"schema"`
	SourceSnapshotID string `json:"source_snapshot_id"`
	ContractID       string `json:"contract_id"`
	PlanID           string `json:"plan_id"`
}

type verifiedRepositoryBaseline struct {
	authority repositoryBaselineVerificationAuthority
	commands  []testCommand
}

func newRepositoryBaselineVerificationAuthority(
	sourceSnapshotID string,
	contractID string,
	commands []testCommand,
) (repositoryBaselineVerificationAuthority, error) {
	planID, err := repositoryVerificationPlanID(commands)
	if err != nil {
		return repositoryBaselineVerificationAuthority{}, err
	}
	baselineID, err := repositoryBaselineID(sourceSnapshotID, contractID, planID)
	if err != nil {
		return repositoryBaselineVerificationAuthority{}, err
	}
	authority := repositoryBaselineVerificationAuthority{
		baselineID: baselineID, sourceSnapshotID: sourceSnapshotID,
		contractID: contractID, planID: planID,
	}
	if err := authority.validate(commands); err != nil {
		return repositoryBaselineVerificationAuthority{}, err
	}
	return authority, nil
}

func (authority repositoryBaselineVerificationAuthority) validate(commands []testCommand) error {
	if !validRepositoryVerificationOpaqueID(authority.baselineID, "repository_baseline_") ||
		!validRepositoryVerificationOpaqueID(authority.sourceSnapshotID, "snapshot_") ||
		!validRepositoryVerificationOwnerID(authority.contractID) ||
		!validRepositoryVerificationSHA256(authority.planID) {
		return fmt.Errorf("repository baseline authority contains a malformed identity")
	}
	planID, err := repositoryVerificationPlanID(commands)
	if err != nil {
		return err
	}
	if planID != authority.planID {
		return fmt.Errorf("repository baseline authority belongs to a different ordered plan")
	}
	expected, err := repositoryBaselineID(
		authority.sourceSnapshotID, authority.contractID, authority.planID,
	)
	if err != nil {
		return err
	}
	if expected != authority.baselineID {
		return fmt.Errorf("repository baseline identity differs from its exact authority")
	}
	return nil
}

func (authority repositoryBaselineVerificationAuthority) metadata() map[string]any {
	metadata := map[string]any{
		"repository_verification_baseline_id": authority.baselineID,
		"repository_source_snapshot_id":       authority.sourceSnapshotID,
		"repository_mutation_owner_id":        authority.contractID,
		"repository_verification_plan_id":     authority.planID,
	}
	setRepositoryOwnerMetadata(metadata, authority.contractID)
	return metadata
}

func (authority repositoryBaselineVerificationAuthority) planIdentity() string {
	return authority.planID
}

func (authority repositoryBaselineVerificationAuthority) allowsScope(scope repositoryVerificationScope) bool {
	return scope == repositoryVerificationBaseline
}

func repositoryBaselineID(sourceSnapshotID, contractID, planID string) (string, error) {
	if !validRepositoryVerificationOpaqueID(sourceSnapshotID, "snapshot_") ||
		!validRepositoryVerificationOwnerID(contractID) ||
		!validRepositoryVerificationSHA256(planID) {
		return "", fmt.Errorf("repository baseline identity is incomplete")
	}
	raw, err := json.Marshal(repositoryBaselineIdentity{
		Schema: repositoryBaselineSchemaV1, SourceSnapshotID: sourceSnapshotID,
		ContractID: contractID, PlanID: planID,
	})
	if err != nil {
		return "", fmt.Errorf("encode repository baseline identity: %w", err)
	}
	digest := sha256.Sum256(append([]byte(repositoryBaselineSchemaV1+"\x00"), raw...))
	return "repository_baseline_" + hex.EncodeToString(digest[:]), nil
}

func (session *directCodingSession) proveExistingRepositoryBaseline(
	source repositoryfacts.Snapshot,
	contractID string,
	commands []testCommand,
) (*verifiedRepositoryBaseline, error) {
	if session == nil || session.runtime == nil || session.repositoryIndex == nil {
		return nil, fmt.Errorf("repository baseline requires one active indexed coding session")
	}
	if err := source.Validate(); err != nil {
		return nil, fmt.Errorf("repository baseline source: %w", err)
	}
	if source.ID != session.repositoryIndex.Snapshot.ID || source.Root != session.root {
		return nil, fmt.Errorf("repository baseline source differs from the active immutable index")
	}
	authority, err := newRepositoryBaselineVerificationAuthority(
		source.ID, contractID, commands,
	)
	if err != nil {
		return nil, err
	}
	projection, err := newRepositorySnapshotProjection(source)
	if err != nil {
		return nil, fmt.Errorf("construct exact repository baseline projection: %w", err)
	}
	if err := session.runExistingRepositoryVerification(
		projection, repositoryVerificationBaseline, commands, authority,
		func(ctx context.Context) error {
			return projection.VerifyExact(ctx)
		},
	); err != nil {
		return nil, wrapRepositoryExecutionError("prove exact repository baseline", err)
	}
	return &verifiedRepositoryBaseline{
		authority: authority, commands: cloneTestCommands(commands),
	}, nil
}

func (baseline *verifiedRepositoryBaseline) RequireAuthority(
	sourceSnapshotID string,
	contractID string,
	commands []testCommand,
) error {
	if baseline == nil {
		return fmt.Errorf("repository change requires one accepted exact baseline")
	}
	if baseline.authority.sourceSnapshotID != sourceSnapshotID ||
		baseline.authority.contractID != contractID {
		return fmt.Errorf("repository baseline belongs to different source or contract authority")
	}
	return baseline.authority.validate(commands)
}

func (baseline *verifiedRepositoryBaseline) Commands() []testCommand {
	if baseline == nil {
		return nil
	}
	return cloneTestCommands(baseline.commands)
}

func assertExactRepositoryBaselineSource(
	ctx context.Context,
	root string,
	source repositoryfacts.Snapshot,
) error {
	if ctx == nil {
		return fmt.Errorf("repository baseline exact-source assertion requires a context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("repository baseline exact-source assertion: %w", err)
	}
	if err := source.Validate(); err != nil {
		return fmt.Errorf("repository baseline exact source: %w", err)
	}
	if root != source.Root {
		return fmt.Errorf("repository baseline root differs from its source authority")
	}
	current, err := repositoryfacts.BuildGitSnapshot(
		ctx, root, repositoryfacts.SnapshotOptions{},
	)
	if err != nil {
		return fmt.Errorf("snapshot repository baseline source: %w", err)
	}
	if current.ID != source.ID {
		return fmt.Errorf(
			"repository baseline source changed; expected=%s current=%s",
			source.ID, current.ID,
		)
	}
	return nil
}
