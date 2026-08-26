package worker

import (
	"fmt"
	"path"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

var rustCommandLineReservedModules = map[string]struct{}{
	"lib": {}, "main": {}, "runtime": {},
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
	if len(target.Paths) != 2 {
		return fmt.Errorf(
			"project stack %s requires exactly one Rust implementation and integration-test leaf",
			genericRustCommandLineAdapter,
		)
	}
	implementationModule := ""
	verificationModule := ""
	for _, artifactPath := range target.Paths {
		switch {
		case strings.HasPrefix(artifactPath, "src/") && strings.HasSuffix(artifactPath, ".rs") &&
			!strings.HasSuffix(artifactPath, "_test.rs"):
			if implementationModule != "" {
				return fmt.Errorf("Rust command-line target tree repeats its implementation leaf")
			}
			implementationModule = strings.TrimSuffix(strings.TrimPrefix(artifactPath, "src/"), ".rs")
			if strings.Contains(implementationModule, "/") {
				return fmt.Errorf("Rust implementation %q must be one src/<snake>.rs leaf", artifactPath)
			}
		case strings.HasPrefix(artifactPath, "tests/") && strings.HasSuffix(artifactPath, "_test.rs"):
			if verificationModule != "" {
				return fmt.Errorf("Rust command-line target tree repeats its verification leaf")
			}
			verificationModule = strings.TrimSuffix(
				strings.TrimPrefix(artifactPath, "tests/"), "_test.rs",
			)
			if strings.Contains(verificationModule, "/") {
				return fmt.Errorf("Rust verification %q must be one tests/<snake>_test.rs leaf", artifactPath)
			}
		default:
			return fmt.Errorf(
				"Rust command-line target path %q must be src/<snake>.rs or tests/<snake>_test.rs",
				artifactPath,
			)
		}
	}
	if err := validateRustCommandLineModuleName(implementationModule); err != nil {
		return fmt.Errorf("Rust implementation module: %w", err)
	}
	if err := validateRustCommandLineModuleName(verificationModule); err != nil {
		return fmt.Errorf("Rust verification module: %w", err)
	}
	if implementationModule != verificationModule {
		return fmt.Errorf(
			"Rust implementation module %q and verification module %q must match",
			implementationModule, verificationModule,
		)
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

func rustCommandLineTaskPair(
	coverage assemblyline.ApplicationFileCoveragePlan,
	taskID string,
) (directCodingTaskArtifactPair, error) {
	pair, err := directCodingTaskSinglePair(coverage, taskID)
	if err != nil {
		return directCodingTaskArtifactPair{}, err
	}
	target := assemblyline.TargetTree{
		StackID: genericRustCommandLineAdapter,
		Paths:   []string{pair.ImplementationPath, pair.VerificationPath},
	}
	if err := validateRustCommandLineTargetTree(target); err != nil {
		return directCodingTaskArtifactPair{}, fmt.Errorf("task %s Rust file coverage: %w", taskID, err)
	}
	return pair, nil
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

func rustCommandLineTestTarget(artifactPath string) (string, error) {
	if path.Dir(artifactPath) != "tests" || !strings.HasSuffix(artifactPath, "_test.rs") {
		return "", fmt.Errorf("Rust test path %q must be one tests/<snake>_test.rs leaf", artifactPath)
	}
	target := strings.TrimSuffix(path.Base(artifactPath), ".rs")
	module := strings.TrimSuffix(target, "_test")
	if err := validateRustCommandLineModuleName(module); err != nil {
		return "", err
	}
	return target, nil
}
