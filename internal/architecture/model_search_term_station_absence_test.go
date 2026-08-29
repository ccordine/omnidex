package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProductionHasNoModelSelectedContextOrRepositorySearchTerms(t *testing.T) {
	t.Parallel()
	root := filepath.Clean(filepath.Join("..", ".."))
	forbidden := []string{
		"ContextSearchTerms",
		"ContextSearchTermLeafInput",
		"WorkContextSearchTermCoverage",
		"WorkContextSearchTerm",
		"RepositorySearchTerm",
		"RepositorySearchAnchorLeafInput",
		"WorkRepositorySearchAnchorCoverage",
		"WorkRepositorySearchAnchor",
		"SearchAnchors",
		"CodingRepositorySearchTerm",
		"BuildLexicalAnchorQuery",
		"context_search_terms_model",
		"coding_repository_search_term_model",
		"OMNI_CONTEXT_SEARCH_TERMS_MODEL",
		"OMNI_CODING_REPOSITORY_SEARCH_TERM_MODEL",
		`json:"retrieval_concepts"`,
	}
	for _, relative := range []string{"cmd", "internal"} {
		walkProductionSource(t, filepath.Join(root, relative), func(path, source string) {
			for _, token := range forbidden {
				if filepath.Base(path) == "removed.go" && strings.HasPrefix(token, "OMNI_") {
					continue
				}
				if strings.Contains(source, token) {
					t.Errorf("production source %s retains model-selected search authority %q", path, token)
				}
			}
		})
	}
	for _, relative := range []string{"default.env", ".env.example"} {
		raw, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil {
			t.Fatal(err)
		}
		for _, token := range forbidden {
			if strings.Contains(string(raw), token) {
				t.Errorf("configuration template %s retains model-selected search authority %q", relative, token)
			}
		}
	}
}
