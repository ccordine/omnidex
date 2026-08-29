package queue

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/exactjson"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
	"github.com/gryph/omnidex/internal/station"
)

func TestRawStationPromptsCrossRetiredThirtyTwoKiBRuler(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name    string
		job     assemblyline.PortableJob
		station station.ID
	}{
		{
			name:    "repository change owner",
			job:     largeRepositoryChangeOwnerJob(t),
			station: station.CodingRepositoryChange,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			prompt, err := assemblyline.RenderPortableJob(fixture.job)
			if err != nil {
				t.Fatal(err)
			}
			if len(prompt) <= 32*1024 || len(prompt) >= maxStationRequestResourceBytes {
				t.Fatalf("prompt=%dB want between retired and coarse ceilings", len(prompt))
			}

			opening, err := validateStationGapOpening(StationGapOpenRecord{
				Authority: stationRawProjectionAuthority(), Job: fixture.job, Station: fixture.station,
				ContextTokens:   262144,
				MaxOutputTokens: portableStationTestMaxOutputTokens(t, fixture.job, 262144),
				OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
			})
			if err != nil {
				t.Fatalf("station rejected raw renderer prompt: %v", err)
			}
			expectedProjection, err := exactjson.Canonical(struct {
				Prompt   string `json:"prompt"`
				Renderer string `json:"renderer"`
			}{prompt, assemblyline.PortableRendererV8})
			if err != nil {
				t.Fatal(err)
			}
			if opening.ProjectionEnvelope != string(expectedProjection) ||
				len(opening.ProjectionEnvelope) >= maxStationRequestResourceBytes {
				t.Fatalf("opening lost exact raw projection authority: projection=%q", opening.ProjectionEnvelope)
			}
		})
	}
}

func TestStandaloneStationResponseSchemaByteRulerIsAbsent(t *testing.T) {
	t.Parallel()

	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve queue source directory")
	}
	directory := filepath.Dir(testFile)
	for _, name := range []string{"station_gap_types.go", "station_gap_validation.go"} {
		raw, err := os.ReadFile(filepath.Join(directory, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{
			"ResponseSchema", "response_schema", "canonicalStationGapSchema",
		} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s retains standalone response-schema ruler %q", name, forbidden)
			}
		}
	}
}

func largeRepositoryChangeOwnerJob(t *testing.T) assemblyline.PortableJob {
	t.Helper()
	const query = "select one repository change owner"
	binding, err := repositoryretrieval.NewQueryBinding(repositoryretrieval.OperationSemanticExcerpts, query)
	if err != nil {
		t.Fatal(err)
	}
	symbols := make([]repositoryretrieval.EvidenceSymbol, 20)
	for index := range symbols {
		symbols[index] = repositoryretrieval.EvidenceSymbol{
			ID:   fmt.Sprintf("SYMBOL_%02d", index),
			Kind: "function", Name: fmt.Sprintf("Symbol%02d", index),
			Signature: fmt.Sprintf("func Symbol%02d(%s)", index, strings.Repeat("x", 1700)),
			Source:    fmt.Sprintf("func Symbol%02d() {}", index),
		}
	}
	pack := repositoryretrieval.EvidencePack{
		Schema:     repositoryretrieval.EvidencePackSchemaV2,
		SnapshotID: "snapshot", AnalysisID: "analysis",
		Operation: repositoryretrieval.OperationSemanticExcerpts, QueryBinding: binding,
		Symbols: symbols, Relations: []repositoryretrieval.EvidenceRelation{},
		SourceOmissions: []repositoryretrieval.SourceOmission{}, OmittedSymbolIDs: []string{},
		MaxBytes: 64 * 1024,
	}
	if err := repositoryretrieval.FinalizeEvidencePack(&pack); err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewRepositoryChangeOwnerJob(assemblyline.RepositoryChangeOwnerInput{
		Authority: assemblyline.RepositoryChangeSurfaceInput{
			ResearchNeed: "Update the selected behavior.",
			Requirements: []string{"selected behavior"}, Evidence: pack,
		},
		FocusedRequirement: "selected behavior",
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func stationRawProjectionAuthority() model.StepAttemptAuthority {
	return model.StepAttemptAuthority{
		JobID: 41, Generation: 1, StepID: 7, Attempt: 1, WorkerID: "schema-resource-test",
	}
}
