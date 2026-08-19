package worker

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestRepositorySurfaceRetainsExplicitUnresolvedRequirementForDesiredState(t *testing.T) {
	t.Parallel()
	pack := repositoryProjectionTestPack(t)
	requirement := "Add func Added() int returning two."
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			decision := assemblyline.RepositoryChangeSurfaceDecision{
				Schema:  assemblyline.RepositoryChangeSurfaceSchemaV2,
				Targets: []assemblyline.RepositoryChangeTarget{},
			}
			raw, err := json.Marshal(decision)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, err
		},
	}
	decision, err := selectExistingRepositoryChangeSurface(
		runtime, "semantic", requirement, []string{requirement}, pack, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	unresolved, err := decision.UnresolvedRequirements(assemblyline.RepositoryChangeSurfaceInput{
		ResearchNeed: requirement,
		Requirements: []string{requirement},
		Evidence:     pack,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(decision.Targets) != 0 || len(unresolved) != 1 || unresolved[0] != requirement {
		t.Fatalf("retained unresolved desired state=%+v", decision)
	}
}
