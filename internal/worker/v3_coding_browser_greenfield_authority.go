package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/gryph/omnidex/internal/workspace"
)

func validateDirectCodingTypeScriptGreenfieldProgram(
	ctx context.Context,
	reader workspace.Reader,
	program directCodingProgram,
) error {
	if program.Project.Stack.ID != genericTypeScriptBrowserAdapter {
		return nil
	}
	paths := make([]string, 0, len(program.StaticFiles)+len(program.Source.Documents))
	for _, file := range program.StaticFiles {
		paths = append(paths, file.Path)
	}
	for _, document := range program.Source.Documents {
		paths = append(paths, document.Path)
	}
	if err := requireAbsentDirectCodingUnownedPaths(ctx, reader, paths); err != nil {
		return fmt.Errorf("TypeScript browser greenfield authority: %w", err)
	}
	return nil
}

func validateDirectCodingTypeScriptGreenfieldAssembly(
	ctx context.Context,
	reader workspace.Reader,
	assembly directCodingAssembly,
) error {
	paths := make([]string, len(assembly.Files))
	for index, file := range assembly.Files {
		paths[index] = file.Path
	}
	if err := requireAbsentDirectCodingUnownedPaths(ctx, reader, paths); err != nil {
		return fmt.Errorf("TypeScript browser write authority: %w", err)
	}
	return nil
}

func requireAbsentDirectCodingUnownedPaths(ctx context.Context, reader workspace.Reader, paths []string) error {
	if reader == nil {
		return fmt.Errorf("greenfield observation requires acquired workspace access")
	}
	rootInfo, err := reader.Stat(ctx, ".")
	if err != nil || rootInfo.Kind != workspace.EntryDirectory {
		return fmt.Errorf("greenfield authority requires one canonical exact workspace root")
	}
	seen := make(map[string]struct{}, len(paths))
	for _, relative := range paths {
		if _, err := requireExactDirectCodingPath(relative); err != nil {
			return err
		}
		if _, duplicate := seen[relative]; duplicate {
			continue
		}
		seen[relative] = struct{}{}
		if err := requireDirectCodingPathParentsSafe(ctx, reader, relative); err != nil {
			return err
		}
		_, err := reader.Stat(ctx, relative)
		switch {
		case errors.Is(err, os.ErrNotExist):
			continue
		case err != nil:
			return fmt.Errorf("inspect unowned path %q: %w", relative, err)
		default:
			return fmt.Errorf(
				"path %q already exists without an exact managed-file receipt; arbitrary existing-project mutation is unsupported",
				relative,
			)
		}
	}
	return nil
}

func requireDirectCodingPathParentsSafe(ctx context.Context, reader workspace.Reader, relative string) error {
	parent := path.Dir(relative)
	for parent != "." {
		info, err := reader.Stat(ctx, parent)
		switch {
		case errors.Is(err, os.ErrNotExist):
			// A missing child does not prove that an existing ancestor is safe.
		case err != nil:
			return fmt.Errorf("inspect parent of unowned path %q: %w", relative, err)
		case info.Kind != workspace.EntryDirectory:
			return fmt.Errorf(
				"parent %q of greenfield path %q is not one exact directory",
				parent, relative,
			)
		}
		next := path.Dir(parent)
		if next == parent {
			return fmt.Errorf("greenfield path %q escaped its workspace parent", relative)
		}
		parent = next
	}
	return nil
}
