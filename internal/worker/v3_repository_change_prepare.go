package worker

import (
	"context"
	"errors"
	"fmt"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

type repositoryChangePrepareOperations struct {
	plan   func(context.Context, []changeapply.DesiredFileState) (*changeapply.StagedChange, error)
	verify func(*changeapply.StagedChange) error
}

func (session *directCodingSession) prepareVerifiedExistingRepositoryChange(
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	contract repositoryfacts.ChangeContract,
	candidates map[string]string,
	commands []testCommand,
) (*verifiedRepositoryChangeStage, error) {
	if session == nil || session.runtime == nil || session.runtime.ctx == nil {
		return nil, fmt.Errorf("prepare repository change requires one active coding session")
	}
	operations := repositoryChangePrepareOperations{
		plan: func(
			ctx context.Context,
			desired []changeapply.DesiredFileState,
		) (*changeapply.StagedChange, error) {
			return changeapply.PlanFileStateTransitions(ctx, changeapply.FileStateInput{
				Snapshot: snapshot, Analysis: analysis, OwnerID: contract.ID,
				Desired: append([]changeapply.DesiredFileState(nil), desired...),
			})
		},
		verify: func(stage *changeapply.StagedChange) error {
			authority, err := newRepositoryVerificationAuthority(
				snapshot.ID, contract.ID, commands, stage,
			)
			if err != nil {
				return err
			}
			projection, err := newRepositoryStagedProjection(session.runtime.ctx, stage)
			if err != nil {
				return err
			}
			return session.runExistingRepositoryVerification(
				projection, repositoryVerificationStaged,
				commands, authority, projection.VerifyExact,
			)
		},
	}
	return prepareVerifiedRepositoryChangeWithOperations(
		session.runtime.ctx, snapshot, analysis, contract, candidates, commands, operations,
	)
}

func prepareVerifiedRepositoryChangeWithOperations(
	ctx context.Context,
	snapshot repositoryfacts.Snapshot,
	analysis repositoryfacts.Analysis,
	contract repositoryfacts.ChangeContract,
	candidates map[string]string,
	commands []testCommand,
	operations repositoryChangePrepareOperations,
) (*verifiedRepositoryChangeStage, error) {
	if ctx == nil || operations.plan == nil || operations.verify == nil {
		return nil, fmt.Errorf("repository change preparation operations are incomplete")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := contract.Validate(snapshot, analysis); err != nil {
		return nil, fmt.Errorf("prepare repository change contract: %w", err)
	}
	if _, err := repositoryVerificationPlanID(commands); err != nil {
		return nil, err
	}
	desired, err := changeapply.AssembleExistingGoFileStates(
		snapshot, analysis, contract, candidates,
	)
	if err != nil {
		return nil, err
	}
	stage, err := operations.plan(ctx, desired)
	if err != nil {
		return nil, fmt.Errorf("stage repository change contract: %w", err)
	}
	if err := operations.verify(stage); err != nil {
		return nil, cleanupFailedRepositoryStage(
			stage, fmt.Errorf("verify staged repository change: %w", err),
		)
	}
	prepared, err := newVerifiedRepositoryChangeStage(contract.ID, commands, stage)
	if err != nil {
		return nil, cleanupFailedRepositoryStage(stage, err)
	}
	return prepared, nil
}

func cleanupFailedRepositoryStage(stage *changeapply.StagedChange, cause error) error {
	if stage == nil {
		return cause
	}
	return errors.Join(cause, stage.Cleanup())
}
