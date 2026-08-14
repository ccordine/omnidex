package worker

import (
	"context"
	"os"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestGenericBrowserTaskStagesPassBeforeCompleteApplicationStage(t *testing.T) {
	if os.Getenv("OMNIDEX_NODE_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_NODE_INTEGRATION=1 to install the pinned browser toolchain")
	}
	specification := genericBrowserSpecification()
	program := stubGenericBrowserProgram(t)
	input := applicationWorkloadInput(specification)
	workspace, err := newDirectCodingTypeScriptStageWorkspace(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()

	err = executeDirectCodingApplicationWorkload(
		input, program.Workload,
		func(taskContext assemblyline.ApplicationTaskContext) error {
			stage, projectErr := projectDirectCodingApplicationTaskStage(program, taskContext)
			if projectErr != nil {
				return projectErr
			}
			commands, commandErr := directCodingApplicationTaskStageCommands(stage, taskContext)
			if commandErr != nil {
				return commandErr
			}
			if resetErr := resetDirectCodingTypeScriptStage(workspace.Root()); resetErr != nil {
				return resetErr
			}
			if writeErr := writeDirectCodingTypeScriptStage(workspace.Root(), stage); writeErr != nil {
				return writeErr
			}
			diagnostic, verifyErr := verifyDirectCodingTypeScriptStageCommands(
				context.Background(), workspace.Root(), stage, commands,
			)
			if verifyErr != nil {
				return verifyErr
			}
			if diagnostic != nil {
				t.Fatalf("task %s failed at %s:\n%s", taskContext.Task.TaskID, diagnostic.BlockID, diagnostic.Message)
			}
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := resetDirectCodingTypeScriptStage(workspace.Root()); err != nil {
		t.Fatal(err)
	}
	if err := writeDirectCodingTypeScriptStage(workspace.Root(), program); err != nil {
		t.Fatal(err)
	}
	diagnostic, err := verifyDirectCodingTypeScriptStage(
		context.Background(), workspace.Root(), program,
	)
	if err != nil {
		t.Fatal(err)
	}
	if diagnostic != nil {
		t.Fatalf("complete application failed at %s:\n%s", diagnostic.BlockID, diagnostic.Message)
	}
}

func TestStructuredVitestRoutingDistinguishesAssertionFromRuntimeException(t *testing.T) {
	if os.Getenv("OMNIDEX_NODE_INTEGRATION") != "1" {
		t.Skip("set OMNIDEX_NODE_INTEGRATION=1 to install the pinned browser toolchain")
	}
	program := stubGenericBrowserProgram(t)
	input := applicationWorkloadInput(genericBrowserSpecification())
	taskContext, err := assemblyline.ProjectApplicationTaskContext(input, program.Workload, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	baseStage, err := projectDirectCodingApplicationTaskStage(program, taskContext)
	if err != nil {
		t.Fatal(err)
	}
	commands, err := directCodingApplicationTaskStageCommands(baseStage, taskContext)
	if err != nil {
		t.Fatal(err)
	}
	workspace, err := newDirectCodingTypeScriptStageWorkspace(context.Background(), program)
	if err != nil {
		t.Fatal(err)
	}
	defer workspace.Close()

	for _, testCase := range []struct {
		name, acceptance, wantBlock string
	}{
		{
			name: "assertion behavior",
			acceptance: "async function VerifyFeature001(): Promise<void> { " +
				"expect('working').toBe('stopped'); }",
			wantBlock: "feature.001",
		},
		{
			name: "runtime exception",
			acceptance: "async function VerifyFeature001(): Promise<void> { " +
				"throw new ReferenceError('test declaration defect'); }",
			wantBlock: "acceptance.001",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			stage := baseStage
			stage.Generated = make(map[string]string, len(baseStage.Generated))
			for blockID, source := range baseStage.Generated {
				stage.Generated[blockID] = source
			}
			stage.Generated["acceptance.001"] = testCase.acceptance
			if err := resetDirectCodingTypeScriptStage(workspace.Root()); err != nil {
				t.Fatal(err)
			}
			if err := writeDirectCodingTypeScriptStage(workspace.Root(), stage); err != nil {
				t.Fatal(err)
			}
			diagnostic, err := verifyDirectCodingTypeScriptStageCommands(
				context.Background(), workspace.Root(), stage, commands,
			)
			if err != nil {
				t.Fatal(err)
			}
			if diagnostic == nil || diagnostic.BlockID != testCase.wantBlock {
				t.Fatalf("diagnostic=%+v want block %s", diagnostic, testCase.wantBlock)
			}
		})
	}
}
