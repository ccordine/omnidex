package worker

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/queue"
)

const phpExpectedCleanupCommand = "compose down --rmi local --volumes --remove-orphans"

func TestDirectSessionSealsExactPHPCleanupAsTrailingRecoveryAuthority(t *testing.T) {
	stack, err := directCodingProjectStackByID(genericPHPServiceAdapter)
	if err != nil {
		t.Fatal(err)
	}
	if len(stack.CleanupCommands) != 1 ||
		strings.Join(stack.CleanupCommands[0].Args, " ") != phpExpectedCleanupCommand {
		t.Fatalf("registered PHP cleanup=%+v", stack.CleanupCommands)
	}

	specification, _, _, _, _ := phpServiceStackFixture(t)
	program, _ := phpAcceptanceFixture(t)
	primary, commands, err := (&directCodingSession{
		specification: &specification,
		program:       &program,
	}).directCodingJournalCommands()
	if err != nil {
		t.Fatal(err)
	}
	if len(primary) == 0 || len(commands) != len(primary)+1 {
		t.Fatalf("journal command counts primary=%d all=%d", len(primary), len(commands))
	}
	for index, command := range commands {
		wantRole := workspaceVerificationPrimary
		if index == len(commands)-1 {
			wantRole = workspaceVerificationCleanup
		}
		if command.WorkspaceRole != wantRole {
			t.Fatalf("journal command %d role=%q want=%q", index+1, command.WorkspaceRole, wantRole)
		}
	}
	cleanup := commands[len(commands)-1]
	if strings.Join(cleanup.Args, " ") != phpExpectedCleanupCommand {
		t.Fatalf("journal cleanup=%+v", cleanup)
	}
	intents := make([]queue.WorkspaceMutationVerificationIntent, len(commands))
	for index, command := range commands {
		raw, err := encodeWorkspaceVerificationCommand(command)
		if err != nil {
			t.Fatal(err)
		}
		intents[index] = queue.WorkspaceMutationVerificationIntent{
			Kind: workspaceVerificationEvidenceKind(command.Purpose), Command: raw,
		}
	}
	plan, err := queue.NewWorkspaceMutationVerificationPlan(intents)
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := workspaceVerificationCommandsFromPlan(plan)
	if err != nil {
		t.Fatal(err)
	}
	if recovered[len(recovered)-1].WorkspaceRole != workspaceVerificationCleanup ||
		strings.Join(recovered[len(recovered)-1].Args, " ") != phpExpectedCleanupCommand {
		t.Fatalf("recovered cleanup=%+v", recovered[len(recovered)-1])
	}
}

func TestStagedPHPVerificationAttemptsCleanupAfterSuccessAndFailure(t *testing.T) {
	for _, testCase := range []struct {
		name, failingCommand string
		wantVerificationErr  bool
	}{
		{name: "live verification succeeds"},
		{
			name:                "live verification fails after services start",
			failingCommand:      "compose run --rm --no-deps app php tests/HttpVerifier.php",
			wantVerificationErr: true,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			workspaceRoot := t.TempDir()
			t.Setenv("WORKSPACE_ROOT", workspaceRoot)
			t.Setenv("HOST_WORKSPACE_PATH", workspaceRoot)
			t.Setenv("DOCKER_HOST", v3RootfulDockerHost)
			logPath := installPHPDockerRecorder(t, testCase.failingCommand)
			program := phpCleanupVerificationProgram(t)
			created, err := newDirectCodingPHPProjectStageExecutor(
				&directCodingSession{
					runtime: &nativeRuntimeV3{ctx: context.Background()}, root: workspaceRoot,
				},
				program,
			)
			if err != nil {
				t.Fatal(err)
			}
			executor, ok := created.(*directCodingLanguageProjectStageExecutor)
			if !ok {
				t.Fatalf("PHP stage executor type=%T", created)
			}

			verificationErr := executor.VerifyFinal(&program)
			if (verificationErr != nil) != testCase.wantVerificationErr {
				t.Fatalf("verification error=%v want_error=%t", verificationErr, testCase.wantVerificationErr)
			}
			if err := executor.Close(); err != nil {
				t.Fatalf("close staged PHP executor: %v", err)
			}
			if _, err := os.Lstat(filepath.Join(workspaceRoot, ".omni")); !os.IsNotExist(err) {
				t.Fatalf("Docker-backed stage boundary persisted after cleanup: %v", err)
			}

			lines := phpRecordedDockerCommands(t, logPath)
			upIndex := phpCleanupStringIndex(lines, "compose up --detach --wait app nginx")
			verifierIndex := phpCleanupStringIndex(
				lines, "compose run --rm --no-deps app php tests/HttpVerifier.php",
			)
			cleanupIndex := phpCleanupStringIndex(lines, phpExpectedCleanupCommand)
			if upIndex < 0 || verifierIndex < 0 || cleanupIndex != len(lines)-1 ||
				upIndex >= verifierIndex || verifierIndex >= cleanupIndex {
				t.Fatalf("staged PHP Docker command order=%v", lines)
			}
		})
	}
}

func phpCleanupVerificationProgram(t *testing.T) directCodingProgram {
	t.Helper()
	program, _ := phpAcceptanceFixture(t)
	program.Generated["acceptance.001"] = `function verifyFeature101(): void {
    $result = feature101(taskInputFixture101(), []);
    RuntimeAssertions::requireResult($result);
    RuntimeAssertions::require($result, $result->output === '/records/1', 'route output mismatch');
    RuntimeAssertions::require($result, $result->error === '', 'unexpected feature failure');
}`
	return program
}

func installPHPDockerRecorder(t *testing.T, failingCommand string) string {
	t.Helper()
	directory := t.TempDir()
	logPath := filepath.Join(directory, "commands.log")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" != \"--host\" ] || [ \"$2\" != \"" + v3RootfulDockerHost + "\" ]; then exit 91; fi\n" +
		"shift 2\n" +
		"printf '%s\\n' \"$*\" >> " + strconv.Quote(logPath) + "\n"
	if failingCommand != "" {
		script += "if [ \"$*\" = " + strconv.Quote(failingCommand) + " ]; then exit 29; fi\n"
	}
	script += "exit 0\n"
	if err := os.WriteFile(filepath.Join(directory, "docker"), []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", directory+string(os.PathListSeparator)+"/usr/bin")
	return logPath
}

func phpRecordedDockerCommands(t *testing.T, logPath string) []string {
	t.Helper()
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(content)), "\n")
}

func phpCleanupStringIndex(values []string, expected string) int {
	for index, value := range values {
		if value == expected {
			return index
		}
	}
	return -1
}
