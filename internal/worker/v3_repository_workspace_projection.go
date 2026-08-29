package worker

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	"github.com/gryph/omnidex/internal/repository/changeapply"
	workspacefacts "github.com/gryph/omnidex/internal/workspace"
)

const (
	maxRepositoryWorkspaceProjectionFiles       = 100_000
	maxRepositoryWorkspaceProjectionPathBytes   = 4 * 1024
	maxRepositoryWorkspaceProjectionTargetBytes = 16 * 1024
)

type repositoryWorkspaceProjectionSource string

const (
	repositoryWorkspaceProjectionBase    repositoryWorkspaceProjectionSource = "base"
	repositoryWorkspaceProjectionDelta   repositoryWorkspaceProjectionSource = "delta"
	repositoryWorkspaceProjectionSymlink repositoryWorkspaceProjectionSource = "symlink"
)

type repositoryWorkspaceProjectionFile struct {
	Path       string
	Kind       workspacefacts.EntryKind
	SHA256     string
	Size       int64
	Mode       uint32
	LinkTarget string
	Source     repositoryWorkspaceProjectionSource
}

// repositoryWorkspaceProjection is a logical workspace assembled from one
// exact source snapshot and, for candidate verification only, one bounded
// changed-file delta. It is not a copied directory tree.
type repositoryWorkspaceProjection struct {
	id        string
	source    workspacefacts.Snapshot
	deltaRoot string
	files     []repositoryWorkspaceProjectionFile
	stage     repositoryWorkspaceProjectionStage
}

type repositoryWorkspaceProjectionStage interface {
	VerifyExactDelta(context.Context) error
}

func newRepositorySnapshotProjection(
	snapshot repositoryfacts.Snapshot,
) (repositoryWorkspaceProjection, error) {
	source, err := workspacefacts.FromRepositorySnapshot(snapshot)
	if err != nil {
		return repositoryWorkspaceProjection{}, fmt.Errorf("repository workspace projection snapshot: %w", err)
	}
	return newWorkspaceSnapshotProjection(source)
}

func newWorkspaceSnapshotProjection(
	snapshot workspacefacts.Snapshot,
) (repositoryWorkspaceProjection, error) {
	if err := snapshot.Validate(); err != nil {
		return repositoryWorkspaceProjection{}, fmt.Errorf("workspace projection snapshot: %w", err)
	}
	files := make([]repositoryWorkspaceProjectionFile, len(snapshot.Entries))
	for index, file := range snapshot.Entries {
		source := repositoryWorkspaceProjectionBase
		if file.Kind == workspacefacts.EntrySymlink {
			source = repositoryWorkspaceProjectionSymlink
		}
		files[index] = repositoryWorkspaceProjectionFile{
			Path: file.Path, Kind: file.Kind, SHA256: file.SHA256,
			Size: file.Size, Mode: file.Mode, LinkTarget: file.LinkTarget,
			Source: source,
		}
	}
	projection := repositoryWorkspaceProjection{
		id: snapshot.ID, source: cloneWorkspaceProjectionSnapshot(snapshot), files: files,
	}
	if err := projection.validate(); err != nil {
		return repositoryWorkspaceProjection{}, err
	}
	return projection, nil
}

func newWorkspaceStagedProjection(
	ctx context.Context,
	stage *workspacefacts.StagedMutation,
) (repositoryWorkspaceProjection, error) {
	if stage == nil {
		return repositoryWorkspaceProjection{}, fmt.Errorf("staged workspace projection requires one sealed delta")
	}
	if err := stage.VerifyExactDelta(ctx); err != nil {
		return repositoryWorkspaceProjection{}, err
	}
	plan := stage.Plan()
	expected := make(map[string]workspacefacts.MutationFileState, len(plan.Files))
	for _, transition := range plan.Files {
		if _, duplicate := expected[transition.Path]; duplicate {
			return repositoryWorkspaceProjection{}, fmt.Errorf("staged workspace projection repeats path %q", transition.Path)
		}
		expected[transition.Path] = transition.Expected
	}
	return newWorkspaceDeltaProjection(
		stage.SourceSnapshot(), plan.ID, stage.DeltaRoot(), expected, stage,
	)
}

