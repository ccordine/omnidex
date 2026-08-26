package worker

import (
	"context"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const phpExpectedCleanupCommand = "compose down --rmi local --volumes --remove-orphans"

func TestDirectSessionDefersExactPHPCleanupAcrossLiveVerificationResults(t *testing.T) {
	stack, err := directCodingProjectStackByID(genericPHPServiceAdapter)
	if err != nil {
		t.Fatal(err)
	}
	if len(stack.CleanupCommands) != 1 ||
		strings.Join(stack.CleanupCommands[0].Args, " ") != phpExpectedCleanupCommand {
		t.Fatalf("registered PHP cleanup=%+v", stack.CleanupCommands)
	}

	program, _ := phpAcceptanceFixture(t)
	commands, err := phpServiceVerificationCommands(program)
	if err != nil {
		t.Fatal(err)
	}
	upIndex := phpCommandIndex(commands, "compose up --detach --wait app nginx")
	verifierIndex := phpCommandIndex(
		commands, "compose run --rm --no-deps app php tests/HttpVerifier.php",
	)
	if upIndex < 0 || verifierIndex != len(commands)-1 || upIndex >= verifierIndex {
		t.Fatalf("live PHP verification command order=%+v", commands)
	}

	verify := directCodingVerifyDeclaration(t)
	deferPosition, loopPosition := token.NoPos, token.NoPos
	cleanupCall, joinedReturnError := false, false
	ast.Inspect(verify.Body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.DeferStmt:
			foundCleanup := false
			ast.Inspect(typed.Call, func(n ast.Node) bool {
				if call, ok := n.(*ast.CallExpr); ok && phpCleanupSelectorName(call.Fun) == "executeDirectCodingCleanup" {
					foundCleanup = true
				}
				if assignment, ok := n.(*ast.AssignStmt); ok &&
					len(assignment.Lhs) == 1 && phpCleanupIdentifierName(assignment.Lhs[0]) == "returnErr" &&
					len(assignment.Rhs) == 1 {
					if call, ok := assignment.Rhs[0].(*ast.CallExpr); ok && phpCleanupSelectorName(call.Fun) == "Join" {
						joinedReturnError = true
					}
				}
				return true
			})
			if foundCleanup {
				deferPosition = typed.Pos()
				cleanupCall = true
			}
		case *ast.RangeStmt:
			if phpCleanupIdentifierName(typed.X) == "commands" {
				loopPosition = typed.Pos()
			}
		}
		return true
	})
	if !cleanupCall || !joinedReturnError || deferPosition == token.NoPos ||
		loopPosition == token.NoPos || deferPosition >= loopPosition {
		t.Fatalf(
			"direct Verify cleanup boundary call=%t joined=%t defer=%d loop=%d",
			cleanupCall, joinedReturnError, deferPosition, loopPosition,
		)
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
			socketPath, closeSocket := openV3DockerTestSocket(t)
			defer closeSocket()
			t.Setenv("WORKSPACE_ROOT", workspaceRoot)
			t.Setenv("HOST_WORKSPACE_PATH", workspaceRoot)
			t.Setenv("DOCKER_HOST", "unix://"+socketPath)
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
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + strconv.Quote(logPath) + "\n"
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

func directCodingVerifyDeclaration(t *testing.T) *ast.FuncDecl {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate PHP cleanup regression source")
	}
	path := filepath.Join(filepath.Dir(current), "v3_coding_driver_verification.go")
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range parsed.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if ok && function.Recv != nil && function.Name.Name == "Verify" {
			return function
		}
	}
	t.Fatal("direct coding session Verify declaration is missing")
	return nil
}

func phpCleanupSelectorName(expression ast.Expr) string {
	selector, ok := expression.(*ast.SelectorExpr)
	if !ok {
		return ""
	}
	return selector.Sel.Name
}

func phpCleanupIdentifierName(expression ast.Expr) string {
	identifier, ok := expression.(*ast.Ident)
	if !ok {
		return ""
	}
	return identifier.Name
}

func phpCommandIndex(commands []testCommand, expected string) int {
	for index, command := range commands {
		if strings.Join(command.Args, " ") == expected {
			return index
		}
	}
	return -1
}

func phpCleanupStringIndex(values []string, expected string) int {
	for index, value := range values {
		if value == expected {
			return index
		}
	}
	return -1
}
