package worker

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationTaskLifecycleGeneratesVerifiesThenAdvancesAndStagesOnce(t *testing.T) {
	t.Parallel()

	input, frozen, program := applicationTaskLifecycleFixture(t)
	var events []string
	finalCalls := 0
	hooks := directCodingApplicationTaskLifecycleHooks{
		BuildBlock: func(
			context assemblyline.ApplicationTaskContext,
			stage *directCodingProgram,
			block assemblyline.TypeScriptBlock,
		) (string, error) {
			events = append(events, "generate:"+block.ID)
			assertTaskStageOwnsOnly(t, *stage, context.Task.TaskID)
			return block.Signature + " { return 1; }", nil
		},
		Verify: func(context assemblyline.ApplicationTaskContext, stage *directCodingProgram) error {
			events = append(events, "verify:"+context.Task.TaskID)
			assertTaskStageOwnsOnly(t, *stage, context.Task.TaskID)
			if len(stage.Generated) != 2 {
				return fmt.Errorf("task stage generated=%d want 2", len(stage.Generated))
			}
			if context.Task.TaskID == "task_001" {
				stage.Generated["feature.001"] = "function Feature001View(): number { return 2; }"
			}
			return nil
		},
		FinalStage: func(complete *directCodingProgram) error {
			finalCalls++
			events = append(events, "final")
			if len(complete.Generated) != 4 {
				return fmt.Errorf("final generated=%d want 4", len(complete.Generated))
			}
			if !strings.Contains(complete.Generated["feature.001"], "return 2") {
				return fmt.Errorf("verified current-task correction was not retained")
			}
			return nil
		},
	}
	if err := runDirectCodingApplicationTaskLifecycle(input, frozen, &program, hooks); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"generate:feature.001", "generate:acceptance.001", "verify:task_001",
		"generate:feature.002", "generate:acceptance.002", "verify:task_002",
		"final",
	}
	if !reflect.DeepEqual(events, want) || finalCalls != 1 {
		t.Fatalf("events=%v final_calls=%d want=%v/1", events, finalCalls, want)
	}
}

func TestApplicationTaskLifecycleFailureStartsNoLaterTaskOrFinalStage(t *testing.T) {
	t.Parallel()

	input, frozen, program := applicationTaskLifecycleFixture(t)
	var events []string
	err := runDirectCodingApplicationTaskLifecycle(
		input, frozen, &program,
		directCodingApplicationTaskLifecycleHooks{
			BuildBlock: func(
				_ assemblyline.ApplicationTaskContext,
				_ *directCodingProgram,
				block assemblyline.TypeScriptBlock,
			) (string, error) {
				events = append(events, "generate:"+block.ID)
				return block.Signature + " { return 1; }", nil
			},
			Verify: func(context assemblyline.ApplicationTaskContext, _ *directCodingProgram) error {
				events = append(events, "verify:"+context.Task.TaskID)
				return errors.New("current task acceptance failed")
			},
			FinalStage: func(*directCodingProgram) error {
				events = append(events, "final")
				return nil
			},
		},
	)
	want := []string{"generate:feature.001", "generate:acceptance.001", "verify:task_001"}
	if err == nil || !strings.Contains(err.Error(), "task_001") || !reflect.DeepEqual(events, want) {
		t.Fatalf("error=%v events=%v want=%v", err, events, want)
	}
	if len(program.Generated) != 0 {
		t.Fatalf("failed task was merged into accepted program: %v", program.Generated)
	}
}

func TestApplicationTaskLifecycleRejectsCorrectionOutsideCurrentProjection(t *testing.T) {
	t.Parallel()

	input, frozen, program := applicationTaskLifecycleFixture(t)
	finalCalled := false
	err := runDirectCodingApplicationTaskLifecycle(
		input, frozen, &program,
		directCodingApplicationTaskLifecycleHooks{
			BuildBlock: func(
				_ assemblyline.ApplicationTaskContext,
				_ *directCodingProgram,
				block assemblyline.TypeScriptBlock,
			) (string, error) {
				return block.Signature + " { return 1; }", nil
			},
			Verify: func(_ assemblyline.ApplicationTaskContext, stage *directCodingProgram) error {
				stage.Generated["feature.002"] = "function Feature002View(): number { return 9; }"
				return nil
			},
			FinalStage: func(*directCodingProgram) error {
				finalCalled = true
				return nil
			},
		},
	)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "current task") {
		t.Fatalf("unowned correction error=%v", err)
	}
	if finalCalled || len(program.Generated) != 0 {
		t.Fatalf("unowned correction reached final=%v generated=%v", finalCalled, program.Generated)
	}
}
