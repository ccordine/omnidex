package changeapply

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
)

const maxExactFileStateTransitions = 8

// PlanFileStateTransitions compares code-owned desired repository truth with
// one exact source snapshot and derives the exact create, replace, or delete
// transition for every desired path.
func PlanFileStateTransitions(
	ctx context.Context,
	input FileStateInput,
) (*StagedChange, error) {
	if ctx == nil {
		return nil, fmt.Errorf("repository file-state staging requires a context")
	}
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("repository file-state staging: %w", err)
	}
	if err := input.Snapshot.Validate(); err != nil {
		return nil, fmt.Errorf("repository file-state snapshot: %w", err)
	}
	if err := input.Analysis.Validate(input.Snapshot); err != nil {
		return nil, fmt.Errorf("repository file-state analysis: %w", err)
	}
	if input.OwnerID == "" || input.OwnerID != strings.TrimSpace(input.OwnerID) || len(input.OwnerID) > 256 {
		return nil, fmt.Errorf("repository file-state staging requires one bounded code-owned objective")
	}
	if len(input.Desired) == 0 || len(input.Desired) > maxExactFileStateTransitions {
		return nil, fmt.Errorf(
			"repository file-state staging requires 1-%d desired states",
			maxExactFileStateTransitions,
		)
	}
	if err := verifyAuthoritativeSnapshot(ctx, input.Snapshot.Root, input.Snapshot.ID); err != nil {
		return nil, fmt.Errorf("repository file-state staging authority: %w", err)
	}
	mutations, err := deriveFileStateMutations(ctx, input)
	if err != nil {
		return nil, err
	}
	return stageAndSealMutations(ctx, input.Snapshot, input.OwnerID, func(workspace string) ([]fileMutation, error) {
		return loadExactFileStateSources(workspace, mutations)
	})
}

func deriveFileStateMutations(ctx context.Context, input FileStateInput) ([]fileMutation, error) {
	files := make(map[string]repositoryfacts.File, len(input.Snapshot.Files))
	for _, file := range input.Snapshot.Files {
		files[file.Path] = file
	}
	mutations := make([]fileMutation, 0, len(input.Desired))
	seen := make(map[string]struct{}, len(input.Desired))
	for _, desired := range input.Desired {
		if _, duplicate := seen[desired.Path]; duplicate {
			return nil, fmt.Errorf("repository desired file state for path %q is duplicated", desired.Path)
		}
		seen[desired.Path] = struct{}{}
		mutation, changed, err := deriveDesiredFileMutation(ctx, input, files, desired)
		if err != nil {
			return nil, err
		}
		if changed {
			mutations = append(mutations, mutation)
		}
	}
	if len(mutations) == 0 {
		return nil, fmt.Errorf("repository desired file states are already exact and require no mutation")
	}
	sort.Slice(mutations, func(left, right int) bool {
		return mutations[left].file.Path < mutations[right].file.Path
	})
	return mutations, nil
}

func deriveDesiredFileMutation(
	ctx context.Context,
	input FileStateInput,
	files map[string]repositoryfacts.File,
	desired DesiredFileState,
) (fileMutation, bool, error) {
	file, sourcePresent := files[desired.Path]
	if desired.Present {
		if sourcePresent {
			mutation, changed, err := deriveSourceToSourceMutation(ctx, input, file, desired)
			return mutation, changed, err
		}
		mutation, err := deriveAbsentToSourceMutation(ctx, input, files, desired)
		return mutation, err == nil, err
	}
	if !sourcePresent {
		if err := validateAlreadyAbsentState(desired); err != nil {
			return fileMutation{}, false, err
		}
		return fileMutation{}, false, nil
	}
	mutation, err := deriveSourceToAbsentMutation(ctx, input, files, desired)
	return mutation, err == nil, err
}

func loadExactFileStateSources(workspace string, planned []fileMutation) ([]fileMutation, error) {
	mutations := append([]fileMutation(nil), planned...)
	for index := range mutations {
		mutation := &mutations[index]
		if !mutation.sourcePresent {
			continue
		}
		absolute := filepath.Join(workspace, filepath.FromSlash(mutation.file.Path))
		original, err := os.ReadFile(absolute)
		if err != nil {
			return nil, fmt.Errorf("read repository file-state source %q: %w", mutation.file.ID, err)
		}
		if int64(len(original)) != mutation.file.Size || digest(original) != mutation.file.SHA256 {
			return nil, fmt.Errorf("repository file-state source %q differs from its exact authority", mutation.file.ID)
		}
		if mutation.file.Language == "go" && repositoryfacts.GeneratedGoSource(original) {
			return nil, fmt.Errorf(
				"repository file-state target %q is generated and cannot be mutated",
				mutation.file.ID,
			)
		}
		if err := validatePatchableSource(mutation.file.ID, original); err != nil {
			return nil, err
		}
		mutation.original = original
	}
	return mutations, nil
}
