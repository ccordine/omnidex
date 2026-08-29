package worker

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestRepositoryRequirementSurfaceReceivesOnlyItsExactGapProjection(t *testing.T) {
	t.Parallel()
	pack := repositoryProjectionTestPack(t)
	requirementQuote := "Change the exact owner."
	var input assemblyline.RepositoryChangeOwnerInput
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			if job.Kind != assemblyline.WorkRepositoryChangeOwner {
				t.Fatalf("work kind=%q", job.Kind)
			}
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				t.Fatal(err)
			}
			return assemblyline.PortableResult{
				JobID: job.ID, Candidate: pack.Symbols[0].ID,
			}, nil
		},
	}
	decision, err := selectExistingRepositoryRequirementSurface(
		runtime, "qwen",
		existingRepositoryEvidenceAcquisition{
			RequirementQuote: requirementQuote, Query: "Owner", Pack: pack,
		},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if input.Authority.ResearchNeed != requirementQuote ||
		!reflect.DeepEqual(input.Authority.Requirements, []string{requirementQuote}) ||
		input.FocusedRequirement != requirementQuote ||
		len(decision.Targets) != 1 {
		t.Fatalf("surface input=%#v decision=%#v", input, decision)
	}
}

func TestRepositoryEvidenceClosureUsesOneExactCodeOwnedQueryForAllRequirements(t *testing.T) {
	t.Parallel()
	const query = "Change both existing behaviors without altering unrelated behavior."
	queries := make([]string, 0, 1)
	results, err := acquireExistingRepositoryEvidence(
		[]string{"first exact requirement", "second exact requirement"},
		query,
		func(query string) (repositoryretrieval.EvidencePack, error) {
			queries = append(queries, query)
			return repositoryAcquisitionTestPack(t, query), nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(queries, []string{query}) || len(results) != 2 ||
		results[0].RequirementQuote != "first exact requirement" ||
		results[0].Query != query ||
		results[0].Pack.ValidateForRequest(
			repositoryretrieval.OperationSemanticExcerpts, query,
		) != nil ||
		results[1].RequirementQuote != "second exact requirement" ||
		results[1].Query != query ||
		results[1].Pack.ValidateForRequest(
			repositoryretrieval.OperationSemanticExcerpts, query,
		) != nil {
		t.Fatalf("results=%#v queries=%v", results, queries)
	}
}

func TestRepositoryEvidenceClosureFailsWithoutInventingAnotherQuery(t *testing.T) {
	t.Parallel()
	buildCalls := 0
	_, err := acquireExistingRepositoryEvidence(
		[]string{"exact requirement"},
		"Change the existing behavior.",
		func(string) (repositoryretrieval.EvidencePack, error) {
			buildCalls++
			return repositoryretrieval.EvidencePack{}, errors.New("PostgreSQL unavailable")
		},
	)
	if err == nil || !strings.Contains(err.Error(), "PostgreSQL unavailable") || buildCalls != 1 {
		t.Fatalf("error=%v build calls=%d", err, buildCalls)
	}
}

func TestRepositoryEvidenceClosureRejectsInvalidAuthorityBeforeAcquisition(t *testing.T) {
	t.Parallel()
	buildCalls := 0
	_, err := acquireExistingRepositoryEvidence(
		[]string{"exact requirement"},
		" invalid authority ",
		func(string) (repositoryretrieval.EvidencePack, error) {
			buildCalls++
			return repositoryretrieval.EvidencePack{}, nil
		},
	)
	if err == nil || buildCalls != 0 {
		t.Fatalf("invalid authority error=%v build calls=%d", err, buildCalls)
	}
}
