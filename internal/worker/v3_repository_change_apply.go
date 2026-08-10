package worker

import (
	"context"
	"errors"
	"fmt"
	"reflect"

	"github.com/gryph/omnidex/internal/omni"
	"github.com/gryph/omnidex/internal/queue"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
	repositoryindex "github.com/gryph/omnidex/internal/repository/indexing"
)

const maxRepositoryPatchEvidenceBytes = 1 << 20

func (session *directCodingSession) applyExistingRepositoryChangeContract(
	contract repositoryfacts.ChangeContract,
	candidates map[string]string,
	baseline *verifiedRepositoryBaseline,
) (summary string, err error) {
	if session == nil || session.runtime == nil || session.repositoryIndex == nil {
		return "", fmt.Errorf("repository change application requires one active indexed coding session")
	}
	before := *session.repositoryIndex
	analysis, err := exactRepositoryChangeAnalysis(before.Analyses, contract.AnalysisID)
	if err != nil {
		return "", err
	}
	commands, err := existingRepositoryGoVerificationCommands(before.Snapshot, analysis, contract)
	if err != nil {
		return "", err
	}
	if err := baseline.RequireAuthority(before.Snapshot.ID, contract.ID, commands); err != nil {
		return "", fmt.Errorf("authorize repository mutation from clean baseline: %w", err)
	}
	result, err := executeExistingRepositoryMutation(
		session.runtime.ctx, contract.ID, commands, before.Snapshot,
		existingRepositoryMutationExecution{
			prepare: func(context.Context) (*verifiedRepositoryChangeStage, error) {
				return session.prepareVerifiedExistingRepositoryChange(
					before.Snapshot, analysis, contract, candidates, commands,
				)
			},
			mutate: func(ctx context.Context, prepared *verifiedRepositoryChangeStage) error {
				session.runtime.svc.emitStepEvent(
					session.runtime.claim.Authority, "repository_change_staged",
					fmt.Sprintf("contract=%s files=%d", contract.ID, len(prepared.ChangedFileIDs())),
				)
				mutation, mutationErr := existingRepositoryMutationCommand(
					session.runtime, contract, before.Snapshot, commands, prepared,
				)
				if mutationErr != nil {
					return mutationErr
				}
				return session.runtime.svc.repo.ApplyRepositoryMutation(
					ctx, session.runtime.claim.Authority, mutation,
					exactRepositoryMutationClassifier(session.root, before.Snapshot),
					func(applyCtx context.Context) error {
						applyResult, applyErr := prepared.ApplyVerified(applyCtx)
						if applyErr != nil {
							return applyErr
						}
						return validateRepositoryPatchResult(
							before.Snapshot, prepared.ChangedFileIDs(), applyResult.Files,
						)
					},
				)
			},
			verifyAuthoritative: func(
				_ context.Context,
				prepared *verifiedRepositoryChangeStage,
				exactCommands []testCommand,
			) error {
				workspace, authority, authorityErr := newExactAuthoritativeRepositoryVerificationWorkspace(
					session.runtime.ctx, session.root, contract.ID, exactCommands,
					prepared, before.Snapshot,
				)
				if authorityErr != nil {
					return authorityErr
				}
				verificationErr := session.runExistingRepositoryVerification(
					workspace.Root(), repositoryVerificationAuthoritative,
					exactCommands, authority, nil,
					func(assertCtx context.Context) error {
						return errors.Join(
							workspace.VerifyExact(assertCtx),
							assertExactAuthoritativeRepositoryPost(
								assertCtx, session.root, before.Snapshot,
								contract.ID, exactCommands, prepared,
							),
						)
					},
				)
				return errors.Join(verificationErr, workspace.Cleanup())
			},
			refresh: func(context.Context) (repositoryindex.Result, error) {
				return session.runtime.refreshExistingRepositoryIndex(session.root)
			},
		},
	)
	if err != nil {
		return "", err
	}
	broad := commands[len(commands)-1:]
	session.repositoryIndex = &result.Refreshed
	summary = fmt.Sprintf(
		"Completed bounded existing-repository change: targets=%d files=%d verification=%s snapshot=%s",
		len(contract.Targets), len(result.ChangedFileIDs),
		directCodingCommandLabel(broad[0]), result.Refreshed.Snapshot.ID,
	)
	session.runtime.svc.emitStepEvent(
		session.runtime.claim.Authority, "repository_change_completed",
		fmt.Sprintf("contract=%s files=%d snapshot=%s", contract.ID, len(result.ChangedFileIDs), result.Refreshed.Snapshot.ID),
	)
	return summary, nil
}

