package worker

import (
	"fmt"
	"sort"

	"github.com/gryph/omnidex/internal/queue"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
)

// repositoryMutationFilesForExpectedState binds desired post truth to its
// exact source presence. This is the only bridge from staged file states to
// queue-owned physical transitions.
func repositoryMutationFilesForExpectedState(
	source repositoryfacts.Snapshot,
	expected []changeapply.ExpectedFileState,
) ([]queue.RepositoryMutationFile, error) {
	if err := source.Validate(); err != nil {
		return nil, fmt.Errorf("repository mutation source: %w", err)
	}
	files := make(map[string]repositoryfacts.File, len(source.Files))
	for _, file := range source.Files {
		files[file.ID] = file
	}
	changed := make([]queue.RepositoryMutationFile, len(expected))
	seen := make(map[string]struct{}, len(expected))
	for index, post := range expected {
		if err := validateExpectedRepositoryFileState(post); err != nil {
			return nil, fmt.Errorf("repository mutation post file %d: %w", index, err)
		}
		if _, duplicate := seen[post.FileID]; duplicate {
			return nil, fmt.Errorf("repository mutation post file %q is duplicated", post.FileID)
		}
		seen[post.FileID] = struct{}{}
		transition := queue.RepositoryMutationFile{
			FileID: post.FileID, Path: post.Path,
			ExpectedPresent: post.Present, ExpectedSHA256: post.SHA256,
			ExpectedSize: post.Size, ExpectedMode: post.Mode,
		}
		if file, exists := files[post.FileID]; exists {
			if file.Path != post.Path || file.Kind != repositoryfacts.EntryRegular {
				return nil, fmt.Errorf("repository mutation target %q differs from its source authority", post.FileID)
			}
			transition.SourcePresent = true
			transition.SourceSHA256 = file.SHA256
			transition.SourceSize = file.Size
			transition.SourceMode = file.Mode
		} else {
			derivedID, err := repositoryfacts.FileIDForAbsentPath(source, post.Path)
			if err != nil || derivedID != post.FileID {
				return nil, fmt.Errorf("repository mutation created file %q has invalid absent source authority", post.FileID)
			}
		}
		changed[index] = transition
	}
	sort.Slice(changed, func(left, right int) bool {
		return changed[left].FileID < changed[right].FileID
	})
	return changed, nil
}
