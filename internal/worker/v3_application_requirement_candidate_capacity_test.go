package worker

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationRequirementQueueStopsSemanticWorkAtAcceptedLeafCapacity(t *testing.T) {
	t.Parallel()
	candidates := make([]string, assemblyline.MaxApplicationRequirementLeaves+1)
	for index := range candidates {
		candidates[index] = fmt.Sprintf("Display the current status for channel %02d.", index+1)
	}
	request := fmt.Sprintf(
		"Build a browser status board that displays the current status for each channel numbered 1 through %d.",
		len(candidates),
	)
	accepted := candidates[:assemblyline.MaxApplicationRequirementLeaves]
	tail := candidates[assemblyline.MaxApplicationRequirementLeaves]
	inventory := strings.Join(candidates, "\n")
	runtime := typedWorkerRuntime{
		Context: t.Context(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			if strings.Contains(string(job.Payload), tail) {
				return assemblyline.PortableResult{}, fmt.Errorf(
					"capacity tail reached semantic work %q", job.Kind,
				)
			}
			candidate := ""
			switch job.Kind {
			case assemblyline.WorkApplicationProductContext:
				candidate = "A browser runtime-outcome utility."
			case assemblyline.WorkApplicationRequirementInventory:
				candidate = inventory
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				candidate = assemblyline.ApplicationRequirementCandidateEntailed
			case assemblyline.WorkApplicationRequirementCandidateKind:
				var err error
				candidate, err = applicationRequirementCandidateContentPresenceForKindForTest(
					job,
					assemblyline.ApplicationRequirementCandidateTaskLocal,
				)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
			case assemblyline.WorkApplicationRequirementCandidateOutcomeRelation:
				candidate = assemblyline.ApplicationRequirementDistinctRuntimeOutcomes
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				presence, presenceErr := applicationRequirementCandidateResultPresenceForRelationForTest(
					job, assemblyline.ApplicationRequirementNoDerivedResult,
				)
				if presenceErr != nil {
					return assemblyline.PortableResult{}, presenceErr
				}
				candidate = presence
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}

	resolution, err := resolveApplicationRequirementQueueFixture(t, runtime, request)
	if err != nil {
		t.Fatal(err)
	}
	if got := applicationRequirementStatements(resolution); !reflect.DeepEqual(got, accepted) {
		t.Fatalf("requirements=%v want=%v", got, accepted)
	}
}

func TestApplicationRequirementQueueContinuesAfterUnresolvedCandidate(t *testing.T) {
	t.Parallel()
	const request = "Build a browser status utility that displays the submitted status."
	const unresolved = "Candidate wording with no classified content."
	const accepted = "Display the submitted status."
	counts := map[assemblyline.WorkKind]int{}
	runtime := typedWorkerRuntime{
		Context: t.Context(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			counts[job.Kind]++
			candidate := ""
			switch job.Kind {
			case assemblyline.WorkApplicationProductContext:
				candidate = "A browser status utility."
			case assemblyline.WorkApplicationRequirementInventory:
				candidate = unresolved + "\n" + accepted
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				candidate = assemblyline.ApplicationRequirementCandidateEntailed
			case assemblyline.WorkApplicationRequirementCandidateKind:
				leaf, err := applicationRequirementKindCandidate(job)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
				if leaf == unresolved {
					candidate = string(assemblyline.ApplicationRequirementCandidateContentAbsent)
					break
				}
				candidate, err = applicationRequirementCandidateContentPresenceForKindForTest(
					job,
					assemblyline.ApplicationRequirementCandidateTaskLocal,
				)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
			case assemblyline.WorkApplicationRequirementCandidateCardinality:
				candidate = assemblyline.ApplicationRequirementOneRuntimeOutcome
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				presence, presenceErr := applicationRequirementCandidateResultPresenceForRelationForTest(
					job, assemblyline.ApplicationRequirementNoDerivedResult,
				)
				if presenceErr != nil {
					return assemblyline.PortableResult{}, presenceErr
				}
				candidate = presence
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}

	resolution, err := resolveApplicationRequirementQueueFixture(t, runtime, request)
	if err != nil {
		t.Fatal(err)
	}
	if got := applicationRequirementStatements(resolution); !reflect.DeepEqual(got, []string{accepted}) {
		t.Fatalf("requirements=%v", got)
	}
	wantCalls := map[assemblyline.WorkKind]int{
		assemblyline.WorkApplicationProductContext:                     1,
		assemblyline.WorkApplicationRequirementInventory:               1,
		assemblyline.WorkApplicationRequirementCandidateAuthorization:  2,
		assemblyline.WorkApplicationRequirementCandidateKind:           3,
		assemblyline.WorkApplicationRequirementCandidateCardinality:    1,
		assemblyline.WorkApplicationRequirementCandidateResultRelation: 1,
	}
	if !reflect.DeepEqual(counts, wantCalls) {
		t.Fatalf("semantic calls=%v want=%v", counts, wantCalls)
	}
}
