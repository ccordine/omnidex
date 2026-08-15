package worker

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestExactApplicationJobSpecificationCorrectsRetainedInitialDefectBeforeReview(t *testing.T) {
	t.Parallel()
	job := exactApplicationSpecificationJob(t, "schedule appointments")
	result, err := convergeExactApplicationJobSpecificationWithReplay(
		context.Background(), 61, 63, job, "planner", "reviewer",
		func(current assemblyline.PortableJob, model string, number int) (ExactStationReplay, error) {
			switch number {
			case 1:
				replay := exactApplicationSpecificationTestReplay(current, model,
					`{"objective":"Implement appointment scheduling.","required_behaviors":["Users can schedule an appointment.","Users can schedule an appointment."],"acceptance_criteria":["A scheduled appointment is visible."]}`)
				return replay, &ExactStationReplayArtifactError{
					Cause: fmt.Errorf("required behavior 1 duplicates earlier item 0"),
				}
			case 2:
				if model != "planner" || current.Kind != assemblyline.WorkResponseCorrection {
					return ExactStationReplay{}, fmt.Errorf("correction model=%s kind=%s", model, current.Kind)
				}
				prompt, schema, renderErr := assemblyline.RenderPortableJob(current)
				if renderErr != nil {
					return ExactStationReplay{}, renderErr
				}
				properties, _ := schema["properties"].(map[string]any)
				if len(properties) != 1 || properties["required_behaviors_002"] == nil ||
					strings.Contains(prompt, "Users can schedule an appointment") {
					return ExactStationReplay{}, fmt.Errorf("correction leaked retained state or field authority: %s %#v", prompt, schema)
				}
				return exactApplicationSpecificationTestReplay(current, model,
					`{"required_behaviors_002":"Submitting the choice creates the appointment."}`), nil
			case 3:
				if model != "reviewer" || current.Kind != assemblyline.WorkApplicationJobSpecificationReview {
					return ExactStationReplay{}, fmt.Errorf("review model=%s kind=%s", model, current.Kind)
				}
				return exactApplicationSpecificationTestReplay(current, model, `{"decision":"accept"}`), nil
			default:
				return ExactStationReplay{}, fmt.Errorf("unexpected call %d", number)
			}
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Terminal != "accepted" || len(result.Calls) != 3 ||
		len(result.Specification.RequiredBehaviors) != 2 {
		t.Fatalf("convergence=%+v", result)
	}
}

func TestExactApplicationJobSpecificationInitialCorrectionContinuesBeyondThreeProgressStates(t *testing.T) {
	t.Parallel()
	job := exactApplicationSpecificationJob(t, "filter inventory")
	responses := []string{
		`{"objective":"","required_behaviors":["A","A","A","A"],"acceptance_criteria":["X","X","X","X"]}`,
		`{"objective":"Implement inventory filtering."}`,
		`{"required_behaviors_002":"B"}`,
		`{"required_behaviors_003":"C"}`,
		`{"required_behaviors_004":"D"}`,
		`{"acceptance_criteria_002":"Y"}`,
		`{"acceptance_criteria_003":"Z"}`,
		`{"acceptance_criteria_004":"W"}`,
		`{"decision":"accept"}`,
	}
	result, err := convergeExactApplicationJobSpecificationWithReplay(
		context.Background(), 67, 69, job, "planner", "reviewer",
		func(current assemblyline.PortableJob, model string, number int) (ExactStationReplay, error) {
			if number < 1 || number > len(responses) {
				return ExactStationReplay{}, fmt.Errorf("unexpected call %d", number)
			}
			wantKind := assemblyline.WorkResponseCorrection
			wantModel := "planner"
			if number == 1 {
				wantKind = assemblyline.WorkApplicationJobSpecification
			}
			if number == len(responses) {
				wantKind, wantModel = assemblyline.WorkApplicationJobSpecificationReview, "reviewer"
			}
			if current.Kind != wantKind || model != wantModel {
				return ExactStationReplay{}, fmt.Errorf("call %d model=%s kind=%s", number, model, current.Kind)
			}
			return exactApplicationSpecificationTestReplay(current, model, responses[number-1]), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Terminal != "accepted" || len(result.Calls) != 9 {
		t.Fatalf("convergence=%+v", result)
	}
}

func TestExactApplicationJobSpecificationInitialCorrectionStopsNoOpAndCycle(t *testing.T) {
	t.Parallel()
	job := exactApplicationSpecificationJob(t, "manage a reading list")
	tests := map[string][]string{
		"no_op": {
			`{"objective":"Implement a reading list.","required_behaviors":["A","A"],"acceptance_criteria":["X"]}`,
			`{"required_behaviors_002":"A"}`,
		},
		"cycle": {
			`{"objective":"Implement a reading list.","required_behaviors":["A","A","A","A"],"acceptance_criteria":["X"]}`,
			`{"required_behaviors_002":"B"}`,
			`{"required_behaviors_003":"B"}`,
			`{"required_behaviors_003":"A"}`,
		},
	}
	for name, responses := range tests {
		name, responses := name, responses
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			calls := 0
			_, err := convergeExactApplicationJobSpecificationWithReplay(
				context.Background(), 71, 73, job, "planner", "reviewer",
				func(current assemblyline.PortableJob, model string, number int) (ExactStationReplay, error) {
					calls++
					if number > len(responses) {
						return ExactStationReplay{}, fmt.Errorf("unexpected call %d", number)
					}
					return exactApplicationSpecificationTestReplay(current, model, responses[number-1]), nil
				},
			)
			if err == nil || calls != len(responses) {
				t.Fatalf("err=%v calls=%d", err, calls)
			}
		})
	}
}

func exactApplicationSpecificationJob(t *testing.T, quote string) assemblyline.PortableJob {
	t.Helper()
	requirement := assemblyline.Requirement{ID: "requirement_001", SourceQuote: quote}
	job, err := assemblyline.NewApplicationJobSpecificationJob(assemblyline.ApplicationJobSpecificationInput{
		Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: "browser tool",
		AcceptedRequirements: []assemblyline.Requirement{requirement}, FocusedRequirement: requirement,
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}
