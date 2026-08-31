package worker

import (
	"context"
	"fmt"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestApplicationRequirementCorrectionCollisionEvaporatesAsDuplicate(t *testing.T) {
	t.Parallel()
	const request = "Build a browser formatter that trims submitted text."
	const vague = "Display the correct result."
	const discarded = "Previously discarded candidate."
	authority, entry := directCodingRequirementQueueEntry(t, request, vague, nil)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			candidate := ""
			switch job.Kind {
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
			case assemblyline.WorkApplicationRequirementCandidateAuthorization:
				candidate = assemblyline.ApplicationRequirementCandidateEntailed
			case assemblyline.WorkApplicationRequirementCandidateResultRelation:
				var err error
				candidate, err = applicationRequirementCandidateResultPresenceForRelationForTest(
					job, assemblyline.ApplicationRequirementMissingResultRelation,
				)
				if err != nil {
					return assemblyline.PortableResult{}, err
				}
			case assemblyline.WorkApplicationRequirementCandidateResultRelationGrounding:
				candidate = assemblyline.ApplicationRequirementExactlyOneDeterminingRelationEntailed
			case assemblyline.WorkApplicationRequirementCandidateResultRelationCorrection:
				candidate = discarded
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %q", job.Kind)
			}
			return assemblyline.PortableResult{JobID: job.ID, Candidate: candidate}, nil
		},
	}
	resolved, err := resolveDirectCodingApplicationRequirementCandidate(
		runtime,
		"intent-model",
		authority,
		entry,
		nil,
		[]string{discarded},
		nil,
	)
	if err != nil || resolved.Disposition != directCodingApplicationRequirementDuplicate {
		t.Fatalf("collision resolution=%+v error=%v", resolved, err)
	}
}
