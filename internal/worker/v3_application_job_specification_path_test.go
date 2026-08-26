package worker

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/modelcontext"
)

func TestApplicationJobSpecificationRejectsKnownArtifactBeforeInferenceOrAcceptance(t *testing.T) {
	t.Parallel()
	provenance, err := modelcontext.NewArtifactIdentityProvenance(
		[]string{"internal/transport.go"},
	)
	if err != nil {
		t.Fatal(err)
	}
	base := assemblyline.ApplicationJobSpecificationInput{
		Surface:      assemblyline.ApplicationSurfaceBrowser,
		ProductQuote: "A Node.js status panel",
		AcceptedRequirements: []assemblyline.Requirement{{
			ID: "requirement_001", SourceQuote: "Show the current status.",
		}},
		FocusedRequirement: assemblyline.Requirement{
			ID: "requirement_001", SourceQuote: "Show the current status.",
		},
	}

	t.Run("known input basename", func(t *testing.T) {
		authority := base
		authority.ProductQuote = "A transport.go status panel"
		job, err := assemblyline.NewApplicationJobSpecificationJob(authority)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		runtime := typedWorkerRuntime{
			Context: context.Background(), MaxAttempts: 3, PathProvenance: provenance,
			Execute: func(assemblyline.PortableJob, string) (assemblyline.PortableResult, error) {
				calls++
				return assemblyline.PortableResult{}, nil
			},
		}
		_, err = runProgressiveApplicationJobSpecificationDraft(
			runtime, "planner", "opaque", job, authority,
		)
		if err == nil || !strings.Contains(err.Error(), "transport.go") || calls != 0 {
			t.Fatalf("known input path error=%v calls=%d", err, calls)
		}
	})

	t.Run("known output basename", func(t *testing.T) {
		job, err := assemblyline.NewApplicationJobSpecificationJob(base)
		if err != nil {
			t.Fatal(err)
		}
		calls := 0
		runtime := typedWorkerRuntime{
			Context: context.Background(), MaxAttempts: 3, PathProvenance: provenance,
			Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
				calls++
				raw, marshalErr := json.Marshal(assemblyline.ApplicationJobSpecification{
					Objective:          "Update transport.go status behavior",
					RequiredBehaviors:  []string{"Show the current status"},
					AcceptanceCriteria: []string{"The current status is visible"},
				})
				return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, marshalErr
			},
		}
		_, err = runProgressiveApplicationJobSpecificationDraft(
			runtime, "planner", "opaque", job, base,
		)
		if err == nil || !strings.Contains(err.Error(), "transport.go") || calls != 1 {
			t.Fatalf("known output path error=%v calls=%d", err, calls)
		}
	})
}

func TestApplicationJobSpecificationRetainsNodeJSWithoutProvenance(t *testing.T) {
	t.Parallel()
	value := assemblyline.ApplicationJobSpecification{
		Objective:          "Implement a Node.js status behavior",
		RequiredBehaviors:  []string{"Expose the Node.js status"},
		AcceptanceCriteria: []string{"The Node.js status is visible"},
	}
	if err := value.ValidatePathFree(assemblyline.ArtifactIdentityProvenance{}); err != nil {
		t.Fatalf("unproven Node.js was rejected: %v", err)
	}
}

func TestApplicationJobSpecificationStopsAfterDistinctInvalidAttempts(t *testing.T) {
	t.Parallel()
	requirement := assemblyline.Requirement{
		ID: "requirement_001", SourceQuote: "Show the current status.",
	}
	authority := assemblyline.ApplicationJobSpecificationInput{
		Surface: assemblyline.ApplicationSurfaceBrowser, ProductQuote: "A status panel",
		AcceptedRequirements: []assemblyline.Requirement{requirement}, FocusedRequirement: requirement,
	}
	job, err := assemblyline.NewApplicationJobSpecificationJob(authority)
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 3,
		Execute: func(portable assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls++
			var candidate string
			switch calls {
			case 1:
				raw, marshalErr := json.Marshal(assemblyline.ApplicationJobSpecification{
					Objective: "Implement status presentation",
					RequiredBehaviors: []string{
						"Show the status", "Show the status",
						"Refresh the status", "Refresh the status",
					},
					AcceptanceCriteria: []string{
						"The status is visible", "The status is visible",
					},
				})
				if marshalErr != nil {
					return assemblyline.PortableResult{}, marshalErr
				}
				candidate = string(raw)
			case 2:
				candidate = `{"required_behaviors_002":"Record a status change"}`
			case 3:
				candidate = `{"required_behaviors_004":"Present status history"}`
			default:
				t.Fatalf("unexpected application specification inference call %d", calls)
			}
			return assemblyline.PortableResult{JobID: portable.ID, Candidate: candidate}, nil
		},
	}
	_, err = runProgressiveApplicationJobSpecificationDraft(
		runtime, "planner", "opaque", job, authority,
	)
	if err == nil || !strings.Contains(err.Error(), "failed after 3 bounded attempts") || calls != 3 {
		t.Fatalf("bounded specification error=%v calls=%d", err, calls)
	}
}
