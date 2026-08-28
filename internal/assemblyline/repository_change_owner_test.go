package assemblyline

import (
	"strings"
	"testing"

	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestRepositoryChangeOwnerProjectsPathBlindEvidenceAndReturnsOneOpaqueLeaf(t *testing.T) {
	t.Parallel()
	input := repositoryChangeOwnerTestInput(t)
	prompt, err := BuildRepositoryChangeOwnerPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		`example.test/private/secret`, "internal/private.go", "return 1",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("repository owner prompt exposed source material %q:\n%s", forbidden, prompt)
		}
	}
	symbolID := input.Authority.Evidence.Symbols[0].ID
	owner, err := DecodeRepositoryChangeOwnerLeaf(input, symbolID)
	if err != nil || owner != symbolID {
		t.Fatalf("owner=%q err=%v", owner, err)
	}
	for _, structured := range []string{
		`{"symbol_id":"` + symbolID + `"}`,
		`"` + symbolID + `"`,
	} {
		if _, err := DecodeRepositoryChangeOwnerLeaf(input, structured); err == nil {
			t.Fatalf("structured owner candidate %q was accepted", structured)
		}
	}
}

func TestRepositoryChangeOwnerCannotSelectSourceOmission(t *testing.T) {
	t.Parallel()
	input := repositoryChangeOwnerTestInput(t)
	symbolID := input.Authority.Evidence.Symbols[0].ID
	input.Authority.Evidence.SourceOmissions = []repositoryretrieval.SourceOmission{{
		SymbolID: symbolID, Reason: "source_span_exceeds_limit",
	}}
	input.Authority.Evidence.Symbols[0].Source = ""
	if err := repositoryretrieval.FinalizeEvidencePack(&input.Authority.Evidence); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeRepositoryChangeOwnerLeaf(input, symbolID); err == nil {
		t.Fatal("source-omitted repository symbol was selectable")
	}
	if owner, err := DecodeRepositoryChangeOwnerLeaf(
		input, RepositoryChangeOwnerNone,
	); err != nil || owner != RepositoryChangeOwnerNone {
		t.Fatalf("owner=%q err=%v", owner, err)
	}
}

func repositoryChangeOwnerTestInput(t *testing.T) RepositoryChangeOwnerInput {
	t.Helper()
	binding, err := repositoryretrieval.NewQueryBinding(
		repositoryretrieval.OperationSemanticExcerpts, "Value",
	)
	if err != nil {
		t.Fatal(err)
	}
	symbolID := "symbol_" + strings.Repeat("1", 64)
	pack := repositoryretrieval.EvidencePack{
		Schema:       repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID:   "snapshot_" + strings.Repeat("2", 64),
		AnalysisID:   "analysis_" + strings.Repeat("3", 64),
		Operation:    repositoryretrieval.OperationSemanticExcerpts,
		QueryBinding: binding,
		Symbols: []repositoryretrieval.EvidenceSymbol{{
			ID: symbolID, Kind: "function", Name: "Value",
			Signature: "func Value() int", SourceSHA256: strings.Repeat("4", 64),
			Source: "// internal/private.go\nfunc Value() int { _ = \"example.test/private/secret\"; return 1 }",
		}},
		Relations:        []repositoryretrieval.EvidenceRelation{},
		SourceOmissions:  []repositoryretrieval.SourceOmission{},
		OmittedSymbolIDs: []string{}, MaxBytes: 12 * 1024,
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	requirement := "Change Value to return two."
	return RepositoryChangeOwnerInput{
		Authority: RepositoryChangeSurfaceInput{
			ResearchNeed: requirement, Requirements: []string{requirement}, Evidence: pack,
		},
		FocusedRequirement: requirement,
	}
}
