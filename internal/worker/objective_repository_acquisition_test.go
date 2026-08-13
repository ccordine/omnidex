package worker

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

type objectiveRepositoryEvidenceBuilderFunc func(
	context.Context,
	repositoryretrieval.Request,
) (repositoryretrieval.EvidencePack, error)

func (build objectiveRepositoryEvidenceBuilderFunc) Build(
	ctx context.Context,
	request repositoryretrieval.Request,
) (repositoryretrieval.EvidencePack, error) {
	return build(ctx, request)
}

func TestBuildObjectiveRepositoryEvidenceKeepsOneMatchingAnalysisUnchanged(t *testing.T) {
	t.Parallel()
	query := "dispatch owner"
	want := objectiveRepositoryAnalysisPack(t, "analysis-b", query, "OwnerB", "source b")
	pack, err := buildObjectiveRepositoryEvidence(
		context.Background(),
		objectiveRepositoryEvidenceBuilderFunc(func(
			_ context.Context,
			request repositoryretrieval.Request,
		) (repositoryretrieval.EvidencePack, error) {
			if request.AnalysisID == "analysis-a" {
				return repositoryretrieval.EvidencePack{}, repositoryretrieval.ErrInsufficientEvidence
			}
			return want, nil
		}),
		17,
		[]repositoryfacts.Analysis{{ID: "analysis-b"}, {ID: "analysis-a"}},
		query,
	)
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID != want.ID || pack.AnalysisID != want.AnalysisID || len(pack.Symbols) != 1 {
		t.Fatalf("single matching analysis was rewritten: got=%#v want=%#v", pack, want)
	}
}

func TestObjectiveRepositoryEvidenceMergesPolyglotMatchesBeforeRelevance(t *testing.T) {
	t.Parallel()
	query := "dispatch owner"
	build := objectiveRepositoryEvidenceBuilderFunc(func(
		_ context.Context,
		request repositoryretrieval.Request,
	) (repositoryretrieval.EvidencePack, error) {
		switch request.AnalysisID {
		case "analysis-a":
			return objectiveRepositoryAnalysisPack(t, request.AnalysisID, query, "OwnerA", "source a"), nil
		case "analysis-b":
			return objectiveRepositoryAnalysisPack(t, request.AnalysisID, query, "OwnerB", "source b"), nil
		default:
			return repositoryretrieval.EvidencePack{}, errors.New("unexpected analysis")
		}
	})
	result, err := acquireObjectiveRepositoryEvidenceClosure(
		query,
		func(searchTerm string) (repositoryretrieval.EvidencePack, error) {
			return buildObjectiveRepositoryEvidence(
				context.Background(), build, 17,
				[]repositoryfacts.Analysis{{ID: "analysis-b"}, {ID: "analysis-a"}},
				searchTerm,
			)
		},
		func(string) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
			return assemblyline.RepositorySearchTermDecision{}, objectiveStationReceipt{}, errors.New("must not expand")
		},
		func(
			_ string,
			evidence []objectiveEvidence,
		) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
			if len(evidence) != 2 || !strings.Contains(evidence[0].Capsule.Text, "OwnerA") ||
				!strings.Contains(evidence[1].Capsule.Text, "OwnerB") {
				t.Fatalf("polyglot evidence order=%#v", evidence)
			}
			return selectedRepositoryEvidence(evidence[1].Capsule.ID), objectiveStationReceipt{Calls: 1}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Evidence) != 1 || !strings.Contains(result.Evidence[0].Capsule.Text, "OwnerB") {
		t.Fatalf("cross-analysis relevance selection=%#v", result)
	}
}

func TestObjectiveRepositoryEvidenceSelectsRelationFromSecondPolyglotPack(t *testing.T) {
	t.Parallel()
	query := "how caller reaches callee"
	build := objectiveRepositoryEvidenceBuilderFunc(func(
		_ context.Context,
		request repositoryretrieval.Request,
	) (repositoryretrieval.EvidencePack, error) {
		pack := objectiveRepositoryAnalysisPack(
			t, request.AnalysisID, query, "Owner"+request.AnalysisID[len(request.AnalysisID)-1:], "source",
		)
		if request.AnalysisID == "analysis-b" {
			pack = objectiveRepositoryRelationPack(t, request.AnalysisID, query)
		}
		return pack, nil
	})
	result, err := acquireObjectiveRepositoryEvidenceClosure(
		query,
		func(searchTerm string) (repositoryretrieval.EvidencePack, error) {
			return buildObjectiveRepositoryEvidence(
				context.Background(), build, 17,
				[]repositoryfacts.Analysis{{ID: "analysis-b"}, {ID: "analysis-a"}}, searchTerm,
			)
		},
		func(string) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
			return assemblyline.RepositorySearchTermDecision{}, objectiveStationReceipt{}, errors.New("must not expand")
		},
		func(
			_ string,
			evidence []objectiveEvidence,
		) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
			for _, item := range evidence {
				if item.SourceType == "repository_relation" && strings.Contains(item.Capsule.Text, "Caller calls Callee") {
					return selectedRepositoryEvidence(item.Capsule.ID), objectiveStationReceipt{Calls: 1}, nil
				}
			}
			t.Fatalf("second-pack relation was dropped: %#v", evidence)
			return assemblyline.RepositoryEvidenceRelevanceDecision{}, objectiveStationReceipt{}, errors.New("unreachable")
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].SourceType != "repository_relation" {
		t.Fatalf("selected relation=%#v", result)
	}
}

