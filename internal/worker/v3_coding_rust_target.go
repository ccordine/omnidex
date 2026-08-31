package worker

import (
	"fmt"
	"path"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

var rustCommandLineReservedModules = map[string]struct{}{
	"as": {}, "break": {}, "const": {}, "continue": {}, "crate": {},
	"else": {}, "enum": {}, "extern": {}, "false": {}, "fn": {},
	"for": {}, "if": {}, "impl": {}, "in": {}, "let": {}, "loop": {},
	"match": {}, "mod": {}, "move": {}, "mut": {}, "pub": {}, "ref": {},
	"return": {}, "self": {}, "static": {}, "struct": {}, "super": {},
	"trait": {}, "true": {}, "type": {}, "unsafe": {}, "use": {},
	"where": {}, "while": {}, "async": {}, "await": {}, "dyn": {},
	"abstract": {}, "become": {}, "box": {}, "do": {}, "final": {},
	"macro": {}, "override": {}, "priv": {}, "typeof": {}, "unsized": {},
	"virtual": {}, "yield": {}, "try": {}, "union": {},
}

func validateRustCommandLineTargetTree(target assemblyline.TargetTree) error {
	stack, err := directCodingProjectStackByID(genericRustCommandLineAdapter)
	if err != nil {
		return err
	}
	if err := validateDirectCodingSingleImplementationTargetTree(stack, target, false); err != nil {
		return err
	}
	artifactPath := target.Paths[0]
	if !strings.HasPrefix(artifactPath, "src/") || !strings.HasSuffix(artifactPath, ".rs") {
		return fmt.Errorf("Rust command-line target path %q must be src/<snake>.rs", artifactPath)
	}
	implementationModule := strings.TrimSuffix(strings.TrimPrefix(artifactPath, "src/"), ".rs")
	if strings.Contains(implementationModule, "/") {
		return fmt.Errorf("Rust implementation %q must be one src/<snake>.rs leaf", artifactPath)
	}
	if err := validateRustCommandLineModuleName(implementationModule); err != nil {
		return fmt.Errorf("Rust implementation module: %w", err)
	}
	return nil
}

func validateRustCommandLineModuleName(value string) error {
	if !rustCommandLineModulePattern.MatchString(value) {
		return fmt.Errorf("module %q must be lower snake case", value)
	}
	if _, reserved := rustCommandLineReservedModules[value]; reserved {
		return fmt.Errorf("module %q conflicts with code-owned or reserved Rust authority", value)
	}
	return nil
}

func rustCommandLineTaskImplementationPath(
	coverage assemblyline.ApplicationFileCoveragePlan,
	taskID string,
) (string, error) {
	implementationPath, err := directCodingTaskSingleImplementationPath(coverage, taskID)
	if err != nil {
		return "", err
	}
	target := assemblyline.TargetTree{
		StackID: genericRustCommandLineAdapter,
		Paths:   []string{implementationPath},
	}
	if err := validateRustCommandLineTargetTree(target); err != nil {
		return "", fmt.Errorf("task %s Rust file coverage: %w", taskID, err)
	}
	return implementationPath, nil
}

func rustCommandLineModuleForPath(artifactPath string) (string, error) {
	if path.Dir(artifactPath) != "src" || !strings.HasSuffix(artifactPath, ".rs") {
		return "", fmt.Errorf("Rust module path %q must be one src/<snake>.rs leaf", artifactPath)
	}
	module := strings.TrimSuffix(path.Base(artifactPath), ".rs")
	if err := validateRustCommandLineModuleName(module); err != nil {
		return "", err
	}
	return module, nil
}
