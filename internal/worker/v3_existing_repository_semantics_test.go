package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestExistingRepositorySemanticsUseOnlyStableTypedCalls(t *testing.T) {
	symbolID := "symbol_" + strings.Repeat("1", 64)
	binding, err := repositoryretrieval.NewQueryBinding(repositoryretrieval.OperationSemanticExcerpts, "Value")
	if err != nil {
		t.Fatal(err)
	}
	pack := repositoryretrieval.EvidencePack{
		Schema:     repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID: "snapshot_" + strings.Repeat("2", 64), AnalysisID: "analysis_" + strings.Repeat("3", 64),
		Operation: repositoryretrieval.OperationSemanticExcerpts, QueryBinding: binding,
		Symbols: []repositoryretrieval.EvidenceSymbol{{
			ID: symbolID, Kind: "function", Name: "Value",
			Signature: "func Value() int", SourceSHA256: strings.Repeat("4", 64), Source: "func Value() int { return 1 }",
		}},
		Relations: []repositoryretrieval.EvidenceRelation{}, SourceOmissions: []repositoryretrieval.SourceOmission{},
		OmittedSymbolIDs: []string{}, MaxBytes: 12 * 1024,
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	calls := make([]assemblyline.WorkKind, 0, 2)
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 2,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			calls = append(calls, job.Kind)
			var candidate any
			switch job.Kind {
			case assemblyline.WorkRepositorySearchTerm:
				candidate = assemblyline.RepositorySearchTermDecision{
					Schema: assemblyline.RepositorySearchTermSchemaV1, Term: "Value",
				}
			case assemblyline.WorkRepositoryChangeSurface:
				candidate = assemblyline.RepositoryChangeSurfaceDecision{
					Schema:                      assemblyline.RepositoryChangeSurfaceSchemaV1,
					Targets:                     []assemblyline.RepositoryChangeTarget{{SymbolID: symbolID, RequirementQuote: "return two"}},
					UnresolvedRequirementQuotes: []string{},
				}
			default:
				return assemblyline.PortableResult{}, fmt.Errorf("unexpected work kind %s", job.Kind)
			}
			raw, _ := json.Marshal(candidate)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, nil
		},
	}
	need := "Change Value to return two."
	searchTerm, err := generateExistingRepositorySearchTerm(runtime, "qwen", need, nil)
	if err != nil {
		t.Fatal(err)
	}
	if searchTerm.Term != "Value" {
		t.Fatalf("search term=%#v", searchTerm)
	}
	surface, err := selectExistingRepositoryChangeSurface(
		runtime, "qwen", need, []string{"return two"}, pack, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(surface.Targets) != 1 || surface.Targets[0].SymbolID != symbolID {
		t.Fatalf("surface=%#v", surface)
	}
	if len(calls) != 2 || calls[0] != assemblyline.WorkRepositorySearchTerm || calls[1] != assemblyline.WorkRepositoryChangeSurface {
		t.Fatalf("work calls=%v", calls)
	}
}