func existingRepositoryMutationCommand(
	runtime *nativeRuntimeV3,
	contract repositoryfacts.ChangeContract,
	snapshot repositoryfacts.Snapshot,
	commands []testCommand,
	stage *verifiedRepositoryChangeStage,
) (queue.RepositoryMutationCommand, error) {
	if runtime == nil || runtime.claim == nil || runtime.svc == nil || runtime.svc.repo == nil || stage == nil {
		return queue.RepositoryMutationCommand{}, fmt.Errorf("repository mutation requires one active queue-owned claim")
	}
	if err := stage.RequireAuthority(contract.ID, commands); err != nil {
		return queue.RepositoryMutationCommand{}, err
	}
	claim := runtime.claim
	if claim.Job.ID <= 0 || claim.Step.ID <= 0 || claim.Step.Generation <= 0 ||
		claim.Step.Generation != claim.Job.CurrentGeneration || claim.Step.WorkerID == "" ||
		claim.Authority.JobID != claim.Job.ID ||
		claim.Authority.Generation != claim.Step.Generation ||
		claim.Authority.StepID != claim.Step.ID || claim.Authority.Attempt <= 0 ||
		claim.Authority.WorkerID != claim.Step.WorkerID {
		return queue.RepositoryMutationCommand{}, fmt.Errorf("repository mutation claim authority is incomplete or stale")
	}
	files := make(map[string]repositoryfacts.File, len(snapshot.Files))
	for _, file := range snapshot.Files {
		files[file.ID] = file
	}
	expected := stage.ExpectedFiles()
	changed := make([]queue.RepositoryMutationFile, len(expected))
	for index, post := range expected {
		file, exists := files[post.FileID]
		if !exists {
			return queue.RepositoryMutationCommand{}, fmt.Errorf(
				"repository mutation target file %q is absent from its source snapshot", post.FileID,
			)
		}
		changed[index] = queue.RepositoryMutationFile{
			FileID: post.FileID, Path: file.Path,
			SourceSHA256: file.SHA256, SourceSize: file.Size,
			ExpectedSHA256: post.SHA256, ExpectedSize: post.Size,
		}
	}
	return queue.RepositoryMutationCommand{
		JobID: claim.Authority.JobID, StepID: claim.Authority.StepID,
		Generation: claim.Authority.Generation, Attempt: claim.Authority.Attempt,
		WorkerID:   claim.Authority.WorkerID,
		ContractID: contract.ID, StageID: stage.ID(), SourceSnapshotID: snapshot.ID,
		Patch: stage.Patch(), PatchSHA256: stage.PatchSHA256(), ChangedFiles: changed,
	}, nil
}

func exactRepositoryCandidateDeclarations(
	contract repositoryfacts.ChangeContract,
	candidates map[string]string,
) ([]changeapply.CandidateDeclaration, error) {
	if len(candidates) != len(contract.Targets) {
		return nil, fmt.Errorf(
			"repository change candidates have %d declarations for %d exact targets",
			len(candidates), len(contract.Targets),
		)
	}
	declarations := make([]changeapply.CandidateDeclaration, 0, len(contract.Targets))
	for _, target := range contract.Targets {
		candidate, exists := candidates[target.SymbolID]
		if !exists {
			return nil, fmt.Errorf("repository change target %q has no candidate declaration", target.SymbolID)
		}
		declarations = append(declarations, changeapply.CandidateDeclaration{
			SymbolID: target.SymbolID, Declaration: candidate,
		})
	}
	return declarations, nil
}

func validateRepositoryPatchResult(
	snapshot repositoryfacts.Snapshot,
	changedFileIDs []string,
	files []omni.PatchFileResult,
) error {
	expected := make(map[string]string, len(changedFileIDs))
	for _, changedID := range changedFileIDs {
		for _, file := range snapshot.Files {
			if file.ID == changedID {
				expected[file.Path] = "update"
				break
			}
		}
	}
	if len(expected) != len(changedFileIDs) || len(files) != len(expected) {
		return fmt.Errorf("repository patch result differs from its exact changed-file authority")
	}
	for _, file := range files {
		if expected[file.Path] != file.Action {
			return fmt.Errorf("repository patch result contains unexpected action for one target file")
		}
		delete(expected, file.Path)
	}
	if len(expected) != 0 {
		return fmt.Errorf("repository patch result omitted one or more exact target files")
	}
	return nil
}

func validateRefreshedRepositoryChange(
	before repositoryfacts.Snapshot,
	after repositoryindex.Result,
	expectedFiles []changeapply.ExpectedFileState,
) error {
	if !after.Complete || after.Snapshot.ID == before.ID ||
		after.Snapshot.RepositoryID != before.RepositoryID ||
		after.Snapshot.HeadCommit != before.HeadCommit ||
		after.Snapshot.Root != before.Root {
		return fmt.Errorf("refreshed repository index does not represent one complete changed worktree")
	}
	expected := make(map[string]changeapply.ExpectedFileState, len(expectedFiles))
	for _, file := range expectedFiles {
		if file.FileID == "" || file.SHA256 == "" || file.Size < 0 {
			return fmt.Errorf("repository change expected post-patch file authority is invalid")
		}
		if _, duplicate := expected[file.FileID]; duplicate {
			return fmt.Errorf("repository change expected post-patch file authority is duplicated")
		}
		expected[file.FileID] = file
	}
	current := make(map[string]repositoryfacts.File, len(after.Snapshot.Files))
	for _, file := range after.Snapshot.Files {
		current[file.ID] = file
	}
	if len(current) != len(before.Files) {
		return fmt.Errorf("repository verification changed the indexed file inventory outside the contract")
	}
	for _, prior := range before.Files {
		next, exists := current[prior.ID]
		if !exists {
			return fmt.Errorf("repository verification removed an indexed file outside the contract")
		}
		if target, isTarget := expected[prior.ID]; isTarget {
			if prior.SHA256 == target.SHA256 && prior.Size == target.Size {
				return fmt.Errorf("repository target file %q has unchanged expected authority", prior.ID)
			}
			prior.SHA256, prior.Size = target.SHA256, target.Size
			delete(expected, prior.ID)
		}
		if !reflect.DeepEqual(prior, next) {
			return fmt.Errorf("repository verification changed file %q outside the exact contract", prior.ID)
		}
	}
	if len(expected) != 0 || !reflect.DeepEqual(before.Exclusions, after.Snapshot.Exclusions) {
		return fmt.Errorf("repository change refresh omitted a target or changed excluded inventory")
	}
	return nil
}
