package worker

import (
	"fmt"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func newDirectCodingLaravelProjectStageExecutor(
	session *directCodingSession,
	_ directCodingProgram,
) (directCodingProjectStageExecutor, error) {
	return newDirectCodingLanguageProjectStageExecutor(session, directCodingLanguageStageConfig{
		Language: "php", AdapterID: phpSourceAdapterID, Timeout: directCodingPHPStageTimeout,
		ProjectFragment:    assemblyline.ProjectPHPFragment,
		ValidateFragment:   validateDirectCodingPHPFragment,
		ValidateAcceptance: validateDirectCodingPHPAcceptance,
		TaskCommands:       laravelTaskVerificationCommands,
		FinalCommands:      laravelVerificationCommands,
		CleanupCommands:    laravelCleanupCommands(),
		Repair:             directCodingPHPRepairConfig(),
	})
}

func laravelCleanupCommands() []testCommand {
	return []testCommand{{
		Family: "docker", Name: "docker", Purpose: verificationSetup,
		Timeout: directCodingPHPStageTimeout,
		Args: laravelComposeArgs(
			"down", "--rmi", "local", "--volumes", "--remove-orphans",
		),
	}}
}

func laravelTaskVerificationCommands(
	context assemblyline.ApplicationTaskContext,
	program directCodingProgram,
) ([]testCommand, error) {
	if context.WorkloadSHA256 == "" || context.WorkloadSHA256 != program.Workload.SHA256 {
		return nil, fmt.Errorf("Laravel task verification context differs from program workload authority")
	}
	if err := validateFocusedPHPServiceStateLifetime(program, context); err != nil {
		return nil, fmt.Errorf("validate focused Laravel state authority: %w", err)
	}
	storage, err := focusedPHPServiceStorage(program, context)
	if err != nil {
		return nil, err
	}
	hasState := storage == directCodingServiceStoragePostgreSQL
	if err := validatePHPServiceTaskEndpoint(program, context); err != nil {
		return nil, fmt.Errorf("validate focused Laravel endpoint authority: %w", err)
	}
	pair, _, err := phpServiceTaskPair(program.Coverage, context.Task.TaskID)
	if err != nil {
		return nil, err
	}
	paths, err := phpServiceSourcePaths(program)
	if err != nil {
		return nil, err
	}
	if err := requirePHPServiceSourcePaths(
		paths, "src/Runtime.php", laravelTestBootstrapPath,
		laravelPlatformVerificationPath, pair.ImplementationPath, pair.VerificationPath,
	); err != nil {
		return nil, err
	}
	if hasState {
		if err := requirePHPServiceSourcePaths(paths, phpServiceStateVerificationPath); err != nil {
			return nil, err
		}
	}
	commands := laravelContainerPreparationCommands()
	commands = append(commands, laravelStateSetupCommands(hasState)...)
	commands = append(commands, laravelLintCommands(paths)...)
	commands = append(commands, laravelStatePersistenceCommands(hasState)...)
	commands = append(commands, laravelStateIsolationCommands(hasState)...)
	commands = append(commands, testCommand{
		Family: "docker", Name: "docker", Purpose: verificationTest,
		Args: laravelComposeArgs(
			"run", "--rm", "--no-deps", "app", "php", pair.VerificationPath,
		),
	})
	commands = append(commands, laravelStateIsolationCommands(hasState)...)
	return commands, nil
}

func laravelVerificationCommands(program directCodingProgram) ([]testCommand, error) {
	if err := validateLaravelServiceStateLifetime(program.Workload, program.ServiceState); err != nil {
		return nil, fmt.Errorf("validate Laravel state authority: %w", err)
	}
	if err := program.ServiceEndpoints.ValidateFor(program.Workload); err != nil {
		return nil, fmt.Errorf("validate Laravel endpoint authority: %w", err)
	}
	hasState, err := laravelProgramRequiresPostgreSQL(program)
	if err != nil {
		return nil, err
	}
	paths, err := phpServiceSourcePaths(program)
	if err != nil {
		return nil, err
	}
	if err := requirePHPServiceSourcePaths(
		paths, "routes/web.php", "src/Runtime.php", laravelTestBootstrapPath,
		laravelPlatformVerificationPath, "tests/HttpVerifier.php", "tests/TestRunner.php",
	); err != nil {
		return nil, err
	}
	if hasState {
		if err := requirePHPServiceSourcePaths(paths, phpServiceStateVerificationPath); err != nil {
			return nil, err
		}
	}
	commands := laravelContainerPreparationCommands()
	commands = append(commands, laravelStateSetupCommands(hasState)...)
	commands = append(commands, laravelLintCommands(paths)...)
	commands = append(commands,
		testCommand{
			Family: "docker", Name: "docker", Purpose: verificationTest,
			Args: laravelComposeArgs(
				"run", "--rm", "--no-deps", "app", "php", laravelPlatformVerificationPath,
			),
		},
		testCommand{
			Family: "docker", Name: "docker", Purpose: verificationSetup,
			Timeout: directCodingPHPStageTimeout,
			Args: laravelComposeArgs(
				"up", "--detach", "--wait", "app",
			),
		},
		testCommand{
			Family: "docker", Name: "docker", Purpose: verificationConfig,
			Args: laravelComposeArgs(
				"run", "--rm", "--no-deps", "nginx", "nginx", "-t",
			),
		},
	)
	commands = append(commands, laravelStatePersistenceCommands(hasState)...)
	commands = append(commands, laravelStateIsolationCommands(hasState)...)
	commands = append(commands, testCommand{
		Family: "docker", Name: "docker", Purpose: verificationTest,
		Args: laravelComposeArgs(
			"run", "--rm", "--no-deps", "app", "php", "tests/TestRunner.php",
		),
	})
	commands = append(commands, laravelStateIsolationCommands(hasState)...)
	commands = append(commands,
		testCommand{
			Family: "docker", Name: "docker", Purpose: verificationSetup,
			Timeout: directCodingPHPStageTimeout,
			Args: laravelComposeArgs(
				"up", "--detach", "--wait", "app", "nginx",
			),
		},
		testCommand{
			Family: "docker", Name: "docker", Purpose: verificationTest,
			Args: laravelComposeArgs(
				"run", "--rm", "--no-deps", "app", "php", "tests/HttpVerifier.php",
			),
		},
	)
	commands = append(commands, laravelStateIsolationCommands(hasState)...)
	return commands, nil
}

func laravelProgramRequiresPostgreSQL(program directCodingProgram) (bool, error) {
	storage, err := deriveDirectCodingServiceStoragePlan(program.Workload, program.ServiceState)
	if err != nil {
		return false, err
	}
	required := storage.RequiresPostgreSQL()
	verifierCount := 0
	for _, file := range program.StaticFiles {
		if file.Path == phpServiceStateVerificationPath {
			verifierCount++
		}
	}
	_, hasRuntime := directCodingSourceBlueprintBlock(program.Source, "runtime.state")
	if verifierCount > 1 || required != (verifierCount == 1) || required != hasRuntime {
		return false, fmt.Errorf(
			"Laravel durable-state projection differs from its derived storage authority",
		)
	}
	return required, nil
}

func laravelContainerPreparationCommands() []testCommand {
	return []testCommand{
		{
			Family: "docker", Name: "docker", Purpose: verificationConfig,
			Args: laravelComposeArgs("config", "--quiet"),
		},
		{
			Family: "docker", Name: "docker", Purpose: verificationBuild,
			Timeout: directCodingPHPStageTimeout,
			Args:    laravelComposeArgs("build", "app", "nginx"),
		},
	}
}

func laravelStateSetupCommands(hasState bool) []testCommand {
	if !hasState {
		return nil
	}
	return []testCommand{
		{
			Family: "docker", Name: "docker", Purpose: verificationSetup,
			Timeout: directCodingPHPStageTimeout,
			Args:    laravelComposeArgs("up", "--detach", "--wait", "db"),
		},
		{
			Family: "docker", Name: "docker", Purpose: verificationSetup,
			Args: laravelComposeArgs(
				"run", "--rm", "--no-deps", "app", "php", "artisan", "migrate", "--force",
			),
		},
	}
}

func laravelLintCommands(paths []string) []testCommand {
	commands := make([]testCommand, 0, len(paths))
	for _, sourcePath := range paths {
		commands = append(commands, testCommand{
			Family: "docker", Name: "docker", Purpose: verificationSyntax,
			Args: laravelComposeArgs(
				"run", "--rm", "--no-deps", "app", "php", "-l", sourcePath,
			),
		})
	}
	return commands
}

func laravelStatePersistenceCommands(hasState bool) []testCommand {
	if !hasState {
		return nil
	}
	commands := make([]testCommand, 0, 2)
	for _, mode := range []string{"write", "read"} {
		commands = append(commands, testCommand{
			Family: "docker", Name: "docker", Purpose: verificationTest,
			Args: laravelComposeArgs(
				"run", "--rm", "--no-deps", "app", "php",
				phpServiceStateVerificationPath, mode,
			),
		})
	}
	return commands
}

func laravelStateIsolationCommands(hasState bool) []testCommand {
	if !hasState {
		return nil
	}
	return []testCommand{{
		Family: "docker", Name: "docker", Purpose: verificationSetup,
		Args: laravelComposeArgs(
			"run", "--rm", "--no-deps", "app", "php",
			phpServiceStateVerificationPath, "reset",
		),
	}}
}

func laravelComposeArgs(args ...string) []string {
	prefix := []string{"compose", "--env-file", laravelVerificationEnvPath}
	return append(prefix, args...)
}

func validateLaravelDockerCompose(args []string) (bool, error) {
	prefix := []string{"compose", "--env-file", laravelVerificationEnvPath}
	if len(args) < len(prefix) || !slicesEqualStrings(args[:len(prefix)], prefix) {
		return false, nil
	}
	command := append([]string{"compose"}, args[len(prefix):]...)
	if slicesEqualStrings(command, []string{"compose", "config", "--quiet"}) ||
		slicesEqualStrings(command, []string{"compose", "build", "app", "nginx"}) ||
		slicesEqualStrings(command, []string{"compose", "up", "--detach", "--wait", "db"}) ||
		slicesEqualStrings(command, []string{"compose", "up", "--detach", "--wait", "app"}) ||
		slicesEqualStrings(command, []string{"compose", "up", "--detach", "--wait", "app", "nginx"}) ||
		slicesEqualStrings(command, []string{
			"compose", "down", "--rmi", "local", "--volumes", "--remove-orphans",
		}) || slicesEqualStrings(command, []string{
		"compose", "run", "--rm", "--no-deps", "nginx", "nginx", "-t",
	}) || slicesEqualStrings(command, []string{
		"compose", "run", "--rm", "--no-deps", "app", "php", "artisan", "migrate", "--force",
	}) {
		return true, nil
	}
	phpPrefix := []string{"compose", "run", "--rm", "--no-deps", "app", "php"}
	if len(command) == len(phpPrefix)+1 &&
		slicesEqualStrings(command[:len(phpPrefix)], phpPrefix) &&
		validV3PHPSourcePath(command[len(phpPrefix)]) {
		return true, nil
	}
	if len(command) == len(phpPrefix)+2 &&
		slicesEqualStrings(command[:len(phpPrefix)], phpPrefix) &&
		command[len(phpPrefix)] == "-l" && validV3PHPSourcePath(command[len(phpPrefix)+1]) {
		return true, nil
	}
	if len(command) == len(phpPrefix)+2 &&
		slicesEqualStrings(command[:len(phpPrefix)], phpPrefix) &&
		command[len(phpPrefix)] == phpServiceStateVerificationPath {
		switch command[len(phpPrefix)+1] {
		case "write", "read", "reset":
			return true, nil
		}
	}
	return true, fmt.Errorf("command.run permits only registered Laravel Docker Compose verification commands")
}
