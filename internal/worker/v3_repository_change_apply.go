package worker

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/omni"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	goadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
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
				_, mutationErr := session.executeQueuedRepositoryWorkspaceMutation(
					ctx, contract.ID, commands, before.Snapshot, prepared,
				)
				return mutationErr
			},
			verifyAuthoritative: func(
				context.Context,
				*verifiedRepositoryChangeStage,
				[]testCommand,
			) error {
				return nil
			},
			refresh: func(context.Context) (repositoryindex.Result, error) {
				return session.runtime.captureExistingRepositoryIndexWithAnalysis(
					session.root, goadapter.AdapterName,
				)
			},
		},
	)
	if err != nil {
		return "", err
	}
	broad := commands[len(commands)-1:]
	session.repositoryIndex = &result.Refreshed
	changedPaths, err := exactRepositoryChangedPaths(before.Snapshot, result.ChangedFileIDs)
	if err != nil {
		return "", err
	}
	summary = fmt.Sprintf(
		"Completed bounded existing-repository change: targets=%d files=[%s] verification=%s snapshot=%s",
		len(contract.Targets),
		strings.Join(changedPaths, ","),
		directCodingCommandLabel(broad[0]), result.Refreshed.Snapshot.ID,
	)
	session.runtime.svc.emitStepEvent(
		session.runtime.claim.Authority, "repository_change_completed",
		fmt.Sprintf("contract=%s files=%d snapshot=%s", contract.ID, len(result.ChangedFileIDs), result.Refreshed.Snapshot.ID),
	)
	return summary, nil
}

func exactRepositoryChangedPaths(
	snapshot repositoryfacts.Snapshot,
	changedFileIDs []string,
) ([]string, error) {
	pathsByID := make(map[string]string, len(snapshot.Files))
	for _, file := range snapshot.Files {
		pathsByID[file.ID] = file.Path
	}
	paths := make([]string, len(changedFileIDs))
	seen := make(map[string]struct{}, len(changedFileIDs))
	for index, fileID := range changedFileIDs {
		path, exists := pathsByID[fileID]
		if !exists {
			return nil, fmt.Errorf("changed repository file %q is absent from its source snapshot", fileID)
		}
		if _, duplicate := seen[fileID]; duplicate {
			return nil, fmt.Errorf("changed repository file %q is duplicated", fileID)
		}
		seen[fileID] = struct{}{}
		paths[index] = path
	}
	sort.Strings(paths)
	return paths, nil
}

func validateRepositoryFileStatePatchResult(
	snapshot repositoryfacts.Snapshot,
	expectedFiles []changeapply.ExpectedFileState,
	files []omni.PatchFileResult,
) error {
	source := make(map[string]repositoryfacts.File, len(snapshot.Files))
	for _, file := range snapshot.Files {
		source[file.ID] = file
	}
	expected := make(map[string]string, len(expectedFiles))
	for _, state := range expectedFiles {
		if err := validateExpectedRepositoryFileState(state); err != nil {
			return fmt.Errorf("repository patch expected state: %w", err)
		}
		if _, duplicate := expected[state.Path]; duplicate {
			return fmt.Errorf("repository patch expected state repeats one path")
		}
		_, existed := source[state.FileID]
		switch {
		case !existed && state.Present:
			expected[state.Path] = "create"
		case existed && !state.Present:
			expected[state.Path] = "delete"
		case existed && state.Present:
			expected[state.Path] = "update"
		default:
			return fmt.Errorf("repository patch expected state has no legal source-to-post transition")
		}
	}
	if len(files) != len(expected) {
		return fmt.Errorf("repository patch result differs from its exact file-state authority")
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
	return validateExactRepositoryPostInventory(before, after.Snapshot, expectedFiles)
}
