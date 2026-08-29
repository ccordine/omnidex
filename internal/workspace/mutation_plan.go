package workspace

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type plannedMutation struct {
	transition MutationFileTransition
	original   []byte
	expected   []byte
	gitTracked bool
}

func PlanMutation(
	ctx context.Context,
	source Snapshot,
	ownerID string,
	desired []DesiredFileState,
) (MutationPlan, error) {
	if ctx == nil {
		return MutationPlan{}, fmt.Errorf("workspace mutation planning requires a context")
	}
	if err := ctx.Err(); err != nil {
		return MutationPlan{}, fmt.Errorf("workspace mutation planning: %w", err)
	}
	if err := source.Validate(); err != nil {
		return MutationPlan{}, fmt.Errorf("workspace mutation source: %w", err)
	}
	if !validMutationOwnerID(ownerID) {
		return MutationPlan{}, fmt.Errorf("workspace mutation requires one exact opaque owner ID")
	}
	if len(desired) == 0 || len(desired) > MaxMutationFiles {
		return MutationPlan{}, fmt.Errorf("workspace mutation planning requires 1-%d desired file states", MaxMutationFiles)
	}
	if err := source.VerifyExact(ctx); err != nil {
		return MutationPlan{}, fmt.Errorf("workspace mutation source authority: %w", err)
	}

	mutations, err := deriveMutationFiles(ctx, source, desired)
	if err != nil {
		return MutationPlan{}, err
	}
	if len(mutations) == 0 {
		return MutationPlan{}, fmt.Errorf("workspace desired file states are already exact and require no mutation")
	}
	sort.Slice(mutations, func(left, right int) bool {
		return mutations[left].transition.Path < mutations[right].transition.Path
	})
	patch, expectedStateID, err := sealMutationDelta(source, mutations)
	if err != nil {
		return MutationPlan{}, err
	}
	patchHash := sha256.Sum256([]byte(patch))
	plan := MutationPlan{
		Schema: MutationPlanSchemaV1, OwnerID: ownerID,
		WorkspaceID: source.WorkspaceID, WorkspaceRoot: source.Root,
		SourceStateID: source.ID, ExpectedStateID: expectedStateID,
		Patch: patch, PatchSHA256: hex.EncodeToString(patchHash[:]),
		Files: make([]MutationFileTransition, len(mutations)),
	}
	if source.Git != nil {
		plan.GitSourceSnapshotID = source.Git.RepositorySnapshotID
		plan.GitRepositoryID = source.Git.RepositoryID
		plan.GitHeadCommit = source.Git.HeadCommit
	}
	for index, mutation := range mutations {
		plan.Files[index] = mutation.transition
	}
	plan.ID = opaqueID(
		"workspace_stage_", plan.OwnerID, plan.WorkspaceID,
		plan.SourceStateID, plan.ExpectedStateID, plan.PatchSHA256,
	)
	if err := plan.ValidateSource(source); err != nil {
		return MutationPlan{}, err
	}
	return plan, nil
}

func deriveMutationFiles(
	ctx context.Context,
	source Snapshot,
	desired []DesiredFileState,
) ([]plannedMutation, error) {
	entries := make(map[string]Entry, len(source.Entries))
	for _, entry := range source.Entries {
		entries[entry.Path] = entry
	}
	seen := make(map[string]struct{}, len(desired))
	mutations := make([]plannedMutation, 0, len(desired))
	for _, state := range desired {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("derive workspace mutation: %w", err)
		}
		if err := validateMutablePath(state.Path); err != nil {
			return nil, err
		}
		if _, duplicate := seen[state.Path]; duplicate {
			return nil, fmt.Errorf("workspace desired file state repeats path %q", state.Path)
		}
		seen[state.Path] = struct{}{}
		mutation, changed, err := deriveMutationFile(ctx, source, entries, state)
		if err != nil {
			return nil, err
		}
		if changed {
			mutations = append(mutations, mutation)
		}
	}
	return mutations, nil
}

