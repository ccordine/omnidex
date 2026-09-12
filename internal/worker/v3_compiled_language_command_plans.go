package worker

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func directCodingJavaScriptCompileCommands(assembly directCodingAssembly) ([]directCodingVerificationCommand, error) {
	paths, err := directCodingCompilationSourcePaths(assembly, "javascript")
	if err != nil {
		return nil, err
	}
	commands := make([]directCodingVerificationCommand, 0, len(paths)+1)
	var imports strings.Builder
	var tests []string
	for _, path := range paths {
		commands = append(commands, directCodingVerificationCommand{
			Argv: []string{"node", "--check", path}, Timeout: defaultDirectCodingVerificationTimeout,
		})
		kind, _ := javascriptArtifactRecognizer(path)
		if kind == assemblyline.TargetArtifactVerification {
			tests = append(tests, path)
			continue
		}
		module, err := json.Marshal("./" + path)
		if err != nil {
			return nil, err
		}
		imports.WriteString("await import(" + string(module) + ");\n")
	}
	commands = append(commands, directCodingVerificationCommand{
		Argv:    []string{"node", "--input-type=module", "--eval", imports.String()},
		Timeout: defaultDirectCodingVerificationTimeout,
	})
	if len(tests) == 0 {
		return nil, fmt.Errorf("JavaScript verification requires exact task-owned test files")
	}
	commands = append(commands, directCodingVerificationCommand{
		Argv:    append([]string{"node", "--test", "--test-concurrency=1"}, tests...),
		Timeout: defaultDirectCodingVerificationTimeout,
	})
	return commands, nil
}

func directCodingRustCompileCommands(assembly directCodingAssembly, output string, complete bool) ([]directCodingVerificationCommand, error) {
	paths, err := directCodingCompilationSourcePaths(assembly, "rust")
	if err != nil {
		return nil, err
	}
	hasTests := false
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.rs") {
			hasTests = true
		}
	}
	if !hasTests {
		return nil, fmt.Errorf("Rust verification requires task-owned test modules")
	}
	environment := []string{
		"CARGO_HOME=" + filepath.Join(output, "cargo"), "CARGO_INCREMENTAL=0",
		"CARGO_TARGET_DIR=" + filepath.Join(output, "target"),
	}
	commands := []directCodingVerificationCommand{
		{
			Argv:        []string{"cargo", "check", "--locked", "--offline", "--lib"},
			Environment: environment, Timeout: defaultDirectCodingVerificationTimeout,
		},
		{
			Argv:        []string{"cargo", "test", "--locked", "--offline", "--lib", "--", "--test-threads=1"},
			Environment: append([]string(nil), environment...), Timeout: defaultDirectCodingVerificationTimeout,
		},
	}
	if complete {
		commands = append(commands, directCodingVerificationCommand{
			Argv:        []string{"cargo", "build", "--locked", "--offline"},
			Environment: append([]string(nil), environment...), Timeout: defaultDirectCodingVerificationTimeout,
		})
	}
	return commands, nil
}

func directCodingJavaCompileCommands(profile directCodingProjectVersionProfile, assembly directCodingAssembly, output string, complete bool) ([]directCodingVerificationCommand, error) {
	paths, err := directCodingCompilationSourcePaths(assembly, "java")
	if err != nil {
		return nil, err
	}
	var tests []string
	for _, sourcePath := range paths {
		if strings.HasSuffix(sourcePath, "Test.java") {
			className := strings.TrimSuffix(filepath.Base(sourcePath), ".java")
			if !javaSourceIdentifier(className) {
				return nil, fmt.Errorf("Java verification requires a valid code-owned test class")
			}
			tests = append(tests, className)
		}
	}
	if len(tests) == 0 {
		return nil, fmt.Errorf("Java verification requires exact task-owned test files")
	}
	release, err := directCodingVersionComponent(profile, "java_release")
	if err != nil {
		return nil, err
	}
	classes := filepath.Join(output, "classes")
	commands := []directCodingVerificationCommand{{
		Argv:    append([]string{"javac", "--release", release, "-encoding", "UTF-8", "-Xlint:all", "-Werror", "-d", classes}, paths...),
		Timeout: defaultDirectCodingVerificationTimeout,
	}}
	for _, className := range tests {
		commands = append(commands, directCodingVerificationCommand{
			Argv:    []string{"java", "-ea", "-cp", classes, className},
			Timeout: defaultDirectCodingVerificationTimeout,
		})
	}
	if complete {
		commands = append(commands, directCodingVerificationCommand{
			Argv:    []string{"jar", "--create", "--file", filepath.Join(output, "application.jar"), "--main-class", "Main", "-C", classes, "."},
			Timeout: defaultDirectCodingVerificationTimeout,
		})
	}
	return commands, nil
}
