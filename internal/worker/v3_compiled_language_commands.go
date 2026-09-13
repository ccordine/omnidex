package worker

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/queue"
)

func verifyDirectCodingCompiledLanguageToolchain(run func(queue.VerificationCommandPhase, directCodingVerificationCommand) (directCodingVerificationCommandResult, error), phase queue.VerificationCommandPhase, profile directCodingProjectVersionProfile) error {
	var tools []string
	switch profile.StackID {
	case genericJavaScriptCommandLineAdapter:
		tools = []string{"node"}
	case genericRustCommandLineAdapter:
		tools = []string{"rustc", "cargo"}
	case genericJavaCommandLineAdapter:
		tools = []string{"javac", "jar"}
	default:
		return fmt.Errorf("stack %s has no compiled-language toolchain", profile.StackID)
	}
	for _, tool := range tools {
		result, err := run(phase, directCodingToolchainVersionCommand(tool))
		if err != nil {
			return err
		}
		if tool == "node" {
			if err := validateDirectCodingToolchainVersion(profile, tool, result.Stdout); err != nil {
				return err
			}
		}
	}
	// Cargo enforces rust-version and edition from the exact manifest; javac
	// enforces the selected API/source release through --release. Their real
	// compilation results, not a second version assertion, establish compatibility.
	return nil
}

func directCodingCompiledLanguageCommands(profile directCodingProjectVersionProfile, assembly directCodingAssembly, root, output string, complete bool) ([]directCodingVerificationCommand, error) {
	if !filepath.IsAbs(root) || filepath.Clean(root) != root || !filepath.IsAbs(output) || filepath.Clean(output) != output {
		return nil, fmt.Errorf("compilation requires exact absolute source and output directories")
	}
	relative, err := filepath.Rel(root, output)
	if err != nil || relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return nil, fmt.Errorf("compiler output must be outside authoritative source")
	}
	// An ancestor output directory would make output cleanup delete the source.
	reverse, err := filepath.Rel(output, root)
	if err != nil || (reverse != ".." && !strings.HasPrefix(reverse, ".."+string(filepath.Separator))) {
		return nil, fmt.Errorf("compiler output must not contain authoritative source")
	}
	switch profile.StackID {
	case genericJavaScriptCommandLineAdapter:
		return directCodingJavaScriptCompileCommands(assembly)
	case genericRustCommandLineAdapter:
		return directCodingRustCompileCommands(assembly, output, complete)
	case genericJavaCommandLineAdapter:
		return directCodingJavaCompileCommands(profile, assembly, output, complete)
	default:
		return nil, fmt.Errorf("stack %s has no compiled-language command implementation", profile.StackID)
	}
}

func directCodingCompilationSourcePaths(assembly directCodingAssembly, adapterID string) ([]string, error) {
	paths := []string{}
	seen := make(map[string]struct{})
	for _, file := range assembly.Files {
		if _, err := requireExactDirectCodingPath(file.Path); err != nil {
			return nil, err
		}
		if _, duplicate := seen[file.Path]; duplicate {
			return nil, fmt.Errorf("compilation repeats source %s", file.Path)
		}
		seen[file.Path] = struct{}{}
		adapter, _, err := directCodingArtifactAdapterForPath(file.Path)
		if err != nil {
			return nil, err
		}
		if adapter.ID == adapterID {
			paths = append(paths, file.Path)
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("compilation has no %s source files", adapterID)
	}
	sort.Strings(paths)
	return paths, nil
}
