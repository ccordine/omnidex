package retrieval

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestEvidenceProjectionKeepsInternalQualifiedIdentityOutOfModelMaterial(t *testing.T) {
	snapshot, analysis := retrievalFixture(t)
	service, err := New(&fixtureStore{snapshot: snapshot, analysis: analysis})
	if err != nil {
		t.Fatal(err)
	}
	qualified := "example.test/platform/services/sample.Value"
	pack, err := service.Build(context.Background(), Request{
		ProjectID: 11, AnalysisID: analysis.ID,
		Operation: OperationSemanticExcerpts, Query: qualified,
		Limits: Limits{MaxSymbols: 4, MaxEdges: 12, MaxSpanBytes: 4096, MaxPackBytes: 12 * 1024},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		snapshot.Root,
		"sample.go",
		"example.test/platform/services/sample",
		`"path"`,
		`"file_id"`,
		`"qualified_name"`,
		`"query"`,
	} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("model evidence leaked repository identity %q: %s", forbidden, raw)
		}
	}
	if err := pack.ValidateForRequest(OperationSemanticExcerpts, qualified); err != nil {
		t.Fatal(err)
	}
	if err := pack.ValidateForRequest(OperationSemanticExcerpts, "example.test/platform/services/sample.Helper"); err == nil {
		t.Fatal("evidence pack accepted a different plaintext retrieval query")
	}
	otherBinding, err := NewQueryBinding(OperationSemanticExcerpts, "example.test/platform/services/sample.Helper")
	if err != nil {
		t.Fatal(err)
	}
	other := pack
	other.QueryBinding = otherBinding
	if err := FinalizeEvidencePack(&other); err != nil {
		t.Fatal(err)
	}
	if other.ID == pack.ID {
		t.Fatal("opaque query binding was absent from the exact evidence-pack identity")
	}
	if _, exposed := reflect.TypeOf(EvidenceSymbol{}).FieldByName("QualifiedName"); exposed {
		t.Fatal("model evidence symbol still exposes the internal qualified name")
	}
	if _, exposed := reflect.TypeOf(EvidencePack{}).FieldByName("Query"); exposed {
		t.Fatal("model evidence pack still exposes the plaintext retrieval query")
	}
	for _, symbol := range analysis.Symbols {
		if strings.HasPrefix(symbol.QualifiedName, "example.test/platform/services/sample.") {
			return
		}
	}
	t.Fatal("fixture did not retain its qualified name in internal repository facts")
}
