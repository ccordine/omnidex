package worker

import (
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestTypeScriptBrowserExactAssemblyPassesRegisteredDockerToolchain(t *testing.T) {
	for _, fixture := range []struct{ name, requirement, action, failedAction, output, expected string }{
		{"text state", "Expose one observable state after an explicit activation.", "actions.set('state', 'active')", "actions.set('state', 'incorrect')", "String(state.state ?? '')", "active"},
		{"numeric state", "Expose a numeric value after an explicit activation.", "actions.set('value', 12)", "actions.set('value', 13)", "String(state.value ?? '')", "12"},
	} {
		for _, broken := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/broken=%t", fixture.name, broken), func(t *testing.T) {
				program := testTypeScriptBrowserProgram(t, "A constructed browser fixture", fixture.requirement)
				taskID := program.Workload.Tasks[0].ID
				implementationID, err := directCodingTaskBlockIDByRole(program.Source, taskID, assemblyline.SourceBlockTaskImplementation)
				if err != nil {
					t.Fatal(err)
				}
				verificationID, err := directCodingTaskBlockIDByRole(program.Source, taskID, assemblyline.SourceBlockTaskVerification)
				if err != nil {
					t.Fatal(err)
				}
				action := fixture.action
				if broken {
					action = fixture.failedAction
				}
				implementationBlock, exists := directCodingSourceBlueprintBlock(program.Source, implementationID)
				if !exists {
					t.Fatal("browser fixture lacks its implementation declaration")
				}
				program.Generated[implementationID] = fmt.Sprintf(`%s {
  return (
    <div className="grid gap-2 p-2">
      <button type="button" onClick={() => %s}>Activate state</button>
      <output aria-label="Current state">{%s}</output>
    </div>
  );
}`, implementationBlock.Signature, action, fixture.output)
				program.Generated[verificationID] = fmt.Sprintf(`async function VerifyFeature001(): Promise<void> {
  fireEvent.click(screen.getByRole('button', { name: 'Activate state' }));
  expect(screen.getByRole('status', { name: 'Current state' })).toHaveTextContent(/^%s$/);
}`, fixture.expected)
				records := runConstructedDockerPublicationFixture(t, program, broken, false)
				installs := 0
				for _, record := range records {
					if len(record.Argv) > 1 && record.Argv[0] == "npm" && record.Argv[1] == "ci" {
						installs++
						if !*record.ContainerNetworkEnabled {
							t.Fatal("dependency installation lacked observed acquisition network")
						}
					}
				}
				want := 2
				if broken {
					want = 1
				}
				if installs != want {
					t.Fatalf("dependency installs=%d; want %d", installs, want)
				}
			})
		}
	}
}

// Only generation is supplied by the fixture. The production implementation
// compiler, surface binding, task checks, publication, and final checks run.
type browserDockerFixtureExecutor struct {
	*directCodingTypeScriptProjectStageExecutor
	declarations map[string]string
}

func (executor *browserDockerFixtureExecutor) GenerateBlock(context assemblyline.ApplicationTaskContext, stage *directCodingProgram, ref assemblyline.SourceBlockRef) (string, error) {
	source := executor.declarations[ref.Block.ID]
	switch ref.Block.Role {
	case assemblyline.SourceBlockTaskImplementation:
		return executor.closeImplementationBeforeVerification(stage, ref, source)
	case assemblyline.SourceBlockTaskVerification:
		if _, err := executor.bindBrowserPublicSurface(context, stage, ref); err != nil {
			return "", err
		}
	}
	return source, nil
}
