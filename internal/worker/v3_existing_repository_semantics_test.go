package worker

import (
	"context"
	"encoding/json"
	"fmt"
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
			case assemblyline.WorkRepositoryRetrieval:
				candidate = assemblyline.RepositoryRetrievalDecision{
					Schema:    assemblyline.RepositoryRetrievalSchemaV2,
					Operation: assemblyline.RetrievalSymbolDeclaration, QueryQuote: "Value",
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
	decision, err := classifyExistingRepositoryRetrieval(runtime, "qwen", need, nil)
	if err != nil {
		t.Fatal(err)
	}
	if decision.QueryQuote != "Value" {
		t.Fatalf("retrieval=%#v", decision)
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
	if len(calls) != 2 || calls[0] != assemblyline.WorkRepositoryRetrieval || calls[1] != assemblyline.WorkRepositoryChangeSurface {
		t.Fatalf("work calls=%v", calls)
	}
}

func TestRecordExistingRepositoryEvidenceRejectsDifferentOpaqueQueryBinding(t *testing.T) {
	t.Parallel()
	binding, err := repositoryretrieval.NewQueryBinding(repositoryretrieval.OperationSemanticExcerpts, "Owner")
	if err != nil {
		t.Fatal(err)
	}
	pack := repositoryretrieval.EvidencePack{
		Schema:       repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID:   "snapshot_" + strings.Repeat("1", 64),
		AnalysisID:   "analysis_" + strings.Repeat("2", 64),
		Operation:    repositoryretrieval.OperationSemanticExcerpts,
		QueryBinding: binding,
		Symbols: []repositoryretrieval.EvidenceSymbol{{
			ID: "symbol_" + strings.Repeat("3", 64), Kind: "function", Name: "Owner",
			Signature: "func Owner()", SourceSHA256: strings.Repeat("4", 64), Source: "func Owner() {}",
		}},
		Relations:        []repositoryretrieval.EvidenceRelation{},
		SourceOmissions:  []repositoryretrieval.SourceOmission{},
		OmittedSymbolIDs: []string{}, MaxBytes: 9 * 1024,
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	decision := assemblyline.RepositoryRetrievalDecision{
		Schema: assemblyline.RepositoryRetrievalSchemaV2, Operation: assemblyline.RetrievalSemanticExcerpts,
		QueryQuote: "DifferentOwner",
	}
	if err := (&directCodingSession{}).recordExistingRepositoryEvidence(decision, pack); err == nil ||
		!strings.Contains(err.Error(), "typed retrieval request") {
		t.Fatalf("opaque query binding mismatch error=%v", err)
	}
}
