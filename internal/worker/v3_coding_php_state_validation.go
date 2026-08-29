package worker

import (
	"fmt"
	"strings"
)

func validatePHPServiceStateAssembly(files map[string]string) error {
	statePaths := []string{
		phpServiceStateMigrationPath,
		phpServiceStateMigrationRunner,
		phpServiceStateVerificationPath,
		phpServiceStateVerificationEnv,
		phpServiceStateDeploymentEnv,
	}
	present := 0
	for _, artifactPath := range statePaths {
		if strings.TrimSpace(files[artifactPath]) != "" {
			present++
		}
	}
	if present == 0 {
		for _, check := range []struct{ path, marker string }{
			{path: "Dockerfile", marker: "pdo_pgsql"},
			{path: "docker-compose.yml", marker: "service_state_data"},
			{path: "src/Runtime.php", marker: "final class RuntimeState"},
		} {
			if strings.Contains(files[check.path], check.marker) {
				return fmt.Errorf(
					"PHP HTTP request-local assembly contains unused durable state in %s",
					check.path,
				)
			}
		}
		return nil
	}
	if present != len(statePaths) {
		return fmt.Errorf(
			"PHP HTTP durable state requires one complete artifact set; found %d of %d",
			present, len(statePaths),
		)
	}
	if files[phpServiceStateMigrationPath] != phpServiceStateMigrationSQL() {
		return fmt.Errorf("PHP HTTP durable state migration differs from code-owned schema")
	}
	if files[phpServiceStateMigrationRunner] != phpServiceStateMigrationRunnerSource() {
		return fmt.Errorf("PHP HTTP durable state migration runner differs from code-owned source")
	}
	if files[phpServiceStateVerificationEnv] != phpServiceStateVerificationEnvironment() {
		return fmt.Errorf("PHP HTTP durable state verification environment differs from code-owned source")
	}
	if files[phpServiceStateDeploymentEnv] != phpServiceStateDeploymentEnvironment() {
		return fmt.Errorf("PHP HTTP durable state deployment environment differs from code-owned source")
	}
	for _, check := range []struct {
		path     string
		required []string
	}{
		{path: "Dockerfile", required: []string{
			"docker-php-ext-install -j1 pdo_pgsql",
			"COPY " + phpServiceStateMigrationPath,
			"COPY " + phpServiceStateMigrationRunner,
			"COPY " + phpServiceStateVerificationPath,
		}},
		{path: "docker-compose.yml", required: []string{
			"postgres:", "condition: service_healthy", "${SERVICE_STATE_DB_PASSWORD}",
			"DATABASE_URL:", "service_state_data:/var/lib/postgresql/data",
		}},
		{path: "src/Runtime.php", required: []string{
			"final class RuntimeDatabase", "final class RuntimeState",
			"DATABASE_URL is required for durable service state",
			"SELECT version FROM " + directCodingServiceStateSchemaTable,
		}},
		{path: phpServiceStateVerificationPath, required: []string{
			"Durable service state did not survive a separate process",
			"Durable application state reset was not authoritative",
			"$applicationScope", "$verificationScope", "RuntimeState::save", "RuntimeState::delete",
		}},
	} {
		for _, marker := range check.required {
			if !strings.Contains(files[check.path], marker) {
				return fmt.Errorf("PHP HTTP durable state artifact %s omits %s", check.path, marker)
			}
		}
	}
	for _, artifactPath := range []string{"Dockerfile", "docker-compose.yml", "src/Runtime.php"} {
		if strings.Contains(files[artifactPath], phpServiceStateVerificationSecret) {
			return fmt.Errorf("PHP HTTP durable state embeds its verification credential in %s", artifactPath)
		}
	}
	return nil
}
