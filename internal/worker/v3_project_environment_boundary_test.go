package worker

import (
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestProjectDevelopmentEnvironmentHasNoSemanticOrHostToolFallback(t *testing.T) {
	files := []string{
		"v3_project_environment.go",
		"v3_project_environment_docker.go",
		"v3_project_environment_specs.go",
		"v3_single_artifact_environment.go",
	}
	for _, name := range files {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, forbidden := range []string{
			"PortableJob", "exactStation", "inference.", "modelclient", "webSearch",
			"validateV3RootlessDockerDaemon", "name=rootless", "image\", \"rm", "prune",
			"--tag", "omnidex-project-environment:",
			"exec.CommandContext(ctx, command.Program", "exec.CommandContext(ctx, program",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s contains forbidden environment authority %q", name, forbidden)
			}
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), name, raw, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imported := range parsed.Imports {
			path := strings.Trim(imported.Path.Value, `"`)
			if strings.Contains(path, "github.com/gryph/omnidex/internal/") {
				t.Errorf("%s imports non-mechanical Omnidex authority %q", name, path)
			}
		}
	}
}

func TestProjectStacksDoNotDeclareUnusedDevelopmentEnvironmentMetadata(t *testing.T) {
	for _, name := range []string{
		"v3_project_stack.go", "v3_coding_laravel_adapter.go",
		"v3_artifact_registry_validation.go",
	} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if strings.Contains(string(raw), "DevelopmentEnvironment") ||
			strings.Contains(string(raw), "directCodingDockerEnvironmentSpecForStack") {
			t.Errorf("%s retains unused project-stack environment metadata", name)
		}
	}
}

func TestPlainTextRecoveryDoesNotReconstructCurrentBinaryEnvironmentDefaults(t *testing.T) {
	raw, err := os.ReadFile("v3_workspace_mutation_recovery.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "directCodingPlainTextEnvironmentSpec") {
		t.Fatal("plain-text recovery consults current binary environment defaults")
	}
	for _, required := range []string{"commands[0].Environment.spec()", "RequireAuthority(commands[0].Environment)"} {
		if !strings.Contains(string(raw), required) {
			t.Errorf("plain-text recovery lacks persisted environment authority %q", required)
		}
	}
}

func TestSingleArtifactModelsHaveNoEnvironmentOrMutationAuthority(t *testing.T) {
	semantic, err := os.ReadFile("v3_existing_repository_single_artifact.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"ExecuteWorkspaceMutation", "invokeDirectCodingDocker", "docker build",
		"docker run", "testCommand", "WorkspaceMutationCommand",
	} {
		if strings.Contains(string(semantic), forbidden) {
			t.Errorf("single-artifact semantic boundary contains environment authority %q", forbidden)
		}
	}
	for _, name := range []string{
		"v3_single_artifact_environment.go", "v3_single_artifact_mutation.go",
	} {
		raw, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		for _, forbidden := range []string{
			"workerModel(", "PortableJob", "station.Coding", "runtime.Execute(",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Errorf("%s contains model authority %q", name, forbidden)
			}
		}
	}
}

func TestSinglePlainTextCreationCannotDelegateAdapterPathOrRetryAuthority(t *testing.T) {
	semantic, err := os.ReadFile("v3_existing_repository_single_artifact.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(semantic)
	sourceLeaf, err := os.ReadFile("v3_single_artifact_source.go")
	if err != nil {
		t.Fatal(err)
	}
	combinedSource := source + string(sourceLeaf)
	for _, forbidden := range []string{
		"runDirectCodingTargetTreeCall", "TargetTreeCorrection", "PathSelectionOnly",
		"station.CodingTargetTree", "resolveSingleArtifactCreationPath",
	} {
		if strings.Contains(source, forbidden) {
			t.Errorf("single plain-text creation retains delegated structural authority %q", forbidden)
		}
	}
	for _, required := range []string{
		"explicitPlainTextArtifactPaths", "FreezePlainTextArtifactTask",
		"NewPlainTextArtifactCoverage", "CompilePlainTextArtifactBlueprint",
		"DiffTargetTree", "RequireGitPathVisible", "validatePlainTextPathBlindValue",
	} {
		if !strings.Contains(combinedSource, required) {
			t.Errorf("single plain-text creation lacks code-owned boundary %q", required)
		}
	}

	targetSource, err := os.ReadFile("../assemblyline/target_tree.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(targetSource), "PathSelectionOnly") ||
		strings.Contains(string(targetSource), "path_selection_only") {
		t.Fatal("target-tree contract retains a snapshot-omission shortcut")
	}
}

func TestDockerTransportStripsAmbientDockerRoutingEnvironment(t *testing.T) {
	got := directCodingDockerProcessEnvironment([]string{
		"PATH=/usr/bin",
		"DOCKER_CONTEXT=rootless", "DOCKER_HOST=unix:///run/user/1000/docker.sock",
		"DOCKER_CONFIG=/tmp/rootless-docker", "DOCKER_CERT_PATH=/tmp/certs",
		"DOCKER_TLS=1", "DOCKER_TLS_VERIFY=1",
		"BUILDKIT_HOST=tcp://127.0.0.1:1234", "BUILDKIT_TLS_SERVER_NAME=rootless",
		"BUILDKIT_TLS_CACERT=/tmp/ca", "BUILDKIT_TLS_CERT=/tmp/cert",
		"BUILDKIT_TLS_KEY=/tmp/key", "BUILDX_BUILDER=rootless",
		"BUILDX_CONFIG=/tmp/buildx",
	})
	if len(got) != 1 || got[0] != "PATH=/usr/bin" {
		t.Fatalf("sanitized Docker environment=%q", got)
	}
	for _, value := range got {
		name, _, _ := strings.Cut(value, "=")
		if directCodingDockerRoutingEnvironment(name) {
			t.Fatalf("Docker routing authority survived sanitization: %q", value)
		}
	}
}

func TestDockerTransportExecutesOnlyLiteralDocker(t *testing.T) {
	raw, err := os.ReadFile("v3_project_environment_docker.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if !strings.Contains(source, `exec.CommandContext(ctx, "docker"`) {
		t.Fatal("Docker transport does not bind execution to the literal Docker CLI")
	}
	if !strings.Contains(source, `v3DockerCLIArguments(args)`) {
		t.Fatal("Docker transport does not project the fixed rootful --host authority")
	}
	for _, hostTool := range []string{`"go"`, `"node"`, `"npm"`, `"cargo"`, `"rustc"`, `"java"`, `"javac"`, `"php"`, `"composer"`} {
		if strings.Contains(source, "exec.CommandContext(ctx, "+hostTool) {
			t.Errorf("Docker transport executes host language tool %s", hostTool)
		}
	}
}
