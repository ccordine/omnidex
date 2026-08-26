package worker

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

var v3DeploymentComposeProjectPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,62}$`)

func validateV3DockerCompose(args []string) error {
	if matched, err := validateLaravelDockerCompose(args); matched {
		return err
	}
	if len(args) >= 3 && slicesEqualStrings(args[:3], []string{
		"compose", "--env-file", phpServiceStateVerificationEnv,
	}) {
		args = append([]string{"compose"}, args[3:]...)
	}
	if slicesEqualStrings(args, []string{"compose", "config", "--quiet"}) ||
		slicesEqualStrings(args, []string{"compose", "config", "--format", "json"}) ||
		slicesEqualStrings(args, []string{"compose", "build", "app"}) ||
		slicesEqualStrings(args, []string{"compose", "up", "--detach", "--wait", "postgres"}) ||
		slicesEqualStrings(args, []string{"compose", "up", "--detach", "--wait", "app"}) ||
		slicesEqualStrings(args, []string{"compose", "up", "--detach", "--wait", "app", "nginx"}) ||
		slicesEqualStrings(args, []string{
			"compose", "down", "--rmi", "local", "--volumes", "--remove-orphans",
		}) ||
		slicesEqualStrings(args, []string{
			"compose", "run", "--rm", "--no-deps", "nginx", "nginx", "-t",
		}) {
		return nil
	}
	phpPrefix := []string{"compose", "run", "--rm", "--no-deps", "app", "php"}
	if len(args) == len(phpPrefix)+1 && slicesEqualStrings(args[:len(phpPrefix)], phpPrefix) &&
		validV3PHPSourcePath(args[len(phpPrefix)]) {
		return nil
	}
	if len(args) == len(phpPrefix)+2 && slicesEqualStrings(args[:len(phpPrefix)], phpPrefix) &&
		args[len(phpPrefix)] == "-l" && validV3PHPSourcePath(args[len(phpPrefix)+1]) {
		return nil
	}
	if len(args) == len(phpPrefix)+2 && slicesEqualStrings(args[:len(phpPrefix)], phpPrefix) &&
		args[len(phpPrefix)] == phpServiceStateVerificationPath &&
		(args[len(phpPrefix)+1] == "write" || args[len(phpPrefix)+1] == "read" ||
			args[len(phpPrefix)+1] == "reset") {
		return nil
	}
	return fmt.Errorf("command.run permits only code-owned Docker Compose verification commands")
}

func validateV3DeploymentDockerCompose(args []string) error {
	if len(args) < 6 || args[0] != "compose" || args[1] != "--project-name" ||
		!v3DeploymentComposeProjectPattern.MatchString(args[2]) ||
		args[3] != "--file" || args[4] != directCodingDeploymentComposePath {
		return fmt.Errorf("persistent deployment requires exact project and Compose-file authority")
	}
	command := args[5:]
	for _, exact := range [][]string{
		{"config", "--hash=*"},
		{"build", "app", "nginx"},
		{"up", "--detach", "--wait", "--remove-orphans"},
		{"ps", "--all", "--orphans", "--format", "json"},
		{"restart", "app"},
		{"down", "--volumes", "--remove-orphans"},
	} {
		if slicesEqualStrings(command, exact) {
			return nil
		}
	}
	phpPrefix := []string{"run", "--rm", "--no-deps", "app", "php"}
	if len(command) == len(phpPrefix)+2 && slicesEqualStrings(command[:len(phpPrefix)], phpPrefix) &&
		command[len(phpPrefix)] == phpServiceStateVerificationPath {
		switch command[len(phpPrefix)+1] {
		case "write", "read":
			return nil
		}
	}
	if len(command) == len(phpPrefix)+1 && slicesEqualStrings(command[:len(phpPrefix)], phpPrefix) &&
		command[len(phpPrefix)] == phpServiceStateMigrationRunner {
		return nil
	}
	return fmt.Errorf("persistent deployment permits only registered Compose lifecycle commands")
}

func validV3PHPSourcePath(value string) bool {
	clean := filepath.ToSlash(filepath.Clean(value))
	return clean == value && clean != "." && !strings.HasPrefix(clean, "../") &&
		strings.HasSuffix(strings.ToLower(clean), ".php")
}