func TestObjectiveRepositoryEvidenceSelectsRelationFromSinglePack(t *testing.T) {
	t.Parallel()
	query := "how caller reaches callee"
	result, err := acquireObjectiveRepositoryEvidenceClosure(
		query,
		func(searchTerm string) (repositoryretrieval.EvidencePack, error) {
			return objectiveRepositoryRelationPack(t, "analysis-a", searchTerm), nil
		},
		func(string) (assemblyline.RepositorySearchTermDecision, objectiveStationReceipt, error) {
			return assemblyline.RepositorySearchTermDecision{}, objectiveStationReceipt{}, errors.New("must not expand")
		},
		func(
			_ string,
			evidence []objectiveEvidence,
		) (assemblyline.RepositoryEvidenceRelevanceDecision, objectiveStationReceipt, error) {
			if len(evidence) != 3 || evidence[2].SourceType != "repository_relation" {
				t.Fatalf("single-pack relation projection=%#v", evidence)
			}
			return selectedRepositoryEvidence(evidence[2].Capsule.ID), objectiveStationReceipt{Calls: 1}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Evidence) != 1 || result.Evidence[0].SourceType != "repository_relation" {
		t.Fatalf("single-pack selected relation=%#v", result)
	}
}

func TestPolyglotRepositoryEvidenceDeduplicatesExactRelationFacts(t *testing.T) {
	t.Parallel()
	query := "how caller reaches callee"
	first := objectiveRepositoryRelationPack(t, "analysis-a", query)
	second := objectiveRepositoryRelationPack(t, "analysis-b", query)
	merged, err := mergeObjectiveRepositoryEvidencePacks(
		[]repositoryretrieval.EvidencePack{second, first}, query,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(merged.Symbols) != 2 || len(merged.Relations) != 1 {
		t.Fatalf("merged exact relation facts were not deduplicated: %#v", merged)
	}
}

func objectiveRepositoryAnalysisPack(
	t *testing.T,
	analysisID string,
	query string,
	name string,
	source string,
) repositoryretrieval.EvidencePack {
	t.Helper()
	binding, err := repositoryretrieval.NewQueryBinding(repositoryretrieval.OperationSemanticExcerpts, query)
	if err != nil {
		t.Fatal(err)
	}
	pack := repositoryretrieval.EvidencePack{
		Schema:     repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID: "snapshot_" + strings.Repeat("1", 64), AnalysisID: analysisID,
		Operation: repositoryretrieval.OperationSemanticExcerpts, QueryBinding: binding,
		Symbols: []repositoryretrieval.EvidenceSymbol{{
			ID: "shared-symbol-id", Kind: "function", Name: name, Signature: "func " + name + "()",
			SourceSHA256: strings.Repeat(strings.ToLower(string(name[len(name)-1])), 64), Source: source,
		}},
		Relations: []repositoryretrieval.EvidenceRelation{}, SourceOmissions: []repositoryretrieval.SourceOmission{},
		OmittedSymbolIDs: []string{}, MaxBytes: 9 * 1024,
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	return pack
}

func objectiveRepositoryRelationPack(
	t *testing.T,
	analysisID string,
	query string,
) repositoryretrieval.EvidencePack {
	t.Helper()
	binding, err := repositoryretrieval.NewQueryBinding(repositoryretrieval.OperationSemanticExcerpts, query)
	if err != nil {
		t.Fatal(err)
	}
	pack := repositoryretrieval.EvidencePack{
		Schema:     repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID: "snapshot_" + strings.Repeat("1", 64), AnalysisID: analysisID,
		Operation: repositoryretrieval.OperationSemanticExcerpts, QueryBinding: binding,
		Symbols: []repositoryretrieval.EvidenceSymbol{
			{ID: "caller", Kind: "function", Name: "Caller", Signature: "func Caller()", SourceSHA256: strings.Repeat("c", 64), Source: "func Caller() { Callee() }"},
			{ID: "callee", Kind: "function", Name: "Callee", Signature: "func Callee()", SourceSHA256: strings.Repeat("d", 64), Source: "func Callee() {}"},
		},
		Relations: []repositoryretrieval.EvidenceRelation{{
			ID: "caller-calls-callee", FromID: "caller", ToID: "callee", Kind: "calls",
			Origin: "go_types", Confidence: 1,
		}},
		SourceOmissions: []repositoryretrieval.SourceOmission{}, OmittedSymbolIDs: []string{}, MaxBytes: 9 * 1024,
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	return pack
}
