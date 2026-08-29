package worker

import (
	"strings"
	"testing"
)

func TestDeploymentDockerCommandsRequireExactProfileAndProject(t *testing.T) {
	t.Parallel()
	project := "omnidex-job-41-g2"
	prefix := []string{
		"compose", "--project-name", project,
		"--file", directCodingDeploymentComposePath,
	}
	valid := [][]string{
		append(append([]string(nil), prefix...), "config", "--hash=*"),
		append(append([]string(nil), prefix...), "build", "app", "nginx"),
		append(append([]string(nil), prefix...), "up", "--detach", "--wait", "--remove-orphans"),
		append(append([]string(nil), prefix...), "ps", "--all", "--orphans", "--format", "json"),
		append(append([]string(nil), prefix...), "restart", "app"),
		append(append([]string(nil), prefix...), "down", "--volumes", "--remove-orphans"),
	}
	for _, args := range valid {
		if err := validateV3CommandForProfile("docker", args, codeCommandProfileDeployment); err != nil {
			t.Fatalf("deployment command %v: %v", args, err)
		}
		if err := validateV3Command("docker", args); err == nil {
			t.Fatalf("verification profile accepted deployment command %v", args)
		}
	}
	for _, args := range [][]string{
		{"compose", "--project-name", "UPPER", "--file", directCodingDeploymentComposePath, "up", "--detach", "--wait"},
		{"compose", "--project-name", project, "up", "--detach", "--wait"},
		{"compose", "--project-name", project, "--file", "compose.yaml", "up", "--detach", "--wait"},
		append(append([]string(nil), prefix...), "exec", "app", "sh"),
		append(append([]string(nil), prefix...), "down"),
		append(append([]string(nil), prefix...), "down", "--remove-orphans"),
		append(append([]string(nil), prefix...), "down", "--remove-orphans", "--volumes"),
		append(append([]string(nil), prefix...), "logs"),
		append(append([]string(nil), prefix...), "port", "nginx", "80"),
	} {
		if err := validateV3CommandForProfile("docker", args, codeCommandProfileDeployment); err == nil {
			t.Fatalf("accepted unregistered deployment command %v", args)
		}
	}
}

func TestDeploymentEnvironmentIsProfileBoundAndNeverRenderedAsCommand(t *testing.T) {
	t.Parallel()
	command := codeCommand{
		Program: "docker", Profile: codeCommandProfileDeployment,
		Args: []string{
			"compose", "--project-name", "omnidex-job-1-g1",
			"--file", directCodingDeploymentComposePath, "config", "--hash=*",
		},
		Environment: map[string]string{
			"APP_KEY": "base64:secret", "HOST_BIND_ADDRESS": "127.0.0.1", "HOST_HTTP_PORT": "0",
		},
	}
	if err := validateV3CommandEnvironment(command); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(append([]string{command.Program}, command.Args...), " "), "secret") {
		t.Fatal("deployment command rendered secret environment")
	}
	command.Profile = ""
	if err := validateV3CommandEnvironment(command); err == nil {
		t.Fatal("verification command accepted deployment environment")
	}
	command.Profile = codeCommandProfileDeployment
	command.Environment["UNREGISTERED_SECRET"] = "secret"
	if err := validateV3CommandEnvironment(command); err == nil {
		t.Fatal("deployment command accepted unregistered environment")
	}
}
