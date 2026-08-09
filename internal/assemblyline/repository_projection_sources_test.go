package assemblyline

import (
	"strings"
	"testing"

	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestResolveRepositoryProjectionSourcesUsesExactValidatedPortablePayload(t *testing.T) {
	t.Parallel()
	retrievalJob, err := NewRepositoryRetrievalJob(RepositoryRetrievalInput{ResearchNeed: "Find the exact owner."})
	if err != nil {
		t.Fatal(err)
	}
	retrieval, err := ResolveRepositoryProjectionSources(retrievalJob)
	if err != nil {
		t.Fatal(err)
	}
	if retrieval.ResearchNeed != "Find the exact owner." || retrieval.Evidence != nil {
		t.Fatalf("retrieval sources=%+v", retrieval)
	}

	pack := repositoryProjectionTestPack(t)
	changeJob, err := NewRepositoryChangeSurfaceJob(RepositoryChangeSurfaceInput{
		ResearchNeed: "Change the exact owner.", RequirementQuotes: []string{"exact owner"}, Evidence: pack,
	})
	if err != nil {
		t.Fatal(err)
	}
	change, err := ResolveRepositoryProjectionSources(changeJob)
	if err != nil {
		t.Fatal(err)
	}
	if change.ResearchNeed != "Change the exact owner." || change.Evidence == nil || change.Evidence.ID != pack.ID {
		t.Fatalf("change sources=%+v", change)
	}
}

func TestResolveRepositoryProjectionSourcesRejectsOtherOrForgedWork(t *testing.T) {
	t.Parallel()
	other, err := NewApplicationClassificationJob(ApplicationClassificationInput{UserRequest: "Build a tool."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveRepositoryProjectionSources(other); err == nil {
		t.Fatal("non-repository work received projection sources")
	}
	retrieval, err := NewRepositoryRetrievalJob(RepositoryRetrievalInput{ResearchNeed: "Find one owner."})
	if err != nil {
		t.Fatal(err)
	}
	retrieval.ID = strings.Repeat("f", 64)
	if _, err := ResolveRepositoryProjectionSources(retrieval); err == nil {
		t.Fatal("forged portable identity received projection sources")
	}
}

func repositoryProjectionTestPack(t *testing.T) repositoryretrieval.EvidencePack {
	t.Helper()
	binding, err := repositoryretrieval.NewQueryBinding(repositoryretrieval.OperationSemanticExcerpts, "owner")
	if err != nil {
		t.Fatal(err)
	}
	pack := repositoryretrieval.EvidencePack{
		Schema:     repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID: "snapshot_" + strings.Repeat("1", 64),
		AnalysisID: "analysis_" + strings.Repeat("2", 64),
		Operation:  repositoryretrieval.OperationSemanticExcerpts, QueryBinding: binding,
		Symbols: []repositoryretrieval.EvidenceSymbol{{
			ID: "symbol_" + strings.Repeat("3", 64), Kind: "function", Name: "Owner",
			Signature: "func Owner()", SourceSHA256: strings.Repeat("4", 64),
		}},
		Relations: []repositoryretrieval.EvidenceRelation{}, SourceOmissions: []repositoryretrieval.SourceOmission{},
		OmittedSymbolIDs: []string{}, MaxBytes: 9 * 1024,
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	return pack
}
