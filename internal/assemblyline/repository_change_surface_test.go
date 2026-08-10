package assemblyline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestRepositoryChangeSurfaceIsEvidenceLinkedAndPathBlind(t *testing.T) {
	t.Parallel()
	symbolID := "symbol_" + strings.Repeat("1", 64)
	binding, err := repositoryretrieval.NewQueryBinding(
		repositoryretrieval.OperationSemanticExcerpts,
		"example.test/platform/monorepo/services/dispatch.Dispatch",
	)
	if err != nil {
		t.Fatal(err)
	}
	pack := repositoryretrieval.EvidencePack{
		Schema:     repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID: "snapshot_" + strings.Repeat("2", 64),
		AnalysisID: "analysis_" + strings.Repeat("3", 64),
		Operation:  repositoryretrieval.OperationSemanticExcerpts, QueryBinding: binding,
		Symbols: []repositoryretrieval.EvidenceSymbol{{
			ID: symbolID, Kind: "function", Name: "Dispatch",
			Signature: "func Dispatch()", SourceSHA256: strings.Repeat("4", 64), Source: "func Dispatch() {}",
		}},
		Relations: []repositoryretrieval.EvidenceRelation{}, SourceOmissions: []repositoryretrieval.SourceOmission{},
		OmittedSymbolIDs: []string{}, MaxBytes: 12 * 1024,
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	input := RepositoryChangeSurfaceInput{
		ResearchNeed:      "Make the dispatch window configurable.",
		RequirementQuotes: []string{"dispatch window configurable"}, Evidence: pack,
	}
	job, err := NewRepositoryChangeSurfaceJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, symbolID) {
		t.Fatalf("change-surface prompt leaked or omitted authority:\n%s", prompt)
	}
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	packJSON, err := json.Marshal(pack)
	if err != nil {
		t.Fatal(err)
	}
	for label, projection := range map[string]string{
		"evidence pack":     string(packJSON),
		"portable payload":  string(job.Payload),
		"model prompt":      prompt,
		"response schema":   string(schemaJSON),
		"portable identity": job.ID,
	} {
		for _, forbidden := range []string{
			"example.test/platform/monorepo/services/dispatch",
			"internal/dispatch/dispatch.go",
			"dispatch.go",
			`"qualified_name"`,
			`"file_id"`,
			`"query"`,
		} {
			if strings.Contains(projection, forbidden) {
				t.Fatalf("%s leaked repository identity %q: %s", label, forbidden, projection)
			}
		}
	}
	for name, payload := range map[string]string{
		"qualified name": strings.Replace(
			string(job.Payload), `"name":"Dispatch"`,
			`"name":"Dispatch","qualified_name":"example.test/platform/monorepo/services/dispatch.Dispatch"`, 1,
		),
		"file identity": strings.Replace(
			string(job.Payload), `"name":"Dispatch"`,
			`"name":"Dispatch","file_id":"internal/dispatch/dispatch.go"`, 1,
		),
		"plaintext query": strings.Replace(
			string(job.Payload), `"query_binding":`,
			`"query":"example.test/platform/monorepo/services/dispatch.Dispatch","query_binding":`, 1,
		),
	} {
		forged := job
		forged.Payload = json.RawMessage(payload)
		forged.ID = portableJobDigest(forged.Schema, forged.Kind, forged.Payload)
		if err := forged.Validate(); err == nil {
			t.Fatalf("portable change-surface envelope accepted legacy %s material", name)
		}
	}
	targetItems := schema["properties"].(map[string]any)["targets"].(map[string]any)["items"].(map[string]any)
	properties := targetItems["properties"].(map[string]any)
	if got := properties["symbol_id"].(map[string]any)["enum"].([]string); len(got) != 1 || got[0] != symbolID {
		t.Fatalf("target enum=%v", got)
	}
	decision := RepositoryChangeSurfaceDecision{
		Schema:                      RepositoryChangeSurfaceSchemaV1,
		Targets:                     []RepositoryChangeTarget{{SymbolID: symbolID, RequirementQuote: "dispatch window configurable"}},
		UnresolvedRequirementQuotes: []string{},
	}
	if err := decision.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	invalid := decision
	invalid.Targets[0].SymbolID = "symbol_" + strings.Repeat("5", 64)
	if err := invalid.ValidateFor(input); err == nil || !strings.Contains(err.Error(), "evidence") {
		t.Fatalf("unknown symbol error=%v", err)
	}
	for _, typ := range []reflect.Type{reflect.TypeOf(input), reflect.TypeOf(decision), reflect.TypeOf(RepositoryChangeTarget{})} {
		for _, forbidden := range []string{"Path", "File", "Command", "Patch", "Test", "Plan", "Workspace"} {
			if _, exists := typ.FieldByName(forbidden); exists {
				t.Fatalf("%s exposes forbidden field %q", typ.Name(), forbidden)
			}
		}
	}
}

func TestRepositoryChangeSurfaceCannotOmitCodeOwnedRequirements(t *testing.T) {
	t.Parallel()
	symbolID := "symbol_" + strings.Repeat("1", 64)
	binding, err := repositoryretrieval.NewQueryBinding(repositoryretrieval.OperationSemanticExcerpts, "dispatch window")
	if err != nil {
		t.Fatal(err)
	}
	pack := repositoryretrieval.EvidencePack{
		Schema:     repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID: "snapshot_" + strings.Repeat("2", 64),
		AnalysisID: "analysis_" + strings.Repeat("3", 64),
		Operation:  repositoryretrieval.OperationSemanticExcerpts, QueryBinding: binding,
		Symbols: []repositoryretrieval.EvidenceSymbol{{
			ID: symbolID, Kind: "function", Name: "Dispatch",
			Signature: "func Dispatch()", SourceSHA256: strings.Repeat("4", 64), Source: "func Dispatch() {}",
		}},
		Relations: []repositoryretrieval.EvidenceRelation{}, SourceOmissions: []repositoryretrieval.SourceOmission{},
		OmittedSymbolIDs: []string{}, MaxBytes: 12 * 1024,
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	input := RepositoryChangeSurfaceInput{
		ResearchNeed:      "Make dispatch configurable and preserve retry timing.",
		RequirementQuotes: []string{"dispatch configurable", "preserve retry timing"},
		Evidence:          pack,
	}
	omitted := RepositoryChangeSurfaceDecision{
		Schema:                      RepositoryChangeSurfaceSchemaV1,
		Targets:                     []RepositoryChangeTarget{{SymbolID: symbolID, RequirementQuote: "dispatch configurable"}},
		UnresolvedRequirementQuotes: []string{},
	}
	if err := omitted.ValidateFor(input); err == nil || !strings.Contains(err.Error(), "omitted") {
		t.Fatalf("omitted requirement error=%v", err)
	}
	omitted.UnresolvedRequirementQuotes = []string{"preserve retry timing"}
	if err := omitted.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
}
