package worker

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

)

func directCodingProgramWorkspaceDiagnostic(
	root string,
	program directCodingProgram,
	initialPaths map[string]directCodingInitialPath,
) (*directCodingDiagnostic, error) {
	if initialPaths == nil {
		return nil, fmt.Errorf("verify program workspace requires captured initial-path authority")
	}
	if _, err := directCodingVersionProfileForProgram(program); err != nil {
		return nil, err
	}
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		return nil, err
	}
	expected := make(map[string]string, len(assembly.Files))
	for _, task := range assembly.Files {
		expected[task.Path] = task.Content
		content, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(task.Path)))
		if os.IsNotExist(readErr) {
			return directCodingStaticFileDiagnostic(task.Path, "code-owned assembled source is missing"), nil
		}
		if readErr != nil {
			return nil, fmt.Errorf("read assembled source %s: %w", task.Path, readErr)
		}
		if !bytes.Equal(content, []byte(task.Content)) {
			return directCodingStaticFileDiagnostic(task.Path, "authoritative source differs from the parser-validated in-memory assembly"), nil
		}
	}
	deleted := make(map[string]struct{}, len(assembly.DeletePaths))
	for _, deletePath := range assembly.DeletePaths {
		deleted[deletePath] = struct{}{}
		_, statErr := os.Lstat(filepath.Join(root, filepath.FromSlash(deletePath)))
		if statErr == nil {
			return directCodingStaticFileDiagnostic(
				deletePath, "code-owned target-tree deletion is still present",
			), nil
		}
		if !os.IsNotExist(statErr) {
			return nil, fmt.Errorf("inspect deleted source %s: %w", deletePath, statErr)
		}
	}
	unexpected, err := directCodingUnexpectedProgramSources(
		root, expected, deleted, program, initialPaths,
	)
	if err != nil {
		return nil, err
	}
	if len(unexpected) > 0 {
		return directCodingStaticFileDiagnostic(
			unexpected[0],
			"deterministic adapter found an undeclared source unit; one authoritative implementation is required",
		), nil
	}
	return nil, nil
}

func directCodingUnexpectedProgramSources(
	root string,
	expected map[string]string,
	deleted map[string]struct{},
	program directCodingProgram,
	initialPaths map[string]directCodingInitialPath,
) ([]string, error) {
	unexpected := make(map[string]struct{})
	seenInitialSources := make(map[string]struct{})
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", ".omni", "vendor", "node_modules", "dist", "build", "target":
				if path != root {
					return filepath.SkipDir
				}
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		sourcePath, err := directCodingProgramSourcePath(relative, program)
		if err != nil {
			return err
		}
		if !sourcePath {
			return nil
		}
		if _, exists := expected[relative]; !exists {
			baseline, existed := initialPaths[relative]
			if !existed {
				unexpected[relative] = struct{}{}
				return nil
			}
			seenInitialSources[relative] = struct{}{}
			digest, err := directCodingWorkspacePathSHA256(root, relative)
			if err != nil {
				return err
			}
			if digest != baseline.SHA256 {
				unexpected[relative] = struct{}{}
			}
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inspect deterministic program sources: %w", err)
	}
	for initialPath := range initialPaths {
		if _, assembled := expected[initialPath]; assembled {
			continue
		}
		if _, intentionallyDeleted := deleted[initialPath]; intentionallyDeleted {
			continue
		}
		sourcePath, err := directCodingProgramSourcePath(initialPath, program)
		if err != nil {
			return nil, err
		}
		if !sourcePath {
			continue
		}
		if _, seen := seenInitialSources[initialPath]; !seen {
			unexpected[initialPath] = struct{}{}
		}
	}
	paths := make([]string, 0, len(unexpected))
	for path := range unexpected {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return paths, nil
}

func directCodingProgramSourcePath(path string, program directCodingProgram) (bool, error) {
	stack, err := directCodingProjectStackByID(program.StackID)
	if err != nil {
		return false, err
	}
	adapter, _, recognized, err := recognizeDirectCodingArtifactAdapterForPath(path)
	if err != nil || !recognized {
		return false, err
	}
	for _, adapterID := range stack.ArtifactAdapterIDs {
		if adapter.ID == adapterID {
			return true, nil
		}
	}
	return false, nil
}

func directCodingNPMInstallArgs() []string {
	return []string{"ci", "--ignore-scripts", "--no-audit", "--no-fund"}
}
