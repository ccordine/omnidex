package worker

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/operation"
)

func TestDeploymentCommandsAreDerivedFromTypedKinds(t *testing.T) {
	t.Parallel()
	project := "omnidex-job-7-g1"
	descriptor := *genericPHPDeploymentDescriptor()
	environment := map[string]string{
		"HOST_BIND_ADDRESS": "127.0.0.1", "HOST_HTTP_PORT": "0",
		"SERVICE_STATE_DB_PASSWORD": "secret",
	}
	expected := map[directCodingDeploymentCommandKind][]string{
		directCodingDeploymentConfig:  {"config", "--hash=*"},
		directCodingDeploymentBuild:   {"build", "app", "nginx"},
		directCodingDeploymentStart:   {"up", "--detach", "--wait", "--remove-orphans"},
		directCodingDeploymentObserve: {"ps", "--all", "--orphans", "--format", "json"},
		directCodingDeploymentMigrate: {"run", "--rm", "--no-deps", "app", "php", phpServiceStateMigrationRunner},
		directCodingDeploymentWrite:   {"run", "--rm", "--no-deps", "app", "php", phpServiceStateVerificationPath, "write"},
		directCodingDeploymentRead:    {"run", "--rm", "--no-deps", "app", "php", phpServiceStateVerificationPath, "read"},
		directCodingDeploymentRestart: {"restart", "app"},
		directCodingDeploymentRollback: {
			"down", "--volumes", "--remove-orphans",
		},
	}
	for kind, suffix := range expected {
		command, err := directCodingDeploymentCommand(kind, project, descriptor, environment)
		if err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
		want := append([]string{
			"compose", "--project-name", project,
			"--file", directCodingDeploymentComposePath,
		}, suffix...)
		if command.Profile != codeCommandProfileDeployment || !reflect.DeepEqual(command.Args, want) ||
			command.Environment["SERVICE_STATE_DB_PASSWORD"] != "secret" {
			t.Fatalf("%s command=%+v", kind, command)
		}
		command.Environment["SERVICE_STATE_DB_PASSWORD"] = "changed"
		if environment["SERVICE_STATE_DB_PASSWORD"] != "secret" {
			t.Fatal("deployment command retained mutable environment authority")
		}
	}
}

func TestDeploymentCommandsRejectApplicationStateReset(t *testing.T) {
	args := []string{
		"compose", "--project-name", "omnidex-job-7-g1",
		"--file", directCodingDeploymentComposePath, "run", "--rm", "--no-deps",
		"app", "php", phpServiceStateVerificationPath, "reset",
	}
	if err := validateV3CommandForProfile("docker", args, codeCommandProfileDeployment); err == nil {
		t.Fatal("persistent deployment accepted destructive application-state reset")
	}
}

func TestDeploymentSecretLeakDetectorIgnoresPublicValuesAndRejectsSecrets(t *testing.T) {
	t.Parallel()
	environment := map[string]string{
		"HOST_BIND_ADDRESS": "127.0.0.1", "HOST_HTTP_PORT": "0",
		"APP_KEY": "base64:private-value", "DATABASE_PASSWORD": "database-private-value",
	}
	if err := validateDirectCodingDeploymentEnvironmentAbsentFromText(
		"published on 127.0.0.1:18080", environment,
	); err != nil {
		t.Fatal(err)
	}
	if err := validateDirectCodingDeploymentEnvironmentAbsentFromText(
		"failure base64:private-value", environment,
	); err == nil {
		t.Fatal("accepted deployment output containing a secret")
	}
}

func TestDeploymentRuntimeDriftCannotReachCommandExecutor(t *testing.T) {
	t.Parallel()
	for _, testCase := range []struct {
		name    string
		engine  string
		compose string
		want    string
	}{
		{name: "engine", engine: "29.5.0", compose: directCodingDockerComposeVersion, want: "Engine"},
		{name: "compose", engine: directCodingDockerEngineVersion, compose: "5.1.3", want: "Compose"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			executions := 0
			probe := func(program string, args ...string) (string, error) {
				switch versionProbeKey(program, args...) {
				case versionProbeKey("docker", "version", "--format", "{{.Server.Version}}"):
					return testCase.engine, nil
				case versionProbeKey("docker", "compose", "version", "--short"):
					return testCase.compose, nil
				default:
					return "", errors.New("unexpected runtime probe")
				}
			}
			_, err := executeDirectCodingDeploymentAfterRuntimeQualification(
				phpServiceVersionProfileV1, probe,
				func() (operation.Result, error) {
					executions++
					return operation.Result{}, nil
				},
			)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("runtime drift error=%v", err)
			}
			if executions != 0 {
				t.Fatalf("runtime drift spawned %d deployment commands", executions)
			}
		})
	}
}

func TestDeploymentRuntimeQualificationExecutesOnceAfterExactProfileMatch(t *testing.T) {
	t.Parallel()
	executions := 0
	probe := func(program string, args ...string) (string, error) {
		switch versionProbeKey(program, args...) {
		case versionProbeKey("docker", "version", "--format", "{{.Server.Version}}"):
			return directCodingDockerEngineVersion, nil
		case versionProbeKey("docker", "compose", "version", "--short"):
			return directCodingDockerComposeVersion, nil
		default:
			return "", errors.New("unexpected runtime probe")
		}
	}
	_, err := executeDirectCodingDeploymentAfterRuntimeQualification(
		laravelVersionProfileV1, probe,
		func() (operation.Result, error) {
			executions++
			return operation.Result{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if executions != 1 {
		t.Fatalf("qualified deployment executions=%d want 1", executions)
	}
}

func TestProtectedDeploymentNamespaceGateIsLastOperationBeforeCommandExecutor(t *testing.T) {
	t.Parallel()
	events := make([]string, 0, 4)
	probe := func(program string, args ...string) (string, error) {
		switch versionProbeKey(program, args...) {
		case versionProbeKey("docker", "version", "--format", "{{.Server.Version}}"):
			events = append(events, "engine")
			return directCodingDockerEngineVersion, nil
		case versionProbeKey("docker", "compose", "version", "--short"):
			events = append(events, "compose")
			return directCodingDockerComposeVersion, nil
		default:
			return "", errors.New("unexpected runtime probe")
		}
	}
	_, err := executeDirectCodingDeploymentAfterRuntimeQualificationAndGate(
		phpServiceVersionProfileV1, probe,
		func() error {
			events = append(events, "namespace")
			return nil
		},
		func() (operation.Result, error) {
			events = append(events, "executor")
			return operation.Result{}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(events, []string{"engine", "compose", "namespace", "executor"}) {
		t.Fatalf("protected command order=%v", events)
	}
}

func TestDeploymentRuntimeQualificationPrecedesDurableSideEffectJournal(t *testing.T) {
	t.Parallel()
	for _, file := range []string{
		"v3_coding_deployment_runtime.go",
		"v3_coding_deployment_rollback.go",
	} {
		source, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		text := string(source)
		qualification := strings.Index(text, "qualifyDirectCodingDeploymentRuntime(")
		journal := strings.Index(text, "BeginGeneratedWorkloadDeployment")
		if qualification < 0 || journal <= qualification {
			t.Fatalf("%s can journal a side-effect command before exact runtime qualification", file)
		}
	}
}
