package worker

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationTaskLifecycleGeneratesAllBlocksBeforeFinalStage(t *testing.T) {
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
		FinalStage: func(complete *directCodingProgram) error {
			finalCalls++
			events = append(events, "final")
			if len(complete.Generated) != 4 {
				return fmt.Errorf("final generated=%d want 4", len(complete.Generated))
			}
			return nil
		},
	}
	if err := runDirectCodingApplicationTaskLifecycle(input, frozen, &program, hooks); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"generate:feature.001", "generate:acceptance.001",
		"generate:feature.002", "generate:acceptance.002",
		"final",
	}
	if !reflect.DeepEqual(events, want) || finalCalls != 1 {
		t.Fatalf("events=%v final_calls=%d want=%v/1", events, finalCalls, want)
	}
}

func TestApplicationTaskLifecycleGenerationFailureStartsNoLaterTaskOrFinalStage(t *testing.T) {
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
				if block.ID == "acceptance.001" {
					return "", errors.New("current task generation failed")
				}
				return block.Signature + " { return 1; }", nil
			},
			FinalStage: func(*directCodingProgram) error {
				events = append(events, "final")
				return nil
			},
		},
	)
	want := []string{"generate:feature.001", "generate:acceptance.001"}
	if err == nil || !strings.Contains(err.Error(), "task_001") || !reflect.DeepEqual(events, want) {
		t.Fatalf("error=%v events=%v want=%v", err, events, want)
	}
	if len(program.Generated) != 0 {
		t.Fatalf("failed task was merged into accepted program: %v", program.Generated)
	}
}

func TestApplicationTaskLifecycleWrapsEveryGeneratedTaskInCodeOwnedCognitionTransitions(t *testing.T) {
	t.Parallel()
	input, frozen, program := applicationTaskLifecycleFixture(t)
	var events []string
	err := runDirectCodingApplicationTaskLifecycle(input, frozen, &program, directCodingApplicationTaskLifecycleHooks{
		BeginTask: func(context assemblyline.ApplicationTaskContext) error {
			events = append(events, "begin:"+context.Task.TaskID)
			return nil
		},
		BuildBlock: func(_ assemblyline.ApplicationTaskContext, _ *directCodingProgram, block assemblyline.TypeScriptBlock) (string, error) {
			events = append(events, "generate:"+block.ID)
			return block.Signature + " { return 1; }", nil
		},
		CompleteTask: func(context assemblyline.ApplicationTaskContext, generated map[string]string) error {
			events = append(events, "complete:"+context.Task.TaskID)
			if len(generated) != 2 {
				t.Fatalf("task %s generated=%v", context.Task.TaskID, generated)
			}
			return nil
		},
		FinalStage: func(*directCodingProgram) error {
			events = append(events, "final")
			return nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"begin:task_001", "generate:feature.001", "generate:acceptance.001", "complete:task_001",
		"begin:task_002", "generate:feature.002", "generate:acceptance.002", "complete:task_002",
		"final",
	}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("events=%v want=%v", events, want)
	}
}
