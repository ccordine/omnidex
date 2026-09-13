package worker

import (
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationLifecyclePublishesBeforeLaterTaskFailure(t *testing.T) {
	for _, failure := range []string{"generation", "verification", "publication", "final"} {
		t.Run(failure, func(t *testing.T) {
			program := publicationProgramFixture(t)
			bodies := program.Generated
			program.Generated = make(map[string]string)
			var events []string
			failureErr := errors.New("observed " + failure + " failure")
			finalCalls := 0
			err := runDirectCodingApplicationTaskLifecycle(program.Workload, &program, directCodingApplicationTaskLifecycleHooks{
				BuildBlock: func(context assemblyline.ApplicationTaskContext, _ *directCodingProgram, ref assemblyline.SourceBlockRef) (string, error) {
					if context.Task.TaskID == "task_002" {
						if len(events) != 2 || events[0] != "verify:task_001" || events[1] != "publish:task_001" {
							t.Fatalf("later task began before verified publication: %v", events)
						}
						if failure == "generation" {
							return "", failureErr
						}
					}
					return bodies[ref.Block.ID], nil
				},
				VerifyTask: func(context assemblyline.ApplicationTaskContext, _ *directCodingProgram) error {
					if context.Task.TaskID == "task_002" && failure == "verification" {
						return failureErr
					}
					events = append(events, "verify:"+context.Task.TaskID)
					return nil
				},
				PublishTask: func(accepted, isolated *directCodingProgram) error {
					taskID := "task_001"
					if len(accepted.Generated) > 2 {
						taskID = "task_002"
					}
					if len(isolated.Generated) != 2 || accepted.Generated["feature.001"] != bodies["feature.001"] {
						t.Fatal("publication lost retained or isolated source authority")
					}
					if taskID == "task_002" && failure == "publication" {
						return failureErr
					}
					events = append(events, "publish:"+taskID)
					return nil
				},
				FinalStage: func(*directCodingProgram) error {
					finalCalls++
					return failureErr
				},
			})
			if !errors.Is(err, failureErr) || program.Generated["feature.001"] != bodies["feature.001"] {
				t.Fatalf("failure discarded accepted work: %v", err)
			}
			if (finalCalls == 1) != (failure == "final") {
				t.Fatalf("final verification ran with incomplete tasks: %d", finalCalls)
			}
		})
	}
}

func TestApplicationLifecycleRejectsMissingPublicationBeforeGeneration(t *testing.T) {
	program := publicationProgramFixture(t)
	program.Generated = make(map[string]string)
	calls := 0
	err := runDirectCodingApplicationTaskLifecycle(program.Workload, &program, directCodingApplicationTaskLifecycleHooks{
		BuildBlock: func(assemblyline.ApplicationTaskContext, *directCodingProgram, assemblyline.SourceBlockRef) (string, error) {
			calls++
			return "", nil
		},
		VerifyTask: func(assemblyline.ApplicationTaskContext, *directCodingProgram) error { return nil },
		FinalStage: func(*directCodingProgram) error { return nil },
	})
	if err == nil || !strings.Contains(err.Error(), "publication hook") || calls != 0 {
		t.Fatalf("missing publication ran generation: %d %v", calls, err)
	}
}