func newRepositoryStagedProjection(
	ctx context.Context,
	stage *changeapply.StagedChange,
) (repositoryWorkspaceProjection, error) {
	if stage == nil {
		return repositoryWorkspaceProjection{}, fmt.Errorf("staged repository projection requires one sealed delta")
	}
	if err := stage.VerifyExactDelta(ctx); err != nil {
		return repositoryWorkspaceProjection{}, err
	}
	repositorySource := stage.SourceSnapshot()
	source, err := workspacefacts.FromRepositorySnapshot(repositorySource)
	if err != nil {
		return repositoryWorkspaceProjection{}, fmt.Errorf("staged repository projection source: %w", err)
	}
	expected := stage.ExpectedFiles()
	states := make(map[string]workspacefacts.MutationFileState, len(expected))
	for _, state := range expected {
		if err := validateExpectedRepositoryFileState(state); err != nil {
			return repositoryWorkspaceProjection{}, fmt.Errorf("staged repository projection state: %w", err)
		}
		if _, duplicate := states[state.Path]; duplicate {
			return repositoryWorkspaceProjection{}, fmt.Errorf("staged repository projection repeats path %q", state.Path)
		}
		states[state.Path] = workspacefacts.MutationFileState{
			Present: state.Present, SHA256: state.SHA256,
			Size: state.Size, Mode: state.Mode,
		}
	}
	return newWorkspaceDeltaProjection(source, stage.ID(), stage.DeltaRoot(), states, stage)
}

func newWorkspaceDeltaProjection(
	source workspacefacts.Snapshot,
	stageID string,
	deltaRoot string,
	states map[string]workspacefacts.MutationFileState,
	stage repositoryWorkspaceProjectionStage,
) (repositoryWorkspaceProjection, error) {
	if err := source.Validate(); err != nil {
		return repositoryWorkspaceProjection{}, fmt.Errorf("staged workspace projection source: %w", err)
	}
	files := make([]repositoryWorkspaceProjectionFile, 0, len(source.Entries)+len(states))
	for _, file := range source.Entries {
		state, changed := states[file.Path]
		if changed {
			delete(states, file.Path)
			if !state.Present {
				continue
			}
			files = append(files, repositoryWorkspaceProjectionFile{
				Path: file.Path, Kind: workspacefacts.EntryRegular,
				SHA256: state.SHA256, Size: state.Size, Mode: state.Mode,
				Source: repositoryWorkspaceProjectionDelta,
			})
			continue
		}
		entrySource := repositoryWorkspaceProjectionBase
		if file.Kind == workspacefacts.EntrySymlink {
			entrySource = repositoryWorkspaceProjectionSymlink
		}
		files = append(files, repositoryWorkspaceProjectionFile{
			Path: file.Path, Kind: file.Kind, SHA256: file.SHA256,
			Size: file.Size, Mode: file.Mode, LinkTarget: file.LinkTarget,
			Source: entrySource,
		})
	}
	for path, state := range states {
		if !state.Present {
			return repositoryWorkspaceProjection{}, fmt.Errorf("staged workspace projection cannot delete absent path %q", path)
		}
		files = append(files, repositoryWorkspaceProjectionFile{
			Path: path, Kind: workspacefacts.EntryRegular,
			SHA256: state.SHA256, Size: state.Size, Mode: state.Mode,
			Source: repositoryWorkspaceProjectionDelta,
		})
	}
	sort.Slice(files, func(left, right int) bool { return files[left].Path < files[right].Path })
	projection := repositoryWorkspaceProjection{
		id: stageID, source: cloneWorkspaceProjectionSnapshot(source),
		deltaRoot: deltaRoot, files: files, stage: stage,
	}
	if err := projection.validate(); err != nil {
		return repositoryWorkspaceProjection{}, err
	}
	return projection, nil
}