func deriveMutationFile(
	ctx context.Context,
	source Snapshot,
	entries map[string]Entry,
	desired DesiredFileState,
) (plannedMutation, bool, error) {
	entry, present := entries[desired.Path]
	if err := validateDesiredSource(desired.Path, entry, present, desired.Source); err != nil {
		return plannedMutation{}, false, err
	}
	var original []byte
	if present {
		if entry.Kind != EntryRegular {
			return plannedMutation{}, false, fmt.Errorf("workspace mutation source %q is not a regular file", desired.Path)
		}
		var err error
		original, err = readExactMutationSource(ctx, source.Root, entry)
		if err != nil {
			return plannedMutation{}, false, err
		}
	}
	if !desired.Present {
		if len(desired.Content) != 0 || desired.Mode != 0 {
			return plannedMutation{}, false, fmt.Errorf("absent workspace desired state %q contains file authority", desired.Path)
		}
		if !present {
			return plannedMutation{}, false, nil
		}
		gitTracked, err := exactGitTrackedPath(ctx, source, desired.Path)
		if err != nil {
			return plannedMutation{}, false, err
		}
		return plannedMutation{
			transition: MutationFileTransition{
				FileID: entry.ID, Path: desired.Path,
				Source: mutationState(entry), Expected: MutationFileState{},
			},
			original: original, gitTracked: gitTracked,
		}, true, nil
	}
	if _, err := BuildFullFileUnifiedPatch(desired.Path, present, original, true, desired.Content); err != nil &&
		!(present && bytes.Equal(original, desired.Content)) {
		return plannedMutation{}, false, err
	}
	if present && desired.Mode != entry.Mode {
		return plannedMutation{}, false, fmt.Errorf(
			"workspace desired state %q must preserve exact source mode %04o",
			desired.Path, entry.Mode,
		)
	}
	if !present && desired.Mode != 0o644 {
		return plannedMutation{}, false, fmt.Errorf("workspace desired create %q requires exact mode 0644", desired.Path)
	}
	expected := MutationFileState{
		Present: true, SHA256: digestMutationBytes(desired.Content),
		Size: int64(len(desired.Content)), Mode: desired.Mode,
	}
	if present && bytes.Equal(original, desired.Content) {
		return plannedMutation{}, false, nil
	}
	return plannedMutation{
		transition: MutationFileTransition{
			FileID: opaqueID("workspace_file_", source.WorkspaceID, desired.Path),
			Path:   desired.Path, Source: mutationState(entry), Expected: expected,
		},
		original: original, expected: append([]byte(nil), desired.Content...),
	}, true, nil
}

func validateDesiredSource(path string, entry Entry, present bool, source *ExactSourceFile) error {
	if !present {
		if source != nil {
			return fmt.Errorf("workspace desired source authority for absent path %q is invalid", path)
		}
		return nil
	}
	if source == nil {
		return fmt.Errorf("workspace desired path %q requires exact source authority", path)
	}
	if source.EntryID != entry.ID || source.SHA256 != entry.SHA256 ||
		source.Size != entry.Size || source.Mode != entry.Mode {
		return fmt.Errorf("workspace desired source authority for %q differs from its snapshot", path)
	}
	return nil
}

func mutationState(entry Entry) MutationFileState {
	if entry.Path == "" {
		return MutationFileState{}
	}
	return MutationFileState{
		Present: true, SHA256: entry.SHA256, Size: entry.Size, Mode: entry.Mode,
	}
}

func readExactMutationSource(ctx context.Context, root string, entry Entry) ([]byte, error) {
	absolute := filepath.Join(root, filepath.FromSlash(entry.Path))
	before, err := os.Lstat(absolute)
	if err != nil || !before.Mode().IsRegular() || before.Mode().Perm() != os.FileMode(entry.Mode) ||
		before.Size() != entry.Size || before.Size() > MaxMutationFileBytes {
		return nil, fmt.Errorf("workspace mutation source %q differs from bounded exact authority", entry.Path)
	}
	if err := rejectMutationSymlinkParents(root, entry.Path); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	content, err := os.ReadFile(absolute)
	if err != nil {
		return nil, fmt.Errorf("read workspace mutation source %q: %w", entry.Path, err)
	}
	after, err := os.Lstat(absolute)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() ||
		before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) ||
		digestMutationBytes(content) != entry.SHA256 {
		return nil, fmt.Errorf("workspace mutation source %q changed while it was read", entry.Path)
	}
	return content, nil
}

func rejectMutationSymlinkParents(root, relative string) error {
	current := root
	parts := strings.Split(relative, "/")
	for _, part := range parts[:len(parts)-1] {
		current = filepath.Join(current, filepath.FromSlash(part))
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("workspace mutation path %q has an invalid parent authority", relative)
		}
	}
	return nil
}

func digestMutationBytes(content []byte) string {
	digest := sha256.Sum256(content)
	return hex.EncodeToString(digest[:])
}
