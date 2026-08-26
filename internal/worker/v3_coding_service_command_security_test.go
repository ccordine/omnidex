package worker

import "testing"

func TestDockerComposeVerificationBoundaryAcceptsOnlyCodeOwnedServiceCommands(t *testing.T) {
	t.Parallel()
	accepted := [][]string{
		{"compose", "config", "--quiet"},
		{"compose", "build", "app"},
		{"compose", "up", "--detach", "--wait", "app"},
		{"compose", "up", "--detach", "--wait", "app", "nginx"},
		{"compose", "run", "--rm", "--no-deps", "app", "php", "tests/Feature001Test.php"},
		{"compose", "run", "--rm", "--no-deps", "app", "php", "-l", "public/index.php"},
		{"compose", "run", "--rm", "--no-deps", "nginx", "nginx", "-t"},
		{"compose", "down", "--rmi", "local", "--volumes", "--remove-orphans"},
	}
	for _, args := range accepted {
		if err := validateV3Command("docker", args); err != nil {
			t.Fatalf("docker %v rejected: %v", args, err)
		}
	}
	rejected := [][]string{
		{"compose", "up", "-d"},
		{"compose", "up", "--detach", "app", "nginx"},
		{"compose", "down", "--volumes"},
		{"compose", "down", "--rmi", "all", "--volumes", "--remove-orphans"},
		{"compose", "run", "--rm", "app", "sh"},
		{"compose", "run", "--rm", "--no-deps", "app", "php", "../outside.php"},
		{"compose", "run", "--rm", "--no-deps", "app", "php", "-r", "system('id');"},
		{"run", "--privileged", "composer:2", "php", "tests/TestRunner.php"},
	}
	for _, args := range rejected {
		if err := validateV3Command("docker", args); err == nil {
			t.Fatalf("docker %v unexpectedly accepted", args)
		}
	}
}
