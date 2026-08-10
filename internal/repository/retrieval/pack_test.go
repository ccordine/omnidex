package retrieval

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	repositoryfacts "github.com/gryph/omnidex/internal/repository"
	golangadapter "github.com/gryph/omnidex/internal/repository/adapters/golang"
)

func TestBuildEvidencePackUsesOpaqueBoundedHashCheckedFacts(t *testing.T) {
	snapshot, analysis := retrievalFixture(t)
	store := &fixtureStore{snapshot: snapshot, analysis: analysis}
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	pack, err := service.Build(context.Background(), Request{
		ProjectID: 11, AnalysisID: analysis.ID,
		Operation: OperationSemanticExcerpts, Query: "example.test/platform/services/sample.Value",
		Limits: Limits{MaxSymbols: 4, MaxEdges: 12, MaxSpanBytes: 4096, MaxPackBytes: 12 * 1024},
	})
	if err != nil {
		t.Fatal(err)
	}
	if pack.ID == "" || pack.Operation != OperationSemanticExcerpts || pack.SnapshotID != snapshot.ID || pack.AnalysisID != analysis.ID || len(pack.Symbols) == 0 {
		t.Fatalf("pack=%#v", pack)
	}
	raw, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	for _, forbidden := range []string{
		snapshot.Root,
		"sample.go",
		`"path"`,
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("model evidence leaked repository identity %q: %s", forbidden, text)
		}
	}
	if !strings.Contains(text, "func Value") || len(raw) > pack.MaxBytes {
		t.Fatalf("bounded pack bytes=%d max=%d content=%s", len(raw), pack.MaxBytes, text)
	}
}

func TestBuildEvidencePackFailsOnStaleSourceAndMissingSeeds(t *testing.T) {
	snapshot, analysis := retrievalFixture(t)
	service, err := New(&fixtureStore{snapshot: snapshot, analysis: analysis})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Build(context.Background(), Request{
		ProjectID: 11, AnalysisID: analysis.ID,
		Operation: OperationSemanticExcerpts,
		Limits:    Limits{MaxSymbols: 4, MaxEdges: 12, MaxSpanBytes: 4096, MaxPackBytes: 12 * 1024},
	}); err == nil || !strings.Contains(err.Error(), "query") {
		t.Fatalf("missing query error=%v", err)
	}
	if err := os.WriteFile(filepath.Join(snapshot.Root, "sample.go"), []byte("package sample\n\nfunc Value() int { return 2 }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Build(context.Background(), Request{
		ProjectID: 11, AnalysisID: analysis.ID,
		Operation: OperationSemanticExcerpts, Query: "example.test/platform/services/sample.Value",
		Limits: Limits{MaxSymbols: 4, MaxEdges: 12, MaxSpanBytes: 4096, MaxPackBytes: 12 * 1024},
	}); err == nil || !strings.Contains(err.Error(), "stale") {
		t.Fatalf("stale evidence error=%v", err)
	}
}

