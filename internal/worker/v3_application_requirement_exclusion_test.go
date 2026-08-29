package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestDirectCodingIntentExcludesNonRuntimeCandidateAndContinuesCoverage(t *testing.T) {
	t.Parallel()
	const request = "Create a browser status board that displays the current status in one source file."
	const excluded = "Keep the project in one source file."
	const retained = "Display the current status."
	applicationContext, err := assemblyline.BootstrapApplicationContext(
		request, assemblyline.ApplicationWorkspaceEmpty,
	)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[assemblyline.WorkKind]int{}
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			counts[job.Kind]++
			var candidate string
			switch job.Kind {
			case assemblyline.WorkApplicationProductContext:
				candidate = "A browser status board."
			case assemblyline.WorkApplicationRequirementCoverage:
				var input assemblyline.ApplicationRequirementCoverageInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				switch counts[job.Kind] {
				case 1:
					if len(input.AcceptedRequirements) != 0 || len(input.ExcludedCandidates) != 0 {
						return assemblyline.PortableResult{}, fmt.Errorf("initial coverage state=%+v", input)
					}
					candidate = assemblyline.ApplicationRequirementRemains
				case 2:
					if !reflect.DeepEqual(input.AcceptedRequirements, []string{retained}) ||
						!reflect.DeepEqual(input.ExcludedCandidates, []string{excluded}) {
						return assemblyline.PortableResult{}, fmt.Errorf("terminal coverage state=%+v", input)
					}
					candidate = assemblyline.ApplicationNoUncoveredRequirement
				default:
					return assemblyline.PortableResult{}, fmt.Errorf("unexpected coverage call")
				}
			case assemblyline.WorkApplicationRequirement:
				var input assemblyline.ApplicationRequirementCandidateInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if err := input.Coverage.ValidateFor(input.Authority); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if counts[job.Kind] == 1 {
					candidate = excluded
				} else {
					if !reflect.DeepEqual(input.Authority.ExcludedCandidates, []string{excluded}) {
						return assemblyline.PortableResult{}, fmt.Errorf("generation omitted exclusions: %+v", input.Authority)
					}
					candidate = retained
				}
			case assemblyline.WorkApplicationRequirementCandidateKind:
				var input assemblyline.ApplicationRequirementCandidateKindInput
				if err := json.Unmarshal(job.Payload, &input); err != nil {
					return assemblyline.PortableResult{}, err
				}
				if input.Candidate == excluded {
					candidate = assemblyline.ApplicationRequirementCandidateNonRuntime
				} else if input.Candidate == retained {
					candidate = assemblyline.ApplicationRequirementCandidateTaskLocal
				} else {
					return assemblyline.PortableResult{}, fmt.Errorf("unexpected kind candidate %q", input.Candidate)
				}
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	resolution, err := resolveDirectCodingApplicationIntent(
		runtime, "intent-model",
		assemblyline.ApplicationIntentInput{UserRequest: request, Context: applicationContext},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(resolution.Requirements) != 1 || resolution.Requirements[0].Statement != retained {
		t.Fatalf("resolution=%+v", resolution)
	}
	if counts[assemblyline.WorkApplicationRequirementCoverage] != 2 ||
		counts[assemblyline.WorkApplicationRequirement] != 2 ||
		counts[assemblyline.WorkApplicationRequirementCandidateKind] != 2 ||
		counts[assemblyline.WorkApplicationRequirementCandidateCardinality] != 1 ||
		counts[assemblyline.WorkApplicationRequirementCandidateDuplicateReplacement] != 0 {
		t.Fatalf("fixed-point calls=%v", counts)
	}
}