func (projection repositoryWorkspaceProjection) validate() error {
	if projection.id == "" {
		return fmt.Errorf("repository workspace projection requires one exact identity")
	}
	if err := projection.source.Validate(); err != nil {
		return fmt.Errorf("repository workspace projection source: %w", err)
	}
	if projection.files == nil || len(projection.files) > maxRepositoryWorkspaceProjectionFiles {
		return fmt.Errorf(
			"repository workspace projection requires 0-%d exact files; received %d",
			maxRepositoryWorkspaceProjectionFiles, len(projection.files),
		)
	}
	if projection.stage == nil && projection.deltaRoot != "" {
		return fmt.Errorf("repository workspace snapshot projection cannot contain a delta root")
	}
	if projection.stage != nil && projection.deltaRoot == "" {
		return fmt.Errorf("staged repository workspace projection requires one delta root")
	}
	previous := ""
	paths := make(map[string]struct{}, len(projection.files))
	for _, file := range projection.files {
		if file.Path == "" || len([]byte(file.Path)) > maxRepositoryWorkspaceProjectionPathBytes ||
			filepath.IsAbs(file.Path) || filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.Path))) != file.Path ||
			file.Path == "." || file.Path == ".." || strings.HasPrefix(file.Path, "../") ||
			strings.ContainsAny(file.Path, "\x00\r\n") {
			return fmt.Errorf("repository workspace projection path %q is invalid", file.Path)
		}
		if repositoryProjectionPathHasProtectedComponent(file.Path) {
			return fmt.Errorf("repository workspace projection path %q enters protected authority", file.Path)
		}
		if previous != "" && file.Path <= previous {
			return fmt.Errorf("repository workspace projection files must be uniquely sorted")
		}
		previous = file.Path
		paths[file.Path] = struct{}{}
		if file.Size < 0 || file.Mode&^uint32(0o777) != 0 || file.SHA256 == "" {
			return fmt.Errorf("repository workspace projection file %q has invalid exact authority", file.Path)
		}
		switch file.Kind {
		case workspacefacts.EntryRegular:
			if file.LinkTarget != "" ||
				(file.Source != repositoryWorkspaceProjectionBase && file.Source != repositoryWorkspaceProjectionDelta) {
				return fmt.Errorf("repository workspace projection regular file %q has invalid source", file.Path)
			}
		case workspacefacts.EntrySymlink:
			if file.Source != repositoryWorkspaceProjectionSymlink || file.LinkTarget == "" ||
				len([]byte(file.LinkTarget)) > maxRepositoryWorkspaceProjectionTargetBytes {
				return fmt.Errorf("repository workspace projection symlink %q has invalid source", file.Path)
			}
			if err := validateRepositoryProjectionSymlink(file.Path, file.LinkTarget); err != nil {
				return err
			}
		default:
			return fmt.Errorf("repository workspace projection file %q has unsupported kind %q", file.Path, file.Kind)
		}
	}
	for _, file := range projection.files {
		for parent := filepath.ToSlash(filepath.Dir(filepath.FromSlash(file.Path))); parent != "."; parent = filepath.ToSlash(filepath.Dir(filepath.FromSlash(parent))) {
			if _, collision := paths[parent]; collision {
				return fmt.Errorf(
					"repository workspace projection path %q descends through file %q",
					file.Path, parent,
				)
			}
		}
	}
	return nil
}

func (projection repositoryWorkspaceProjection) VerifyExact(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("verify repository workspace projection requires a context")
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("verify repository workspace projection: %w", err)
	}
	if err := projection.validate(); err != nil {
		return err
	}
	if err := projection.source.VerifyExact(ctx); err != nil {
		return err
	}
	if projection.stage != nil {
		if err := projection.stage.VerifyExactDelta(ctx); err != nil {
			return err
		}
	}
	return nil
}

func cloneWorkspaceProjectionSnapshot(snapshot workspacefacts.Snapshot) workspacefacts.Snapshot {
	cloned := snapshot
	cloned.Entries = make([]workspacefacts.Entry, len(snapshot.Entries))
	copy(cloned.Entries, snapshot.Entries)
	cloned.Exclusions = make([]workspacefacts.Exclusion, len(snapshot.Exclusions))
	copy(cloned.Exclusions, snapshot.Exclusions)
	if snapshot.Git != nil {
		binding := *snapshot.Git
		cloned.Git = &binding
	}
	return cloned
}

func validateRepositoryProjectionSymlink(relative, target string) error {
	if filepath.IsAbs(target) {
		return fmt.Errorf("repository workspace projection symlink %q has an absolute target", relative)
	}
	resolved := filepath.ToSlash(filepath.Clean(filepath.Join(
		filepath.Dir(filepath.FromSlash(relative)), target,
	)))
	if resolved == ".." || strings.HasPrefix(resolved, "../") ||
		repositoryProjectionPathHasProtectedComponent(resolved) {
		return fmt.Errorf("repository workspace projection symlink %q escapes exact projected authority", relative)
	}
	return nil
}

func repositoryProjectionPathHasProtectedComponent(relative string) bool {
	for _, component := range strings.Split(relative, "/") {
		if component == ".git" || component == ".omni" {
			return true
		}
	}
	return false
}
