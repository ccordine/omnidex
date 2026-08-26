package worker

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestLaravelDockerRuntimeQualification(t *testing.T) {
	if os.Getenv("OMNIDEX_LARAVEL_DOCKER_TEST") != "1" {
		t.Skip("set OMNIDEX_LARAVEL_DOCKER_TEST=1 for the real Docker qualification")
	}
	t.Run("request-local-server-rendered", func(t *testing.T) {
		qualifyLaravelDockerProgram(t, laravelFixtureProgram(t, laravelWeatherFixtureInput()))
	})
	t.Run("durable-cross-endpoint-json", func(t *testing.T) {
		qualifyLaravelDockerProgram(t, laravelLifecycleFixtureProgram(t, laravelLifecycleFixture{
			Package: "equipment-history", Product: "Equipment inspection history",
			WriteRequirement: "Retain an accepted equipment inspection between requests.",
			ReadRequirement:  "Present the current equipment inspection history.",
			RootField:        "inspections", ValueField: "reference", DetailField: "status",
			WriteRoute: "/equipment/inspections", ReadRoute: "/equipment/current",
			WriteMethod: assemblyline.ApplicationServiceEndpointPOST,
		}))
	})
}

func qualifyLaravelDockerProgram(t *testing.T, program directCodingProgram) {
	t.Helper()
	storage, err := deriveDirectCodingServiceStoragePlan(program.Workload, program.ServiceState)
	if err != nil {
		t.Fatal(err)
	}
	durable := storage.RequiresPostgreSQL()
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingAssemblySources(program, assembly); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	for _, file := range assembly.Files {
		target := filepath.Join(root, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, []byte(file.Content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	project := "omnidex-laravel-qualification-" + program.Workload.SHA256[:12]
	run := func(expectFailure bool, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		command := exec.CommandContext(ctx, "docker", args...)
		command.Dir = root
		command.Env = make([]string, 0, len(os.Environ())+1)
		for _, value := range os.Environ() {
			if strings.HasPrefix(value, "APP_KEY=") || strings.HasPrefix(value, "DATABASE_PASSWORD=") ||
				strings.HasPrefix(value, "COMPOSE_PROJECT_NAME=") {
				continue
			}
			command.Env = append(command.Env, value)
		}
		command.Env = append(command.Env, "COMPOSE_PROJECT_NAME="+project)
		output, runErr := command.CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf("Docker command timed out: %v", args)
		}
		if expectFailure == (runErr == nil) {
			if !expectFailure {
				logs := exec.CommandContext(ctx, "docker", "compose", "--env-file", laravelVerificationEnvPath,
					"logs", "--no-color", "app", "db", "nginx")
				logs.Dir = root
				logs.Env = command.Env
				if logOutput, logErr := logs.CombinedOutput(); logErr == nil || len(logOutput) != 0 {
					output = append(output, []byte("\nCompose logs:\n")...)
					output = append(output, logOutput...)
				}
			}
			t.Fatalf("Docker command %v error=%v output:\n%s", args, runErr, output)
		}
		return string(output)
	}
	compose := func(arguments ...string) []string {
		return append([]string{"compose", "--env-file", laravelVerificationEnvPath}, arguments...)
	}
	defer run(false, compose("down", "--rmi", "local", "--volumes", "--remove-orphans")...)

	missingKeyPath := filepath.Join(root, "tests", "laravel-missing-key.env.example")
	missingKeyEnvironment := "HOST_BIND_ADDRESS=127.0.0.1\nHOST_HTTP_PORT=0\n"
	if durable {
		missingKeyEnvironment += "DATABASE_PASSWORD=omnidex-laravel-verification-only-password\n"
	}
	if err := os.WriteFile(missingKeyPath, []byte(missingKeyEnvironment), 0o600); err != nil {
		t.Fatal(err)
	}
	missingOutput := run(true, "compose", "--env-file", "tests/laravel-missing-key.env.example", "config", "--quiet")
	if !strings.Contains(missingOutput, "APP_KEY") {
		t.Fatalf("missing APP_KEY failure was not explicit:\n%s", missingOutput)
	}
	if durable {
		missingDatabasePath := filepath.Join(root, "tests", "laravel-missing-database.env.example")
		if err := os.WriteFile(missingDatabasePath, []byte(
			"APP_KEY=base64:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=\n"+
				"HOST_BIND_ADDRESS=127.0.0.1\nHOST_HTTP_PORT=0\n",
		), 0o600); err != nil {
			t.Fatal(err)
		}
		missingDatabaseOutput := run(true, "compose", "--env-file",
			"tests/laravel-missing-database.env.example", "config", "--quiet")
		if !strings.Contains(missingDatabaseOutput, "DATABASE_PASSWORD") {
			t.Fatalf("missing DATABASE_PASSWORD failure was not explicit:\n%s", missingDatabaseOutput)
		}
	}

	run(false, compose("config", "--quiet")...)
	run(false, compose("build", "app", "nginx")...)
	if durable {
		run(false, compose("up", "--detach", "--wait", "db")...)
		run(false, compose("run", "--rm", "--no-deps", "app", "php", "artisan", "migrate", "--force")...)
	}
	run(false, compose("run", "--rm", "--no-deps", "app", "php", laravelPlatformVerificationPath)...)
	run(false, compose("up", "--detach", "--wait", "app")...)
	run(false, compose("run", "--rm", "--no-deps", "nginx", "nginx", "-t")...)
	if durable {
		for _, mode := range []string{"write", "read", "reset"} {
			run(false, compose("run", "--rm", "--no-deps", "app", "php",
				phpServiceStateVerificationPath, mode)...)
		}
	}
	resetState := func() {
		if durable {
			run(false, compose("run", "--rm", "--no-deps", "app", "php",
				phpServiceStateVerificationPath, "reset")...)
		}
	}
	resetState()
	run(false, compose("run", "--rm", "--no-deps", "app", "php", "tests/TestRunner.php")...)
	resetState()
	run(false, compose("up", "--detach", "--wait", "app", "nginx")...)
	resetState()
	run(false, compose("run", "--rm", "--no-deps", "app", "php", "tests/HttpVerifier.php")...)
	resetState()
	published := strings.TrimSpace(run(false, "compose", "--env-file", laravelVerificationEnvPath, "port", "nginx", "80"))
	match := regexp.MustCompile(`^127\.0\.0\.1:([1-9][0-9]*)$`).FindStringSubmatch(published)
	if len(match) != 2 {
		t.Fatalf("dynamic loopback host port=%q", published)
	}
	t.Logf("qualified Laravel fixture on %s with project %s", fmt.Sprintf("http://%s", published), project)
}
