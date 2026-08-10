package worker

import (
	"context"
	"errors"
	"fmt"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
	"github.com/gryph/omnidex/internal/specialist"
)

const maxRepositoryGoVerificationCorrectionRounds = 2

type repositoryChangePrepareOperations struct {
	plan    func(context.Context, []changeapply.CandidateDeclaration) (*changeapply.StagedChange, error)
	verify  func(*changeapply.StagedChange, repositoryGoCorrectionOwnership) error
	correct func(repositoryfacts.ChangeTarget, string, string) (string, error)
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
	var correctionModel string
	operations := repositoryChangePrepareOperations{
		plan: func(
			ctx context.Context,
			declarations []changeapply.CandidateDeclaration,
		) (*changeapply.StagedChange, error) {
			return changeapply.Plan(ctx, changeapply.Input{
				Snapshot: snapshot, Analysis: analysis, Contract: contract,
				Candidates: declarations,
			})
		},
		verify: func(
			stage *changeapply.StagedChange,
			ownership repositoryGoCorrectionOwnership,
		) error {
			authority, err := newRepositoryVerificationAuthority(
				snapshot.ID, contract.ID, commands, stage,
			)
			if err != nil {
				return err
			}
			return session.runExistingRepositoryVerification(
				stage.Workspace(), repositoryVerificationStaged,
				commands, authority, &ownership, stage.VerifyExactWorkspace,
			)
		},
		correct: func(
			target repositoryfacts.ChangeTarget,
			current string,
			diagnostic string,
		) (string, error) {
			if correctionModel == "" {
				modelName, err := session.workerModel(
					"coding_fragment_correction", specialist.RoleCodingFragmentCorrectionStation,
				)
				if err != nil {
					return "", err
				}
				correctionModel = modelName
			}
			return runRepositoryGoVerificationCorrection(
				directCodingWorkerRuntime(session), correctionModel, target, current, diagnostic,
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
	if ctx == nil || operations.plan == nil || operations.verify == nil || operations.correct == nil {
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
	current := cloneRepositoryCandidates(candidates)
	if _, err := exactRepositoryCandidateDeclarations(contract, current); err != nil {
		return nil, err
	}
	targets := make(map[string]repositoryfacts.ChangeTarget, len(contract.Targets))
	seen := make(map[string]map[string]struct{}, len(contract.Targets))
	for _, target := range contract.Targets {
		targets[target.SymbolID] = target
		seen[target.SymbolID] = map[string]struct{}{current[target.SymbolID]: {}}
	}
	correctionRounds := 0
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		declarations, err := exactRepositoryCandidateDeclarations(contract, current)
		if err != nil {
			return nil, err
		}
		stage, err := operations.plan(ctx, declarations)
		if err != nil {
			return nil, fmt.Errorf("stage repository change contract: %w", err)
		}
		ownership, err := buildRepositoryGoCorrectionOwnership(
			snapshot, contract, current, stage.Workspace(),
		)
		if err != nil {
			return nil, cleanupFailedRepositoryStage(stage, err)
		}
		verificationErr := operations.verify(stage, ownership)
		if verificationErr == nil {
			prepared, sealErr := newVerifiedRepositoryChangeStage(contract.ID, commands, stage)
			if sealErr != nil {
				return nil, cleanupFailedRepositoryStage(stage, sealErr)
			}
			return prepared, nil
		}
		var failure *repositoryGoVerificationFailure
		if !errors.As(verificationErr, &failure) {
			return nil, cleanupFailedRepositoryStage(stage, verificationErr)
		}
		if correctionRounds >= maxRepositoryGoVerificationCorrectionRounds {
			return nil, cleanupFailedRepositoryStage(stage, fmt.Errorf(
				"repository staged Go verification exceeded correction round limit %d: %w",
				maxRepositoryGoVerificationCorrectionRounds, verificationErr,
			))
		}
		target, exists := targets[failure.targetSymbolID]
		if !exists {
			return nil, cleanupFailedRepositoryStage(stage, fmt.Errorf(
				"repository staged Go verification named unknown target %q",
				failure.targetSymbolID,
			))
		}
		if err := stage.Cleanup(); err != nil {
			return nil, fmt.Errorf("clean failed repository correction stage: %w", err)
		}
		candidate, err := operations.correct(
			target, current[target.SymbolID], failure.diagnostic,
		)
		if err != nil {
			return nil, err
		}
		if candidate == current[target.SymbolID] {
			return nil, fmt.Errorf("repository correction made no progress for target %q", target.SymbolID)
		}
		if _, duplicate := seen[target.SymbolID][candidate]; duplicate {
			return nil, fmt.Errorf("repository correction repeated a prior declaration for target %q", target.SymbolID)
		}
		seen[target.SymbolID][candidate] = struct{}{}
		current[target.SymbolID] = candidate
		correctionRounds++
	}
}

func cleanupFailedRepositoryStage(stage *changeapply.StagedChange, cause error) error {
	if stage == nil {
		return cause
	}
	return errors.Join(cause, stage.Cleanup())
}

func cloneRepositoryCandidates(candidates map[string]string) map[string]string {
	cloned := make(map[string]string, len(candidates))
	for targetID, candidate := range candidates {
		cloned[targetID] = candidate
	}
	return cloned
}
