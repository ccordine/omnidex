package cognitionenv

import (
	"fmt"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func testRequestBuilder(
	t *testing.T,
	analysis repositoryfacts.Analysis,
	snapshot repositoryfacts.Snapshot,
) *recordingBuilder {
	t.Helper()
	return &recordingBuilder{build: func(
		request repositoryretrieval.Request,
	) (repositoryretrieval.EvidencePack, error) {
		return testPackForRequest(t, request, analysis, snapshot)
	}}
}

func testPackForRequest(
	t *testing.T,
	request repositoryretrieval.Request,
	analysis repositoryfacts.Analysis,
	snapshot repositoryfacts.Snapshot,
) (repositoryretrieval.EvidencePack, error) {
	t.Helper()
	binding, err := repositoryretrieval.NewQueryBinding(request.Operation, request.Query)
	if err != nil {
		return repositoryretrieval.EvidencePack{}, err
	}
	target, found := testRequestSymbol(analysis, request.Query)
	if !found {
		return repositoryretrieval.EvidencePack{}, repositoryretrieval.ErrInsufficientEvidence
	}
	pack := repositoryretrieval.EvidencePack{
		Schema:     repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID: snapshot.ID, AnalysisID: analysis.ID,
		Operation: request.Operation, QueryBinding: binding,
		Symbols:          []repositoryretrieval.EvidenceSymbol{testEvidenceSymbol(t, snapshot, target)},
		Relations:        []repositoryretrieval.EvidenceRelation{},
		SourceOmissions:  []repositoryretrieval.SourceOmission{},
		OmittedSymbolIDs: []string{}, MaxBytes: request.Limits.MaxPackBytes,
	}
	if request.Operation == repositoryretrieval.OperationSymbolDeclaration ||
		request.Operation == repositoryretrieval.OperationDirectReferences {
		pack.SubjectSymbolID = target.ID
	}
	if request.Operation == repositoryretrieval.OperationDirectReferences {
		addTestReferenceEvidence(t, &pack, analysis, snapshot, target)
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		return repositoryretrieval.EvidencePack{}, err
	}
	return pack, nil
}

func testRequestSymbol(
	analysis repositoryfacts.Analysis,
	query string,
) (repositoryfacts.Symbol, bool) {
	for _, symbol := range analysis.Symbols {
		if strings.EqualFold(symbol.Name, query) || strings.EqualFold(symbol.QualifiedName, query) {
			return symbol, true
		}
	}
	return repositoryfacts.Symbol{}, false
}

func testEvidenceSymbol(
	t *testing.T,
	snapshot repositoryfacts.Snapshot,
	symbol repositoryfacts.Symbol,
) repositoryretrieval.EvidenceSymbol {
	t.Helper()
	span, err := repositoryfacts.ReadExactSymbolSpan(snapshot, symbol, fixedLimits.MaxSpanBytes)
	if err != nil {
		t.Fatal(err)
	}
	return repositoryretrieval.EvidenceSymbol{
		ID: symbol.ID, Kind: symbol.Kind, Name: symbol.Name, Signature: symbol.Signature,
		SourceSHA256: symbol.SourceSHA256, Source: span.Content,
	}
}

func addTestReferenceEvidence(
	t *testing.T,
	pack *repositoryretrieval.EvidencePack,
	analysis repositoryfacts.Analysis,
	snapshot repositoryfacts.Snapshot,
	target repositoryfacts.Symbol,
) {
	t.Helper()
	symbols := make(map[string]repositoryfacts.Symbol, len(analysis.Symbols))
	for _, symbol := range analysis.Symbols {
		symbols[symbol.ID] = symbol
	}
	for _, edge := range analysis.Edges {
		caller, exists := symbols[edge.FromID]
		if edge.ToID != target.ID || !exists {
			continue
		}
		pack.Symbols = append(pack.Symbols, testEvidenceSymbol(t, snapshot, caller))
		pack.Relations = append(pack.Relations, repositoryretrieval.EvidenceRelation{
			ID: edge.ID, FromID: edge.FromID, ToID: edge.ToID,
			Kind: edge.Kind, Origin: string(edge.Origin), Confidence: edge.Confidence,
		})
		return
	}
	panic(fmt.Sprintf("test analysis has no incoming edge to %s", target.ID))
}
