package worker

import (
	"strings"
	"testing"

	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestRepositoryEvidenceCapsulesBindExactPackAndSymbolProvenance(t *testing.T) {
	t.Parallel()

	capsules, err := repositoryEvidenceCapsules(repositoryretrieval.EvidencePack{
		ID: "pack-17",
		Symbols: []repositoryretrieval.EvidenceSymbol{{
			ID: "symbol-31", Kind: "function", Name: "ReadThing",
			Signature: "func ReadThing() string", Source: "func ReadThing() string { return value }",
			SourceSHA256: strings.Repeat("a", 64),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(capsules) != 1 || capsules[0].Capsule.ID != "R01" {
		t.Fatalf("capsules=%#v", capsules)
	}
	if capsules[0].SourceType != "repository_symbol" || capsules[0].SourceRef != "pack-17#symbol-31" {
		t.Fatalf("provenance=%#v", capsules[0])
	}
	if capsules[0].SHA256 == "" || !strings.Contains(capsules[0].Capsule.Text, "func ReadThing() string") {
		t.Fatalf("exact capsule was not retained: %#v", capsules[0])
	}
}

func TestRepositoryEvidenceCapsulesPreserveRelationAsFirstClassEvidence(t *testing.T) {
	t.Parallel()

	pack := repositoryretrieval.EvidencePack{
		ID: "pack-17",
		Symbols: []repositoryretrieval.EvidenceSymbol{
			{ID: "symbol-1", Kind: "function", Name: "Caller", Signature: "func Caller()", Source: "func Caller() { Callee() }", SourceSHA256: strings.Repeat("a", 64)},
			{ID: "symbol-2", Kind: "function", Name: "Callee", Signature: "func Callee()", Source: "func Callee() {}", SourceSHA256: strings.Repeat("b", 64)},
		},
		Relations: []repositoryretrieval.EvidenceRelation{{
			ID: "relation-1", FromID: "symbol-1", ToID: "symbol-2", Kind: "calls",
			Origin: "go_types", Confidence: 1,
		}},
		SourceOmissions: []repositoryretrieval.SourceOmission{}, OmittedSymbolIDs: []string{},
	}
	capsules, err := repositoryEvidenceCapsules(pack)
	if err != nil {
		t.Fatal(err)
	}
	if len(capsules) != 3 || capsules[2].Capsule.ID != "R03" ||
		capsules[2].SourceType != "repository_relation" ||
		capsules[2].SourceRef != "pack-17#relation-1" ||
		!strings.Contains(capsules[2].Capsule.Text, "Caller calls Callee") ||
		capsules[2].SHA256 == "" {
		t.Fatalf("relation capsule=%#v all=%#v", capsules[2], capsules)
	}
}

func TestRepositoryEvidenceCapsulesRejectIncompleteRetrievalProjection(t *testing.T) {
	t.Parallel()
	base := repositoryretrieval.EvidencePack{
		ID: "pack", Symbols: []repositoryretrieval.EvidenceSymbol{{
			ID: "symbol", Kind: "function", Name: "Owner", Source: "bounded",
			SourceSHA256: strings.Repeat("a", 64),
		}}, Relations: []repositoryretrieval.EvidenceRelation{},
		SourceOmissions: []repositoryretrieval.SourceOmission{}, OmittedSymbolIDs: []string{},
	}
	for name, mutate := range map[string]func(*repositoryretrieval.EvidencePack){
		"source omission": func(pack *repositoryretrieval.EvidencePack) {
			pack.SourceOmissions = []repositoryretrieval.SourceOmission{{SymbolID: "symbol", Reason: "source_span_exceeds_limit"}}
		},
		"symbol omission": func(pack *repositoryretrieval.EvidencePack) {
			pack.OmittedSymbolIDs = []string{"omitted"}
		},
		"edge omission": func(pack *repositoryretrieval.EvidencePack) { pack.OmittedEdges = 1 },
	} {
		t.Run(name, func(t *testing.T) {
			pack := base
			mutate(&pack)
			if _, err := repositoryEvidenceCapsules(pack); err == nil {
				t.Fatalf("incomplete %s retrieval projection was silently accepted", name)
			}
		})
	}
}

func TestRepositoryEvidenceCapsulesRejectBeforeUnboundedProjection(t *testing.T) {
	t.Parallel()

	tooMany := make([]repositoryretrieval.EvidenceSymbol, maxObjectiveRepositoryEvidenceCapsules+1)
	for index := range tooMany {
		tooMany[index] = repositoryretrieval.EvidenceSymbol{
			ID: string(rune('a' + index)), Source: "bounded", SourceSHA256: strings.Repeat("a", 64),
		}
	}
	if _, err := repositoryEvidenceCapsules(repositoryretrieval.EvidencePack{ID: "pack", Symbols: tooMany}); err == nil {
		t.Fatal("unbounded symbol collection was copied and projected")
	}
	oversized := repositoryretrieval.EvidencePack{ID: "pack", Symbols: []repositoryretrieval.EvidenceSymbol{{
		ID: "symbol", Source: strings.Repeat("x", maxObjectiveRepositoryEvidenceTextBytes+1),
		SourceSHA256: strings.Repeat("a", 64),
	}}}
	if _, err := repositoryEvidenceCapsules(oversized); err == nil {
		t.Fatal("oversized symbol source was joined into a model-visible capsule")
	}
	oversizedPackID := repositoryretrieval.EvidencePack{
		ID: strings.Repeat("p", maxObjectiveEvidenceSourceRefBytes+1),
		Symbols: []repositoryretrieval.EvidenceSymbol{{
			ID: "symbol", Source: "bounded", SourceSHA256: strings.Repeat("a", 64),
		}},
	}
	if _, err := repositoryEvidenceCapsules(oversizedPackID); err == nil {
		t.Fatal("oversized provenance identity was concatenated before rejection")
	}
}
