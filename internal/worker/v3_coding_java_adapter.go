package worker

import (
	"fmt"
	"path"
	"sort"
	"strings"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const genericJavaCommandLineAdapter = "java_command_line_capabilities_v1"

func compileGenericJavaCommandLineBlueprint(
	_ string,
	specification assemblyline.ApplicationSpecification,
	skills map[string]directCodingSkillBinding,
	workload assemblyline.FrozenApplicationWorkload,
	capabilities directCodingCapabilityGraph,
	target assemblyline.TargetTree,
	coverage assemblyline.ApplicationFileCoveragePlan,
) (assemblyline.SourceBlueprint, []directCodingFileTask, error) {
	if err := specification.Validate(); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if specification.Surface != assemblyline.ApplicationSurfaceCommandLine {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf(
			"generic Java command-line stack does not support surface %s", specification.Surface,
		)
	}
	if target.StackID != genericJavaCommandLineAdapter {
		return assemblyline.SourceBlueprint{}, nil, fmt.Errorf(
			"Java command-line compiler received target stack %q", target.StackID,
		)
	}
	if err := validateDirectCodingSkillBindings(specification.Requirements, skills); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if err := validateDirectCodingCapabilityGraph(specification.Requirements, capabilities); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	if err := validateJavaCommandLineCoverage(target, workload, coverage); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	contexts, err := directCodingApplicationTaskContexts(applicationWorkloadInput(specification), workload)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	documents, err := genericJavaCommandLineDocuments(
		specification, skills, contexts, capabilities, coverage,
	)
	if err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	blueprint := assemblyline.SourceBlueprint{Documents: documents}
	if err := assemblyline.ValidateJavaSourceBlueprint(blueprint); err != nil {
		return assemblyline.SourceBlueprint{}, nil, err
	}
	return blueprint, genericJavaCommandLineStaticFiles(), nil
}

func genericJavaCommandLineStaticFiles() []directCodingFileTask {
	return []directCodingFileTask{
		{Path: "TestRunner.java", Content: javaCommandLineTestRunnerSource()},
		{Path: "build/classes/.gitignore", Content: "*\n!.gitignore\n"},
	}
}

func validateJavaCommandLineTargetTree(target assemblyline.TargetTree) error {
	stack, err := directCodingProjectStackByID(genericJavaCommandLineAdapter)
	if err != nil {
		return err
	}
	reserved := map[string]struct{}{
		"Main.java": {}, "Runtime.java": {}, "TestRunner.java": {},
	}
	return validateDirectCodingSinglePairTargetTree(stack, target, reserved, true)
}

func validateJavaCommandLineCoverage(
	target assemblyline.TargetTree,
	workload assemblyline.FrozenApplicationWorkload,
	coverage assemblyline.ApplicationFileCoveragePlan,
) error {
	if err := coverage.ValidateFor(target, workload); err != nil {
		return fmt.Errorf("validate Java command-line file coverage: %w", err)
	}
	for _, file := range coverage.Files {
		if len(file.TaskIDs) != 1 {
			return fmt.Errorf(
				"Java command-line source %s requires exactly one task owner", file.Path,
			)
		}
	}
	for _, task := range workload.Tasks {
		if _, err := directCodingTaskSinglePair(coverage, task.ID); err != nil {
			return fmt.Errorf("validate Java command-line task %s coverage: %w", task.ID, err)
		}
	}
	return nil
}

func validateJavaCommandLineAssembly(assembly directCodingAssembly) error {
	files := make(map[string]string, len(assembly.Files))
	for _, file := range assembly.Files {
		files[file.Path] = file.Content
		if strings.HasSuffix(file.Path, ".java") && path.Dir(file.Path) != "." {
			return fmt.Errorf("Java command-line source %s must belong to the root package", file.Path)
		}
	}
	for _, required := range []string{
		"Main.java", "Runtime.java", "TestRunner.java", "build/classes/.gitignore",
	} {
		if _, exists := files[required]; !exists {
			return fmt.Errorf("Java command-line assembly lacks code-owned artifact %s", required)
		}
	}
	return nil
}

func javaCommandLineVerificationCommands(
	program directCodingProgram,
) ([]testCommand, error) {
	targets, err := javaCommandLineVerificationTargets(program)
	if err != nil {
		return nil, err
	}
	return javaCommandLineCommandSet(program, targets, true)
}

func javaCommandLineTaskCommands(
	context assemblyline.ApplicationTaskContext,
	program directCodingProgram,
) ([]testCommand, error) {
	sequence, err := javaCommandLineTaskSequence(program.Workload, context.Task.TaskID)
	if err != nil {
		return nil, err
	}
	return javaCommandLineCommandSet(program, []javaCommandLineVerificationTarget{{
		Class:  fmt.Sprintf("FeatureTest%03d", sequence),
		Method: fmt.Sprintf("verifyFeature%03d", sequence),
	}}, false)
}

type javaCommandLineVerificationTarget struct {
	Class  string
	Method string
}

func javaCommandLineCommandSet(
	program directCodingProgram,
	verificationTargets []javaCommandLineVerificationTarget,
	includeArchive bool,
) ([]testCommand, error) {
	profile, err := directCodingVersionProfileForProgram(program)
	if err != nil {
		return nil, err
	}
	release, err := directCodingVersionComponent(profile, "java_release")
	if err != nil {
		return nil, err
	}
	if len(verificationTargets) == 0 {
		return nil, fmt.Errorf("Java command-line stage has no verification targets")
	}
	for _, target := range verificationTargets {
		if strings.TrimSpace(target.Class) == "" || strings.TrimSpace(target.Method) == "" {
			return nil, fmt.Errorf("Java command-line stage has an incomplete verification target")
		}
	}
	sources, err := javaCommandLineSourcePaths(program)
	if err != nil {
		return nil, err
	}
	compileArgs := append([]string{
		"--release", release, "-Xlint:all", "-Werror", "-d", "build/classes",
	}, sources...)
	commands := []testCommand{
		{Family: "java", Name: "javac", Args: compileArgs, Purpose: verificationBuild},
	}
	for _, target := range verificationTargets {
		commands = append(commands, testCommand{
			Family: "java", Name: "java", Purpose: verificationTest,
			Args: []string{
				"-ea", "-cp", "build/classes", "TestRunner", target.Class, target.Method,
			},
		})
	}
	if includeArchive {
		commands = append(commands, testCommand{
			Family: "java", Name: "jar", Purpose: verificationBuild,
			Args: []string{
				"--create", "--file", "build/application.jar", "--main-class", "Main",
				"-C", "build/classes", ".",
			},
		})
	}
	return commands, nil
}

func javaCommandLineSourcePaths(program directCodingProgram) ([]string, error) {
	seen := make(map[string]struct{})
	add := func(sourcePath string) error {
		normalized, err := normalizeDirectCodingPath(sourcePath)
		if err != nil || normalized != sourcePath {
			return fmt.Errorf("Java source path %q is not normalized", sourcePath)
		}
		if path.Dir(sourcePath) != "." || !strings.HasSuffix(sourcePath, ".java") {
			return fmt.Errorf("Java source path %q must be one root .java file", sourcePath)
		}
		if _, duplicate := seen[sourcePath]; duplicate {
			return fmt.Errorf("Java command-line stage repeats source path %s", sourcePath)
		}
		seen[sourcePath] = struct{}{}
		return nil
	}
	for _, document := range program.Source.Documents {
		if err := add(document.Path); err != nil {
			return nil, err
		}
	}
	for _, file := range program.StaticFiles {
		if !strings.HasSuffix(file.Path, ".java") {
			continue
		}
		if err := add(file.Path); err != nil {
			return nil, err
		}
	}
	paths := make([]string, 0, len(seen))
	for value := range seen {
		paths = append(paths, value)
	}
	sort.Strings(paths)
	if len(paths) < 4 {
		return nil, fmt.Errorf("Java command-line stage has only %d source files", len(paths))
	}
	return paths, nil
}

func javaCommandLineVerificationTargets(
	program directCodingProgram,
) ([]javaCommandLineVerificationTarget, error) {
	targets := make([]javaCommandLineVerificationTarget, len(program.Workload.Tasks))
	for index, task := range program.Workload.Tasks {
		if _, err := javaCommandLineTaskSequence(program.Workload, task.ID); err != nil {
			return nil, err
		}
		targets[index] = javaCommandLineVerificationTarget{
			Class:  fmt.Sprintf("FeatureTest%03d", index+1),
			Method: fmt.Sprintf("verifyFeature%03d", index+1),
		}
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("Java command-line verification has no frozen tasks")
	}
	return targets, nil
}

func javaCommandLineTaskSequence(
	workload assemblyline.FrozenApplicationWorkload,
	taskID string,
) (int, error) {
	for index, task := range workload.Tasks {
		if task.ID == taskID {
			return index + 1, nil
		}
	}
	return 0, fmt.Errorf("Java command-line workload has no task %s", taskID)
}
