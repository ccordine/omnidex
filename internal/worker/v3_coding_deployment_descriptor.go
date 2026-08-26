package worker

import (
	"fmt"
	"sort"
	"strings"
)

const directCodingDeploymentReadinessPath = "/__omnidex/health"

// directCodingDeploymentDescriptor is technical stack authority. It contains
// no product behavior and cannot supply arbitrary commands.
type directCodingDeploymentDescriptor struct {
	GatewayService              string
	GatewayContainerPort        int
	ReadinessPath               string
	BaseServices                []string
	StateService                string
	ApplicationKeyEnvironment   string
	DatabasePasswordEnvironment string
	MigrationScript             string
}

func genericPHPDeploymentDescriptor() *directCodingDeploymentDescriptor {
	return &directCodingDeploymentDescriptor{
		GatewayService: "nginx", GatewayContainerPort: 80,
		ReadinessPath: directCodingDeploymentReadinessPath,
		BaseServices:  []string{"app", "nginx"}, StateService: "postgres",
		DatabasePasswordEnvironment: "SERVICE_STATE_DB_PASSWORD",
		MigrationScript:             phpServiceStateMigrationRunner,
	}
}

func laravelDeploymentDescriptor() *directCodingDeploymentDescriptor {
	return &directCodingDeploymentDescriptor{
		GatewayService: "nginx", GatewayContainerPort: 80,
		ReadinessPath: directCodingDeploymentReadinessPath,
		BaseServices:  []string{"app", "nginx"}, StateService: "db",
		ApplicationKeyEnvironment:   "APP_KEY",
		DatabasePasswordEnvironment: "DATABASE_PASSWORD",
	}
}

func (descriptor directCodingDeploymentDescriptor) validate() error {
	if descriptor.GatewayService != "nginx" || descriptor.GatewayContainerPort != 80 ||
		descriptor.ReadinessPath != directCodingDeploymentReadinessPath {
		return fmt.Errorf("deployment descriptor requires the registered HTTP gateway authority")
	}
	if len(descriptor.BaseServices) != 2 || descriptor.BaseServices[0] != "app" ||
		descriptor.BaseServices[1] != "nginx" {
		return fmt.Errorf("deployment descriptor requires exact ordered app and nginx services")
	}
	if descriptor.StateService != "db" && descriptor.StateService != "postgres" {
		return fmt.Errorf("deployment descriptor state service %q is unsupported", descriptor.StateService)
	}
	if descriptor.ApplicationKeyEnvironment != "" && descriptor.ApplicationKeyEnvironment != "APP_KEY" {
		return fmt.Errorf("deployment descriptor application-key environment is unsupported")
	}
	switch descriptor.DatabasePasswordEnvironment {
	case "DATABASE_PASSWORD", "SERVICE_STATE_DB_PASSWORD":
	default:
		return fmt.Errorf("deployment descriptor database environment is unsupported")
	}
	if descriptor.MigrationScript != "" && descriptor.MigrationScript != phpServiceStateMigrationRunner {
		return fmt.Errorf("deployment descriptor migration script is unsupported")
	}
	return nil
}

func (descriptor directCodingDeploymentDescriptor) environment(
	program directCodingProgram,
	settings DeploymentSettings,
	endpointPort uint16,
	secrets map[string]string,
) (map[string]string, error) {
	if err := descriptor.validate(); err != nil {
		return nil, err
	}
	hasState, err := directCodingProgramRequiresDurableState(program)
	if err != nil {
		return nil, err
	}
	environment := map[string]string{
		"HOST_BIND_ADDRESS": settings.BindAddress,
		"HOST_HTTP_PORT":    fmt.Sprintf("%d", endpointPort),
	}
	if name := descriptor.ApplicationKeyEnvironment; name != "" {
		if secrets[name] == "" {
			return nil, fmt.Errorf("deployment secrets omit %s", name)
		}
		environment[name] = secrets[name]
	}
	if hasState {
		name := descriptor.DatabasePasswordEnvironment
		if secrets[name] == "" {
			return nil, fmt.Errorf("deployment secrets omit %s", name)
		}
		environment[name] = secrets[name]
	}
	return environment, nil
}

func (descriptor directCodingDeploymentDescriptor) expectedServices(
	program directCodingProgram,
) ([]string, error) {
	if err := descriptor.validate(); err != nil {
		return nil, err
	}
	hasState, err := directCodingProgramRequiresDurableState(program)
	if err != nil {
		return nil, err
	}
	services := append([]string(nil), descriptor.BaseServices...)
	if hasState {
		services = append(services, descriptor.StateService)
	}
	sort.Strings(services)
	return services, nil
}

func directCodingProgramRequiresDurableState(program directCodingProgram) (bool, error) {
	switch program.StackID {
	case genericPHPServiceAdapter:
		return phpServiceProgramRequiresPostgreSQL(program)
	case laravelHTTPServiceAdapter:
		return laravelProgramRequiresPostgreSQL(program)
	default:
		if len(program.ServiceState.ByTask) != 0 {
			return false, fmt.Errorf("non-service stack %s contains service state authority", program.StackID)
		}
		return false, nil
	}
}

func directCodingDeploymentComposeArgs(project string, command ...string) ([]string, error) {
	if !v3DeploymentComposeProjectPattern.MatchString(project) || len(command) == 0 {
		return nil, fmt.Errorf("deployment Compose arguments require exact project and command authority")
	}
	args := append([]string{
		"compose", "--project-name", project,
		"--file", directCodingDeploymentComposePath,
	}, command...)
	if err := validateV3DeploymentDockerCompose(args); err != nil {
		return nil, err
	}
	return args, nil
}

func validateDirectCodingDeploymentEnvironmentAbsentFromText(
	text string,
	environment map[string]string,
) error {
	for name, value := range environment {
		switch name {
		case "APP_KEY", "DATABASE_PASSWORD", "SERVICE_STATE_DB_PASSWORD":
		default:
			continue
		}
		if strings.TrimSpace(value) != "" && strings.Contains(text, value) {
			return fmt.Errorf("deployment output exposed environment value for %s", name)
		}
	}
	return nil
}