func TestBuildOperationsProduceDistinctBoundedEvidence(t *testing.T) {
	snapshot, analysis := retrievalFixture(t)
	store := &fixtureStore{snapshot: snapshot, analysis: analysis}
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	limits := Limits{MaxSymbols: 4, MaxEdges: 12, MaxSpanBytes: 4096, MaxPackBytes: 12 * 1024}

	semantic, err := service.Build(context.Background(), Request{
		ProjectID: 11, AnalysisID: analysis.ID,
		Operation: OperationSemanticExcerpts, Query: "example.test/platform/services/sample.Value", Limits: limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	declaration, err := service.Build(context.Background(), Request{
		ProjectID: 11, AnalysisID: analysis.ID,
		Operation: OperationSymbolDeclaration, Query: "example.test/platform/services/sample.Value", Limits: limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	references, err := service.Build(context.Background(), Request{
		ProjectID: 11, AnalysisID: analysis.ID,
		Operation: OperationDirectReferences, Query: "Helper", Limits: limits,
	})
	if err != nil {
		t.Fatal(err)
	}
	if semantic.ID == declaration.ID || declaration.ID == references.ID || semantic.ID == references.ID {
		t.Fatalf("operation was absent from evidence identity: semantic=%s declaration=%s references=%s", semantic.ID, declaration.ID, references.ID)
	}
	if declaration.SubjectSymbolID == "" || len(declaration.Symbols) != 1 || len(declaration.Relations) != 0 {
		t.Fatalf("declaration pack=%#v", declaration)
	}
	if references.SubjectSymbolID == "" || len(references.Symbols) != 2 || len(references.Relations) != 1 {
		t.Fatalf("references pack=%#v", references)
	}
	relation := references.Relations[0]
	if relation.ToID != references.SubjectSymbolID || relation.Kind != "calls" {
		t.Fatalf("direct reference relation=%#v subject=%s", relation, references.SubjectSymbolID)
	}
	if len(semantic.Symbols) < 2 || len(semantic.Relations) == 0 || semantic.SubjectSymbolID != "" {
		t.Fatalf("semantic pack=%#v", semantic)
	}
	if store.graphCalls != 2 {
		t.Fatalf("graph calls=%d want semantic and direct-reference calls only", store.graphCalls)
	}
}

func TestExactSymbolOperationsRejectAmbiguousNames(t *testing.T) {
	snapshot, analysis := retrievalFixture(t)
	service, err := New(&fixtureStore{snapshot: snapshot, analysis: analysis})
	if err != nil {
		t.Fatal(err)
	}
	limits := Limits{MaxSymbols: 4, MaxEdges: 12, MaxSpanBytes: 4096, MaxPackBytes: 12 * 1024}
	for _, operation := range []Operation{OperationSymbolDeclaration, OperationDirectReferences} {
		_, err := service.Build(context.Background(), Request{
			ProjectID: 11, AnalysisID: analysis.ID, Operation: operation, Query: "Value", Limits: limits,
		})
		if err == nil || !strings.Contains(err.Error(), "ambiguous") {
			t.Fatalf("operation=%s ambiguity error=%v", operation, err)
		}
	}
}

func TestGraphBoundaryFailsInsteadOfHidingTruncation(t *testing.T) {
	snapshot, analysis := retrievalFixture(t)
	store := &fixtureStore{snapshot: snapshot, analysis: analysis, repeatGraphEdge: true}
	service, err := New(store)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Build(context.Background(), Request{
		ProjectID: 11, AnalysisID: analysis.ID,
		Operation: OperationDirectReferences, Query: "Helper",
		Limits: Limits{MaxSymbols: 4, MaxEdges: 1, MaxSpanBytes: 4096, MaxPackBytes: 12 * 1024},
	})
	if err == nil || !strings.Contains(err.Error(), "graph") || !strings.Contains(err.Error(), "limit") {
		t.Fatalf("graph boundary error=%v", err)
	}
}

func TestEveryRegisteredOperationHasAProductionConsumer(t *testing.T) {
	snapshot, analysis := retrievalFixture(t)
	service, err := New(&fixtureStore{snapshot: snapshot, analysis: analysis})
	if err != nil {
		t.Fatal(err)
	}
	queries := map[Operation]string{
		OperationSemanticExcerpts:  "example.test/platform/services/sample.Value",
		OperationSymbolDeclaration: "example.test/platform/services/sample.Value",
		OperationDirectReferences:  "Helper",
	}
	operations := SupportedOperations()
	if len(operations) != len(queries) {
		t.Fatalf("registered operations=%v consumers=%v", operations, queries)
	}
	for _, operation := range operations {
		query, exists := queries[operation]
		if !exists {
			t.Fatalf("operation %q has no production consumer test", operation)
		}
		if _, err := service.Build(context.Background(), Request{
			ProjectID: 11, AnalysisID: analysis.ID, Operation: operation, Query: query,
			Limits: Limits{MaxSymbols: 4, MaxEdges: 12, MaxSpanBytes: 4096, MaxPackBytes: 12 * 1024},
		}); err != nil {
			t.Fatalf("operation %q: %v", operation, err)
		}
	}
}

type fixtureStore struct {
	snapshot        repositoryfacts.Snapshot
	analysis        repositoryfacts.Analysis
	graphCalls      int
	repeatGraphEdge bool
}

func (store *fixtureStore) RepositoryAnalysis(context.Context, int64, string) (repositoryfacts.Analysis, error) {
	return store.analysis, nil
}

func (store *fixtureStore) RepositorySnapshot(context.Context, int64, string) (repositoryfacts.Snapshot, error) {
	return store.snapshot, nil
}

func (store *fixtureStore) SearchRepositorySymbols(_ context.Context, _ int64, _ string, query string, _ int) ([]repositoryfacts.SymbolMatch, error) {
	matches := make([]repositoryfacts.SymbolMatch, 0)
	for _, symbol := range store.analysis.Symbols {
		name, qualified, query := strings.ToLower(symbol.Name), strings.ToLower(symbol.QualifiedName), strings.ToLower(query)
		if name == query || qualified == query {
			matches = append(matches, repositoryfacts.SymbolMatch{Symbol: symbol, MatchKind: "exact", Score: 4})
		} else if strings.Contains(qualified, query) {
			matches = append(matches, repositoryfacts.SymbolMatch{Symbol: symbol, MatchKind: "full_text", Score: 1})
		}
	}
	return matches, nil
}

func (store *fixtureStore) RepositoryGraphNeighborhood(_ context.Context, _ int64, analysisID string, subjects []string, limit int) (repositoryfacts.GraphNeighborhood, error) {
	store.graphCalls++
	edges := make([]repositoryfacts.Edge, 0)
	for _, edge := range store.analysis.Edges {
		for _, subject := range subjects {
			if edge.FromID == subject || edge.ToID == subject {
				edges = append(edges, edge)
				break
			}
		}
		if len(edges) == limit {
			break
		}
	}
	if store.repeatGraphEdge && len(edges) > 0 {
		for len(edges) < limit {
			edges = append(edges, edges[0])
		}
	}
	return repositoryfacts.GraphNeighborhood{AnalysisID: analysisID, SubjectIDs: subjects, Edges: edges}, nil
}

func retrievalFixture(t *testing.T) (repositoryfacts.Snapshot, repositoryfacts.Analysis) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is required")
	}
	root := t.TempDir()
	writeRetrievalFile(t, root, "go.mod", "module example.test/platform/services/sample\n\ngo 1.24.1\n")
	writeRetrievalFile(t, root, "sample.go", "package sample\n\nfunc Helper() int { return 1 }\n\nfunc Value() int { return Helper() }\n")
	writeRetrievalFile(t, root, "other/other.go", "package other\n\nfunc Value() int { return 2 }\n")
	runRetrievalGit(t, root, "init")
	runRetrievalGit(t, root, "config", "user.email", "retrieval@example.test")
	runRetrievalGit(t, root, "config", "user.name", "Retrieval Test")
	runRetrievalGit(t, root, "add", ".")
	runRetrievalGit(t, root, "commit", "-m", "fixture")
	snapshot, err := repositoryfacts.BuildGitSnapshot(context.Background(), root, repositoryfacts.SnapshotOptions{})
	if err != nil {
		t.Fatal(err)
	}
	analysis, err := golangadapter.Analyze(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	return snapshot, analysis
}

func writeRetrievalFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runRetrievalGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
}