func TestRepositoryRequirementSurfaceReceivesOnlyItsExactGapProjection(t *testing.T) {
	t.Parallel()
	pack := repositoryProjectionTestPack(t)
	requirementQuote := "Change the exact owner."
	var input assemblyline.RepositoryChangeSurfaceInput
	runtime := typedWorkerRuntime{
		Context: context.Background(), MaxAttempts: 1,
		Execute: func(job assemblyline.PortableJob, _ string) (assemblyline.PortableResult, error) {
			if job.Kind != assemblyline.WorkRepositoryChangeSurface {
				t.Fatalf("work kind=%q", job.Kind)
			}
			if err := json.Unmarshal(job.Payload, &input); err != nil {
				t.Fatal(err)
			}
			candidate := assemblyline.RepositoryChangeSurfaceDecision{
				Schema: assemblyline.RepositoryChangeSurfaceSchemaV1,
				Targets: []assemblyline.RepositoryChangeTarget{{
					SymbolID: pack.Symbols[0].ID, RequirementQuote: requirementQuote,
				}},
				UnresolvedRequirementQuotes: []string{},
			}
			raw, err := json.Marshal(candidate)
			return assemblyline.PortableResult{JobID: job.ID, Candidate: string(raw)}, err
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
	if input.ResearchNeed != requirementQuote ||
		!reflect.DeepEqual(input.RequirementQuotes, []string{requirementQuote}) ||
		len(decision.Targets) != 1 {
		t.Fatalf("surface input=%#v decision=%#v", input, decision)
	}
}

func TestRepositoryEvidenceClosureUsesCodeHeldQueriesBeforeInference(t *testing.T) {
	t.Parallel()
	queries := make([]string, 0, 2)
	stationCalls := 0
	results, err := acquireExistingRepositoryEvidence(
		[]string{"first exact requirement", "second exact requirement"},
		func(query string) (repositoryretrieval.EvidencePack, error) {
			queries = append(queries, query)
			return repositoryAcquisitionTestPack(t, query), nil
		},
		func(string) (assemblyline.RepositorySearchTermDecision, error) {
			stationCalls++
			return assemblyline.RepositorySearchTermDecision{}, fmt.Errorf("station must not run")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(queries, []string{"first exact requirement", "second exact requirement"}) ||
		stationCalls != 0 || len(results) != 2 ||
		results[0].RequirementQuote != "first exact requirement" ||
		results[0].Query != "first exact requirement" || results[0].SearchTermCalls != 0 ||
		results[0].Pack.ValidateForRequest(
			repositoryretrieval.OperationSemanticExcerpts, "first exact requirement",
		) != nil ||
		results[1].RequirementQuote != "second exact requirement" ||
		results[1].Query != "second exact requirement" || results[1].SearchTermCalls != 0 ||
		results[1].Pack.ValidateForRequest(
			repositoryretrieval.OperationSemanticExcerpts, "second exact requirement",
		) != nil {
		t.Fatalf("results=%#v queries=%v station calls=%d", results, queries, stationCalls)
	}
}

func TestRepositoryEvidenceClosureOpensOneSearchTermGapOnlyAfterExhaustion(t *testing.T) {
	t.Parallel()
	queries := make([]string, 0, 3)
	stationCalls := 0
	results, err := acquireExistingRepositoryEvidence(
		[]string{"first exact requirement", "second exact requirement"},
		func(query string) (repositoryretrieval.EvidencePack, error) {
			queries = append(queries, query)
			if query == "first exact requirement" {
				return repositoryAcquisitionTestPack(t, query), nil
			}
			if query == "alternate term" {
				return repositoryAcquisitionTestPack(t, query), nil
			}
			return repositoryretrieval.EvidencePack{}, fmt.Errorf(
				"no match for %q: %w", query, repositoryretrieval.ErrInsufficientEvidence,
			)
		},
		func(requirementQuote string) (assemblyline.RepositorySearchTermDecision, error) {
			stationCalls++
			if requirementQuote != "second exact requirement" {
				return assemblyline.RepositorySearchTermDecision{}, fmt.Errorf(
					"wrong unresolved concept %q", requirementQuote,
				)
			}
			return assemblyline.RepositorySearchTermDecision{
				Schema: assemblyline.RepositorySearchTermSchemaV1, Term: "alternate term",
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(queries, []string{
		"first exact requirement", "second exact requirement", "alternate term",
	}) || stationCalls != 1 || len(results) != 2 ||
		results[0].SearchTermCalls != 0 || results[0].Query != "first exact requirement" ||
		results[1].SearchTermCalls != 1 || results[1].Query != "alternate term" ||
		results[1].Pack.ValidateForRequest(
			repositoryretrieval.OperationSemanticExcerpts, "alternate term",
		) != nil {
		t.Fatalf("results=%#v queries=%v station calls=%d", results, queries, stationCalls)
	}
}

func TestRepositoryEvidenceClosureFailsWithoutInferenceOnMechanicalError(t *testing.T) {
	t.Parallel()
	stationCalls := 0
	results, err := acquireExistingRepositoryEvidence(
		[]string{"exact requirement"},
		func(string) (repositoryretrieval.EvidencePack, error) {
			return repositoryretrieval.EvidencePack{}, errors.New("PostgreSQL unavailable")
		},
		func(string) (assemblyline.RepositorySearchTermDecision, error) {
			stationCalls++
			return assemblyline.RepositorySearchTermDecision{}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "PostgreSQL unavailable") ||
		stationCalls != 0 || len(results) != 1 || results[0].SearchTermCalls != 0 {
		t.Fatalf("results=%#v error=%v station calls=%d", results, err, stationCalls)
	}
}

func TestRepositoryEvidenceClosureHasNoGapRetryOrInvalidQueryFallback(t *testing.T) {
	t.Parallel()
	buildCalls := 0
	stationCalls := 0
	results, err := acquireExistingRepositoryEvidence(
		[]string{"exact requirement"},
		func(string) (repositoryretrieval.EvidencePack, error) {
			buildCalls++
			return repositoryretrieval.EvidencePack{}, repositoryretrieval.ErrInsufficientEvidence
		},
		func(requirementQuote string) (assemblyline.RepositorySearchTermDecision, error) {
			stationCalls++
			if requirementQuote != "exact requirement" {
				t.Fatalf("unresolved concept=%q", requirementQuote)
			}
			return assemblyline.RepositorySearchTermDecision{
				Schema: assemblyline.RepositorySearchTermSchemaV1, Term: "exact requirement",
			}, nil
		},
	)
	if !errors.Is(err, repositoryretrieval.ErrInsufficientEvidence) ||
		buildCalls != 2 || stationCalls != 1 || len(results) != 1 || results[0].SearchTermCalls != 1 {
		t.Fatalf("results=%#v error=%v build calls=%d station calls=%d", results, err, buildCalls, stationCalls)
	}

	buildCalls = 0
	_, err = acquireExistingRepositoryEvidence(
		[]string{"valid term", " invalid term "},
		func(string) (repositoryretrieval.EvidencePack, error) {
			buildCalls++
			return repositoryretrieval.EvidencePack{}, nil
		},
		nil,
	)
	if err == nil || buildCalls != 0 {
		t.Fatalf("invalid query preflight error=%v build calls=%d", err, buildCalls)
	}
}
