package worker

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestRepositoryRequirementClosureAcquiresEveryExactQuoteBeforeSurfaceInference(t *testing.T) {
	t.Parallel()
	events := make([]string, 0, 6)
	searchTermCalls := 0
	resolutions, err := prepareExistingRepositoryRequirementResolutions(
		[]string{"first exact requirement", "second exact requirement"},
		func(query string) (repositoryretrieval.EvidencePack, error) {
			events = append(events, "acquire:"+query)
			return repositoryAcquisitionTestPack(t, query), nil
		},
		func(requirementQuote string) (assemblyline.RepositorySearchTermDecision, error) {
			searchTermCalls++
			return assemblyline.RepositorySearchTermDecision{}, fmt.Errorf(
				"search-term station must not run for %q", requirementQuote,
			)
		},
		func(acquisition existingRepositoryEvidenceAcquisition) error {
			events = append(events, "record:"+acquisition.RequirementQuote)
			return nil
		},
		func(acquisition existingRepositoryEvidenceAcquisition) (assemblyline.RepositoryChangeSurfaceDecision, error) {
			events = append(events, "surface:"+acquisition.RequirementQuote)
			return assemblyline.RepositoryChangeSurfaceDecision{
				Schema:  assemblyline.RepositoryChangeSurfaceSchemaV2,
				Targets: []assemblyline.RepositoryChangeTarget{{SymbolID: acquisition.Pack.Symbols[0].ID, Requirement: acquisition.RequirementQuote}},
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{
		"acquire:first exact requirement",
		"acquire:second exact requirement",
		"record:first exact requirement",
		"record:second exact requirement",
		"surface:first exact requirement",
		"surface:second exact requirement",
	}
	if !reflect.DeepEqual(events, wantEvents) || searchTermCalls != 0 || len(resolutions) != 2 {
		t.Fatalf("events=%v search-term calls=%d resolutions=%#v", events, searchTermCalls, resolutions)
	}
	for _, resolution := range resolutions {
		if resolution.Acquisition.SearchTermCalls != 0 {
			t.Fatalf("deterministic acquisition opened inference: %#v", resolution)
		}
	}
}

func TestRepositoryRequirementClosureOpensOneGapOnlyForTheMissingRequirement(t *testing.T) {
	t.Parallel()
	events := make([]string, 0, 8)
	searchTermCalls := make([]string, 0, 1)
	resolutions, err := prepareExistingRepositoryRequirementResolutions(
		[]string{"first exact requirement", "second exact requirement"},
		func(query string) (repositoryretrieval.EvidencePack, error) {
			events = append(events, "acquire:"+query)
			switch query {
			case "first exact requirement", `"alternate" OR "second" OR "term"`:
				return repositoryAcquisitionTestPack(t, query), nil
			case "second exact requirement":
				return repositoryretrieval.EvidencePack{}, repositoryretrieval.ErrInsufficientEvidence
			default:
				return repositoryretrieval.EvidencePack{}, fmt.Errorf("unexpected query %q", query)
			}
		},
		func(requirementQuote string) (assemblyline.RepositorySearchTermDecision, error) {
			events = append(events, "search-term:"+requirementQuote)
			searchTermCalls = append(searchTermCalls, requirementQuote)
			return assemblyline.RepositorySearchTermDecision{
				Schema:  assemblyline.RepositorySearchTermSchemaV2,
				Anchors: []string{"alternate second term"},
			}, nil
		},
		func(acquisition existingRepositoryEvidenceAcquisition) error {
			events = append(events, "record:"+acquisition.RequirementQuote)
			return nil
		},
		func(acquisition existingRepositoryEvidenceAcquisition) (assemblyline.RepositoryChangeSurfaceDecision, error) {
			events = append(events, "surface:"+acquisition.RequirementQuote)
			return assemblyline.RepositoryChangeSurfaceDecision{
				Schema:  assemblyline.RepositoryChangeSurfaceSchemaV2,
				Targets: []assemblyline.RepositoryChangeTarget{{SymbolID: acquisition.Pack.Symbols[0].ID, Requirement: acquisition.RequirementQuote}},
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{
		"acquire:first exact requirement",
		"acquire:second exact requirement",
		"search-term:second exact requirement",
		`acquire:"alternate" OR "second" OR "term"`,
		"record:first exact requirement",
		"record:second exact requirement",
		"surface:first exact requirement",
		"surface:second exact requirement",
	}
	if !reflect.DeepEqual(events, wantEvents) ||
		!reflect.DeepEqual(searchTermCalls, []string{"second exact requirement"}) ||
		len(resolutions) != 2 || resolutions[0].Acquisition.SearchTermCalls != 0 ||
		resolutions[1].Acquisition.SearchTermCalls != 1 ||
		resolutions[1].Acquisition.Query != `"alternate" OR "second" OR "term"` {
		t.Fatalf("events=%v search-term calls=%v resolutions=%#v", events, searchTermCalls, resolutions)
	}
}

func TestRepositoryRequirementClosureDispatchesNoSurfaceWhenAcquisitionFails(t *testing.T) {
	t.Parallel()
	events := make([]string, 0, 2)
	_, err := prepareExistingRepositoryRequirementResolutions(
		[]string{"first exact requirement", "second exact requirement"},
		func(query string) (repositoryretrieval.EvidencePack, error) {
			events = append(events, "acquire:"+query)
			if query == "first exact requirement" {
				return repositoryAcquisitionTestPack(t, query), nil
			}
			return repositoryretrieval.EvidencePack{}, fmt.Errorf("index unavailable")
		},
		func(string) (assemblyline.RepositorySearchTermDecision, error) {
			events = append(events, "search-term")
			return assemblyline.RepositorySearchTermDecision{}, nil
		},
		func(existingRepositoryEvidenceAcquisition) error {
			events = append(events, "record")
			return nil
		},
		func(existingRepositoryEvidenceAcquisition) (assemblyline.RepositoryChangeSurfaceDecision, error) {
			events = append(events, "surface")
			return assemblyline.RepositoryChangeSurfaceDecision{}, nil
		},
	)
	if err == nil || !reflect.DeepEqual(events, []string{
		"acquire:first exact requirement", "acquire:second exact requirement",
	}) {
		t.Fatalf("error=%v events=%v", err, events)
	}
}

func TestRepositoryRequirementClosureRejectsCrossAuthorityBeforeSurfaceInference(t *testing.T) {
	t.Parallel()
	events := make([]string, 0, 4)
	_, err := prepareExistingRepositoryRequirementResolutions(
		[]string{"first exact requirement", "second exact requirement"},
		func(query string) (repositoryretrieval.EvidencePack, error) {
			events = append(events, "acquire:"+query)
			pack := repositoryAcquisitionTestPack(t, query)
			if query == "second exact requirement" {
				pack.ID = ""
				pack.SnapshotID = "snapshot_" + strings.Repeat("9", 64)
				if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
					t.Fatal(err)
				}
			}
			return pack, nil
		},
		func(string) (assemblyline.RepositorySearchTermDecision, error) {
			events = append(events, "search-term")
			return assemblyline.RepositorySearchTermDecision{}, nil
		},
		func(existingRepositoryEvidenceAcquisition) error {
			events = append(events, "record")
			return nil
		},
		func(existingRepositoryEvidenceAcquisition) (assemblyline.RepositoryChangeSurfaceDecision, error) {
			events = append(events, "surface")
			return assemblyline.RepositoryChangeSurfaceDecision{}, nil
		},
	)
	if err == nil || !strings.Contains(err.Error(), "share one snapshot and analysis") ||
		!reflect.DeepEqual(events, []string{
			"acquire:first exact requirement", "acquire:second exact requirement",
		}) {
		t.Fatalf("error=%v events=%v", err, events)
	}
}

func repositoryAcquisitionTestPack(t *testing.T, query string) repositoryretrieval.EvidencePack {
	t.Helper()
	binding, err := repositoryretrieval.NewQueryBinding(
		repositoryretrieval.OperationSemanticExcerpts, query,
	)
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
		Relations: []repositoryretrieval.EvidenceRelation{}, SourceOmissions: []repositoryretrieval.SourceOmission{},
		OmittedSymbolIDs: []string{}, MaxBytes: 9 * 1024,
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	return pack
}
