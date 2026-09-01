package worker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptBrowserExactAssemblyPassesRegisteredToolchain(t *testing.T) {
	if os.Getenv("OMNIDEX_RUN_BROWSER_TOOLCHAIN_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_RUN_BROWSER_TOOLCHAIN_INTEGRATION=1 to run the exact npm toolchain fixture")
	}
	for _, executable := range []string{"node", "npm"} {
		if _, err := exec.LookPath(executable); err != nil {
			t.Skipf("registered browser toolchain executable %s is unavailable: %v", executable, err)
		}
	}
	program := testTypeScriptBrowserProgram(
		t,
		"neutral state fixture",
		"A neutral state fixture",
		"Expose one observable state after an explicit activation.",
	)
	taskID := program.Workload.Tasks[0].ID
	implementationID, err := directCodingTaskBlockIDByRole(
		program.Source, taskID, assemblyline.SourceBlockTaskImplementation,
	)
	if err != nil {
		t.Fatal(err)
	}
	verificationID, err := directCodingTaskBlockIDByRole(
		program.Source, taskID, assemblyline.SourceBlockTaskVerification,
	)
	if err != nil {
		t.Fatal(err)
	}
	program.Generated[implementationID] = `function Feature001View({ state, capabilities, actions }: Feature001ViewProps): ReactElement {
  return (
    <div className="grid gap-2 p-2">
      <button type="button" onClick={() => actions.set('state', 'active')}>Activate state</button>
      <output aria-label="Current state">{String(state.state ?? '')}</output>
    </div>
  );
}`
	program.Generated[verificationID] = `async function VerifyFeature001(): Promise<void> {
  fireEvent.click(screen.getByRole('button', { name: 'Activate state' }));
  expect(screen.getByRole('status', { name: 'Current state' })).toHaveTextContent(/^active$/);
}`
	assembly, err := directCodingAssemblyFromProgram(program)
	if err != nil {
		t.Fatalf("assemble exact browser fixture: %v", err)
	}
	if err := validateDirectCodingProgramAssembly(program, assembly); err != nil {
		t.Fatalf("validate exact browser fixture: %v", err)
	}
	root := t.TempDir()
	for _, file := range assembly.Files {
		if err := writeDirectCodingStageFile(root, file); err != nil {
			t.Fatalf("materialize exact browser fixture: %v", err)
		}
	}
	cacheRoot, err := os.MkdirTemp("", "omnidex-browser-integration-cache-")
	if err != nil {
		t.Fatalf("create isolated integration cache: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(cacheRoot); err != nil {
			t.Errorf("remove isolated integration cache: %v", err)
		}
	}()

	for _, component := range []string{"node", "npm"} {
		result := runExactBrowserIntegrationCommand(
			t, root, directCodingToolchainVersionCommand(component),
		)
		if strings.TrimSpace(string(result.Stderr)) != "" {
			t.Fatalf("%s version probe wrote stderr: %s", component, result.Stderr)
		}
		if err := validateDirectCodingToolchainVersion(
			program.Project.Profile, component, result.Stdout,
		); err != nil {
			t.Fatal(err)
		}
	}
	install, err := directCodingNPMInstallCommand(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	runExactBrowserIntegrationCommand(t, root, install)
	for _, command := range directCodingImplementationStageCommands() {
		runExactBrowserIntegrationCommand(t, root, command)
	}
	contextAuthority, err := assemblyline.ProjectApplicationTaskContext(
		program.Workload, taskID,
	)
	if err != nil {
		t.Fatal(err)
	}
	focused, err := directCodingApplicationTaskStageCommands(program, contextAuthority)
	if err != nil {
		t.Fatal(err)
	}
	for _, command := range focused {
		runExactBrowserIntegrationCommand(t, root, command)
	}
	for _, command := range directCodingFullTypeScriptStageCommands() {
		runExactBrowserIntegrationCommand(t, root, command)
	}
	if err := validateDirectCodingBrowserProductionArtifacts(root); err != nil {
		t.Fatalf("validate exact production browser artifacts: %v", err)
	}
}

func runExactBrowserIntegrationCommand(
	t *testing.T,
	root string,
	command directCodingVerificationCommand,
) directCodingVerificationCommandResult {
	t.Helper()
	environment, err := directCodingVerificationProcessEnvironment(command.Environment)
	if err != nil {
		t.Fatalf("construct exact command environment: %v", err)
	}
	timeout := command.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	commandContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	process := exec.CommandContext(commandContext, command.Argv[0], command.Argv[1:]...)
	process.Dir = root
	process.Env = environment
	if command.Stdin != nil {
		process.Stdin = bytes.NewReader(command.Stdin)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	process.Stdout = &stdout
	process.Stderr = &stderr
	if err := process.Run(); err != nil {
		t.Fatalf(
			"exact browser command %s failed: %v\nstdout:\n%s\nstderr:\n%s",
			fmt.Sprint(command.Argv), err, stdout.String(), stderr.String(),
		)
	}
	if err := commandContext.Err(); err != nil {
		t.Fatalf("exact browser command %v context ended: %v", command.Argv, err)
	}
	return directCodingVerificationCommandResult{
		Stdout: append([]byte{}, stdout.Bytes()...),
		Stderr: append([]byte{}, stderr.Bytes()...),
	}
}
