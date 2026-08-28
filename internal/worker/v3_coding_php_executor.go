package worker

import (
	"fmt"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

const directCodingPHPStageTimeout = 5 * time.Minute

func newDirectCodingPHPProjectStageExecutor(
	session *directCodingSession,
	_ directCodingProgram,
) (directCodingProjectStageExecutor, error) {
	return newDirectCodingLanguageProjectStageExecutor(session, directCodingLanguageStageConfig{
		Language: "php", AdapterID: "php", Timeout: directCodingPHPStageTimeout,
		ValidateFragment:   validateDirectCodingPHPFragment,
		ValidateAcceptance: validateDirectCodingPHPAcceptance,
		TaskCommands:       phpServiceTaskVerificationCommands,
		FinalCommands:      phpServiceVerificationCommands,
		CleanupCommands:    phpServiceCleanupCommands(),
		Repair:             directCodingPHPRepairConfig(),
	})
}

func phpServiceCleanupCommands() []testCommand {
	return []testCommand{{
		Family: "docker", Name: "docker", Purpose: verificationSetup,
		Timeout: directCodingPHPStageTimeout,
		Args: []string{
			"compose", "down", "--rmi", "local", "--volumes", "--remove-orphans",
		},
	}}
}

func phpServiceTaskVerificationCommands(
	context assemblyline.ApplicationTaskContext,
	program directCodingProgram,
) ([]testCommand, error) {
	if context.WorkloadSHA256 == "" || context.WorkloadSHA256 != program.Workload.SHA256 {
		return nil, fmt.Errorf("PHP HTTP task verification context differs from program workload authority")
	}
	if err := validateFocusedPHPServiceStateLifetime(program, context); err != nil {
		return nil, fmt.Errorf("validate focused PHP HTTP state authority: %w", err)
	}
	hasState, err := phpServiceProgramRequiresPostgreSQL(program)
	if err != nil {
		return nil, err
	}
	if err := validatePHPServiceTaskEndpoint(program, context); err != nil {
		return nil, fmt.Errorf("validate focused PHP HTTP endpoint authority: %w", err)
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
		paths, "src/Runtime.php", pair.ImplementationPath, pair.VerificationPath,
	); err != nil {
		return nil, err
	}
	commands := phpServiceContainerPreparationCommands(hasState)
	commands = append(commands, phpServiceStateSetupCommands(hasState)...)
	commands = append(commands, phpServiceLintCommands(paths, hasState)...)
	commands = append(commands, phpServiceStatePersistenceCommands(hasState)...)
	commands = append(commands, phpServiceStateIsolationCommands(hasState)...)
	commands = append(commands, testCommand{
		Family: "docker", Name: "docker", Purpose: verificationTest,
		Args: phpServiceComposeArgs(
			hasState, "run", "--rm", "--no-deps", "app", "php", pair.VerificationPath,
		),
	})
	commands = append(commands, phpServiceStateIsolationCommands(hasState)...)
	return commands, nil
}

func validatePHPServiceTaskEndpoint(
	program directCodingProgram,
	context assemblyline.ApplicationTaskContext,
) error {
	plan := program.ServiceEndpoints
	if plan.WorkloadSHA256 == "" || plan.WorkloadSHA256 != program.Workload.SHA256 ||
		context.WorkloadSHA256 != program.Workload.SHA256 {
		return fmt.Errorf("focused PHP HTTP endpoint differs from workload authority")
	}
	if len(plan.Requirements) != 1 {
		return fmt.Errorf("focused PHP HTTP endpoint plan requires exactly one task decision")
	}
	requirement, exists := plan.Requirements[context.Task.TaskID]
	if !exists {
		return fmt.Errorf("focused PHP HTTP endpoint omits task decision %s", context.Task.TaskID)
	}
	if plan.ProductContext == "" || plan.ProductContext != context.ProductQuote {
		return fmt.Errorf("focused PHP HTTP endpoint contract lost accepted authority")
	}
	switch requirement {
	case assemblyline.ApplicationServiceSupportOnly:
		if len(plan.ByTask) != 0 {
			return fmt.Errorf("focused PHP HTTP support-only task has an endpoint contract")
		}
	case assemblyline.ApplicationServiceEndpointRequired:
		contract, hasContract := plan.ByTask[context.Task.TaskID]
		if len(plan.ByTask) != 1 || !hasContract ||
			contract.Schema != assemblyline.ApplicationServiceEndpointContractSchemaV1 {
			return fmt.Errorf("focused PHP HTTP endpoint-required task lost its contract")
		}
	default:
		return fmt.Errorf("focused PHP HTTP task has unsupported endpoint decision %q", requirement)
	}
	return nil
}

func phpServiceVerificationCommands(program directCodingProgram) ([]testCommand, error) {
	if err := validatePHPServiceStateLifetime(program.Workload, program.ServiceState); err != nil {
		return nil, fmt.Errorf("validate PHP HTTP state authority: %w", err)
	}
	workloadInput, err := applicationWorkloadInputFromFrozen(program.Workload)
	if err != nil {
		return nil, err
	}
	if err := program.ServiceEndpoints.ValidateFor(workloadInput, program.Workload); err != nil {
		return nil, fmt.Errorf("validate PHP HTTP endpoint authority: %w", err)
	}
	hasState, err := phpServiceProgramRequiresPostgreSQL(program)
	if err != nil {
		return nil, err
	}
	paths, err := phpServiceSourcePaths(program)
	if err != nil {
		return nil, err
	}
	if err := requirePHPServiceSourcePaths(
		paths, "src/Runtime.php", "public/index.php", "tests/TestRunner.php", "tests/HttpVerifier.php",
	); err != nil {
		return nil, err
	}
	commands := phpServiceContainerPreparationCommands(hasState)
	commands = append(commands, phpServiceStateSetupCommands(hasState)...)
	commands = append(commands, phpServiceLintCommands(paths, hasState)...)
	commands = append(commands,
		testCommand{
			Family: "docker", Name: "docker", Purpose: verificationSetup,
			Timeout: directCodingPHPStageTimeout,
			Args:    phpServiceComposeArgs(hasState, "up", "--detach", "--wait", "app"),
		},
		testCommand{
			Family: "docker", Name: "docker", Purpose: verificationConfig,
			Args: phpServiceComposeArgs(
				hasState, "run", "--rm", "--no-deps", "nginx", "nginx", "-t",
			),
		},
	)
	commands = append(commands, phpServiceStatePersistenceCommands(hasState)...)
	commands = append(commands, phpServiceStateIsolationCommands(hasState)...)
	commands = append(commands,
		testCommand{
			Family: "docker", Name: "docker", Purpose: verificationTest,
			Args: phpServiceComposeArgs(
				hasState, "run", "--rm", "--no-deps", "app", "php", "tests/TestRunner.php",
			),
		},
	)
	commands = append(commands, phpServiceStateIsolationCommands(hasState)...)
	commands = append(commands,
		testCommand{
			Family: "docker", Name: "docker", Purpose: verificationSetup,
			Timeout: directCodingPHPStageTimeout,
			Args:    phpServiceComposeArgs(hasState, "up", "--detach", "--wait", "app", "nginx"),
		},
		testCommand{
			Family: "docker", Name: "docker", Purpose: verificationTest,
			Args: phpServiceComposeArgs(
				hasState, "run", "--rm", "--no-deps", "app", "php", "tests/HttpVerifier.php",
			),
		},
	)
	commands = append(commands, phpServiceStateIsolationCommands(hasState)...)
	return commands, nil
}

func phpServiceContainerPreparationCommands(hasState bool) []testCommand {
	return []testCommand{
		{
			Family: "docker", Name: "docker", Purpose: verificationConfig,
			Args: phpServiceComposeArgs(hasState, "config", "--quiet"),
		},
		{
			Family: "docker", Name: "docker", Purpose: verificationBuild,
			Timeout: directCodingPHPStageTimeout,
			Args:    phpServiceComposeArgs(hasState, "build", "app"),
		},
	}
}

func phpServiceLintCommands(paths []string, hasState bool) []testCommand {
	commands := make([]testCommand, 0, len(paths))
	for _, sourcePath := range paths {
		commands = append(commands, testCommand{
			Family: "docker", Name: "docker", Purpose: verificationSyntax,
			Args: phpServiceComposeArgs(
				hasState, "run", "--rm", "--no-deps", "app", "php", "-l", sourcePath,
			),
		})
	}
	return commands
}

func phpServiceStateSetupCommands(hasState bool) []testCommand {
	if !hasState {
		return nil
	}
	return []testCommand{
		{
			Family: "docker", Name: "docker", Purpose: verificationSetup,
			Timeout: directCodingPHPStageTimeout,
			Args:    phpServiceComposeArgs(true, "up", "--detach", "--wait", "postgres"),
		},
		{
			Family: "docker", Name: "docker", Purpose: verificationSetup,
			Args: phpServiceComposeArgs(
				true, "run", "--rm", "--no-deps", "app", "php", phpServiceStateMigrationRunner,
			),
		},
	}
}

func phpServiceStatePersistenceCommands(hasState bool) []testCommand {
	if !hasState {
		return nil
	}
	return []testCommand{
		{
			Family: "docker", Name: "docker", Purpose: verificationTest,
			Args: phpServiceComposeArgs(
				true, "run", "--rm", "--no-deps", "app", "php",
				phpServiceStateVerificationPath, "write",
			),
		},
		{
			Family: "docker", Name: "docker", Purpose: verificationTest,
			Args: phpServiceComposeArgs(
				true, "run", "--rm", "--no-deps", "app", "php",
				phpServiceStateVerificationPath, "read",
			),
		},
	}
}

func phpServiceStateIsolationCommands(hasState bool) []testCommand {
	if !hasState {
		return nil
	}
	return []testCommand{{
		Family: "docker", Name: "docker", Purpose: verificationSetup,
		Args: phpServiceComposeArgs(
			true, "run", "--rm", "--no-deps", "app", "php",
			phpServiceStateVerificationPath, "reset",
		),
	}}
}

func phpServiceComposeArgs(hasState bool, args ...string) []string {
	prefix := []string{"compose"}
	if hasState {
		prefix = append(prefix, "--env-file", phpServiceStateVerificationEnv)
	}
	return append(prefix, args...)
}
