package worker

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
)

func TestRepositoryRequirementClosureAcquiresCodeOwnedAuthorityBeforeSurfaceInference(t *testing.T) {
	t.Parallel()
	const authority = "Change both exact existing behaviors."
	events := make([]string, 0, 5)
	resolutions, err := prepareExistingRepositoryRequirementResolutions(
		[]string{"first exact requirement", "second exact requirement"},
		authority,
		func(query string) (repositoryretrieval.EvidencePack, error) {
			events = append(events, "acquire:"+query)
			return repositoryAcquisitionTestPack(t, query), nil
		},
		func(acquisition existingRepositoryEvidenceAcquisition) error {
			events = append(events, "record:"+acquisition.RequirementQuote)
			return nil
		},
		func(acquisition existingRepositoryEvidenceAcquisition) (assemblyline.RepositoryChangeSurfaceDecision, error) {
			events = append(events, "surface:"+acquisition.RequirementQuote)
			return assemblyline.RepositoryChangeSurfaceDecision{
				Schema: assemblyline.RepositoryChangeSurfaceSchemaV2,
				Targets: []assemblyline.RepositoryChangeTarget{{
					SymbolID:    acquisition.Pack.Symbols[0].ID,
					Requirement: acquisition.RequirementQuote,
				}},
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	wantEvents := []string{
		"acquire:" + authority,
		"record:first exact requirement",
		"record:second exact requirement",
		"surface:first exact requirement",
		"surface:second exact requirement",
	}
	if !reflect.DeepEqual(events, wantEvents) || len(resolutions) != 2 {
		t.Fatalf("events=%v resolutions=%#v", events, resolutions)
	}
	for _, resolution := range resolutions {
		if resolution.Acquisition.Query != authority ||
			resolution.Acquisition.Need.Schema != assemblyline.ApplicationEvidenceNeedSchemaV2 {
			t.Fatalf("model-derived operation authority leaked into acquisition: %#v", resolution)
		}
	}
}

func TestRepositoryRequirementClosureDispatchesNoSurfaceWhenAcquisitionFails(t *testing.T) {
	t.Parallel()
	events := make([]string, 0, 2)
	_, err := prepareExistingRepositoryRequirementResolutions(
		[]string{"first exact requirement", "second exact requirement"},
		"Change both exact existing behaviors.",
		func(query string) (repositoryretrieval.EvidencePack, error) {
			events = append(events, "acquire:"+query)
			return repositoryretrieval.EvidencePack{}, fmt.Errorf("index unavailable")
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
		"acquire:Change both exact existing behaviors.",
	}) {
		t.Fatalf("error=%v events=%v", err, events)
	}
}

func TestRepositoryRequirementClosureRejectsInvalidPackBeforeSurfaceInference(t *testing.T) {
	t.Parallel()
	events := make([]string, 0, 4)
	_, err := prepareExistingRepositoryRequirementResolutions(
		[]string{"first exact requirement", "second exact requirement"},
		"Change both exact existing behaviors.",
		func(query string) (repositoryretrieval.EvidencePack, error) {
			events = append(events, "acquire:"+query)
			pack := repositoryAcquisitionTestPack(t, query)
			pack.QueryBinding = "query_binding_" + strings.Repeat("9", 64)
			return pack, nil
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
		"acquire:Change both exact existing behaviors.",
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
