package worker

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingProgramWorkspaceDiagnostic(root string, program directCodingProgram) (*directCodingDiagnostic, error) {
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
	unexpected, err := directCodingUnexpectedProgramSources(root, expected, program)
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

func directCodingUnexpectedProgramSources(root string, expected map[string]string, program directCodingProgram) ([]string, error) {
	unexpected := make([]string, 0)
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
		if !directCodingProgramSourcePath(relative, program) {
			return nil
		}
		if _, exists := expected[relative]; !exists {
			unexpected = append(unexpected, relative)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("inspect deterministic program sources: %w", err)
	}
	sort.Strings(unexpected)
	return unexpected, nil
}

func directCodingProgramSourcePath(path string, program directCodingProgram) bool {
	extension := strings.ToLower(filepath.Ext(path))
	switch extension {
	case ".ts", ".tsx", ".js", ".jsx", ".css", ".html":
		return true
	default:
		return path == "package.json" || path == "tsconfig.json"
	}
}

func directCodingProgramVerificationCommands(
	specification assemblyline.ApplicationSpecification,
	program directCodingProgram,
) ([]testCommand, error) {
	if specification.Surface == assemblyline.ApplicationSurfaceBrowser && program.Adapter == genericTypeScriptBrowserAdapter {
		return []testCommand{
			{Family: "node", Name: "npm", Args: directCodingNPMInstallArgs()},
			{Family: "node", Name: "npm", Args: []string{"test"}},
			{Family: "node", Name: "npm", Args: []string{"run", "typecheck"}},
			{Family: "node", Name: "npm", Args: []string{"run", "build"}},
		}, nil
	}
	return nil, fmt.Errorf(
		"adapter %s has no code-owned verification command for surface %s",
		program.Adapter,
		specification.Surface,
	)
}

func directCodingNPMInstallArgs() []string {
	return []string{"install", "--ignore-scripts", "--no-audit", "--no-fund", "--package-lock=false"}
}
