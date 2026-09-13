package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"sort"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/workspace"
)

func snapshotDirectCodingTargetTreeOccupation(
	ctx context.Context,
	reader workspace.Reader,
	stack directCodingProjectStack,
) (directCodingTargetTreeOccupation, error) {
	if stack.ID != genericTypeScriptBrowserAdapter {
		return directCodingTargetTreeOccupation{}, nil
	}
	if reader == nil {
		return directCodingTargetTreeOccupation{}, fmt.Errorf("target-tree snapshot requires acquired workspace access")
	}
	rootInfo, err := reader.Stat(ctx, ".")
	if err != nil || rootInfo.Kind != workspace.EntryDirectory {
		return directCodingTargetTreeOccupation{}, fmt.Errorf(
			"target-tree snapshot root is not one exact directory",
		)
	}
	sourceInfo, err := reader.Stat(ctx, "src")
	if errors.Is(err, os.ErrNotExist) {
		return directCodingTargetTreeOccupation{}, nil
	}
	if err != nil {
		return directCodingTargetTreeOccupation{}, fmt.Errorf("inspect target-tree source root: %w", err)
	}
	if sourceInfo.Kind == workspace.EntryLink {
		return directCodingTargetTreeOccupation{}, fmt.Errorf("target-tree source root src is a symbolic link")
	}
	if sourceInfo.Kind != workspace.EntryDirectory {
		if sourceInfo.Kind == workspace.EntryFile {
			return directCodingTargetTreeOccupation{FilePaths: []string{"src"}}, nil
		}
		return directCodingTargetTreeOccupation{}, fmt.Errorf("target-tree source root src is non-regular")
	}
	occupation := directCodingTargetTreeOccupation{DirectoryPaths: []string{"src"}}
	count, after := 0, ""
	for {
		page, err := reader.ReadDirectory(ctx, "src", after, workspace.MaxDirectoryPageEntries)
		if err != nil {
			return directCodingTargetTreeOccupation{}, fmt.Errorf("read target-tree source root: %w", err)
		}
		count += len(page.Entries)
		if count > 4096 || (count == 4096 && page.HasMore) {
			return directCodingTargetTreeOccupation{}, fmt.Errorf("target-tree source root exceeds 4096 entries")
		}
		for _, entry := range page.Entries {
			switch entry.Kind {
			case workspace.EntryDirectory:
				occupation.DirectoryPaths = append(occupation.DirectoryPaths, entry.Path)
			case workspace.EntryFile:
				occupation.FilePaths = append(occupation.FilePaths, entry.Path)
			default:
				return directCodingTargetTreeOccupation{}, fmt.Errorf("target-tree path %s is non-regular", entry.Path)
			}
			after = path.Base(entry.Path)
		}
		if !page.HasMore {
			break
		}
	}
	sort.Strings(occupation.FilePaths)
	sort.Strings(occupation.DirectoryPaths)
	return occupation, nil
}

func reconcileTypeScriptBrowserCompleteTargetTree(
	stack directCodingProjectStack,
	authoritativePaths []string,
	current directCodingTargetTreeOccupation,
) (assemblyline.TargetTree, error) {
	if stack.ProjectCompleteTargetTree == nil {
		return assemblyline.TargetTree{}, fmt.Errorf(
			"TypeScript browser stack lacks one code-owned target-tree projector",
		)
	}
	return stack.ProjectCompleteTargetTree(
		directCodingTargetTreeOccupationFor(
			stack, map[string]struct{}{}, authoritativePaths, current,
		),
	)
}
