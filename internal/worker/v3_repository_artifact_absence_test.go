package worker

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
)

func TestPathFreeArtifactAbsenceQuotesExcludeNamedArtifactDecisions(t *testing.T) {
	t.Parallel()
	named := "ARTIFACT_1 must no longer exist."
	pathFree := "The obsolete semantic capability and all behavior it owns must no longer exist."
	got, err := pathFreeArtifactAbsenceQuotes(
		[]string{named, pathFree},
		[]assemblyline.ArtifactDirective{{
			Token: "ARTIFACT_1", Disposition: assemblyline.ArtifactForbid,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != pathFree {
		t.Fatalf("path-free quotes=%v", got)
	}
}

func TestRepositoryArtifactAbsencePartitionPreservesOneDecisionPerQuote(t *testing.T) {
	t.Parallel()
	quotes := []string{
		"The obsolete semantic artifact must no longer exist.",
		"The retained behavior must be updated.",
	}
	answers := []assemblyline.RepositoryArtifactAbsenceRelation{
		assemblyline.RepositoryArtifactMustBeAbsent,
		assemblyline.RepositoryArtifactAbsenceNotExplicit,
	}
	call := 0
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			if job.Kind != assemblyline.WorkRepositoryArtifactAbsence || call >= len(answers) {
				t.Fatalf("unexpected semantic call %d kind=%q", call, job.Kind)
			}
			answer := answers[call]
			call++
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(answer)}, nil
		},
	}
	partition, err := classifyRepositoryArtifactAbsenceQuotes(
		runtime, "semantic", quotes, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if call != len(quotes) || len(partition.MustBeAbsent) != 1 ||
		partition.MustBeAbsent[0] != quotes[0] || len(partition.NotExplicit) != 1 ||
		partition.NotExplicit[0] != quotes[1] {
		t.Fatalf("partition=%+v calls=%d", partition, call)
	}
}
