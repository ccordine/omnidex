package queue

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/model"
	repositoryretrieval "github.com/gryph/omnidex/internal/repository/retrieval"
	"github.com/gryph/omnidex/internal/station"
)

func TestStructuredStationSchemasCrossRetiredThirtyTwoKiBRuler(t *testing.T) {
	t.Parallel()

	fixtures := []struct {
		name    string
		job     assemblyline.PortableJob
		station station.ID
	}{
		{
			name:    "acceptance grounding",
			job:     largeAcceptanceGroundingJob(t, 12),
			station: station.CodingWorkloadReview,
		},
		{
			name:    "repository change surface",
			job:     largeRepositoryChangeSurfaceJob(t),
			station: station.CodingRepositoryChange,
		},
	}
	for _, fixture := range fixtures {
		fixture := fixture
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			_, schema, err := assemblyline.RenderPortableJob(fixture.job)
			if err != nil {
				t.Fatal(err)
			}
			rawSchema, err := json.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			if len(rawSchema) <= 32*1024 || len(rawSchema) >= maxStationRequestResourceBytes {
				t.Fatalf("schema=%dB want between retired and coarse ceilings", len(rawSchema))
			}

			opening, err := validateStationGapOpening(StationGapOpenRecord{
				Authority: stationResponseSchemaAuthority(), Job: fixture.job, Station: fixture.station,
				ContextTokens: 262144, MaxOutputTokens: 262144,
				OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
			})
			if err != nil {
				t.Fatalf("station rejected renderer-admitted structured schema: %v", err)
			}
			if len(opening.ResponseSchema) != len(rawSchema) ||
				len(opening.ProjectionEnvelope) >= maxStationRequestResourceBytes {
				t.Fatalf("opening lost schema or coarse authority: schema=%d projection=%d", len(opening.ResponseSchema), len(opening.ProjectionEnvelope))
			}
		})
	}
}

func TestStructuredStationStillRejectsGrossProjectionEnvelope(t *testing.T) {
	t.Parallel()

	job := largeAcceptanceGroundingJob(t, 90)
	_, schema, err := assemblyline.RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	rawSchema, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	if len(rawSchema) <= maxStationRequestResourceBytes {
		t.Fatalf("gross schema fixture=%dB want >%d", len(rawSchema), maxStationRequestResourceBytes)
	}
	_, err = validateStationGapOpening(StationGapOpenRecord{
		Authority: stationResponseSchemaAuthority(), Job: job, Station: station.CodingWorkloadReview,
		ContextTokens: 262144, MaxOutputTokens: 262144,
		OutputLimitMode: llm.ExactPreparedOutputLimitNatural,
	})
	if err == nil || !strings.Contains(err.Error(), "projection exceeds coarse") {
		t.Fatalf("gross projection error=%v", err)
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
		for _, forbidden := range []string{"maxStationGapSchemaBytes", "station gap response schema exceeds"} {
			if strings.Contains(string(raw), forbidden) {
				t.Fatalf("%s retains standalone response-schema ruler %q", name, forbidden)
			}
		}
	}
}

func largeAcceptanceGroundingJob(t *testing.T, assertionCount int) assemblyline.PortableJob {
	t.Helper()
	var source strings.Builder
	source.WriteString("async function VerifyLargeAcceptance(): Promise<void> {\n")
	for index := 0; index < assertionCount; index++ {
		fmt.Fprintf(&source, "expect(screen.getByText(%q)).toBeInTheDocument();\n", fmt.Sprintf("Record %03d", index))
	}
	source.WriteString("}")
	criteria := make([]string, 4)
	for index := range criteria {
		criteria[index] = strings.Repeat("界", 510) + fmt.Sprintf("%02d", index)
	}
	input, err := assemblyline.NewApplicationAcceptanceGroundingReviewInput(
		assemblyline.ApplicationTaskContext{
			WorkloadSHA256: strings.Repeat("a", 64),
			Task: assemblyline.ApplicationTaskContextTask{
				TaskID: "task_001", AcceptanceCriteria: criteria,
			},
		},
		source.String(), true, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	job, err := assemblyline.NewApplicationAcceptanceGroundingReviewJob(input)
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func largeRepositoryChangeSurfaceJob(t *testing.T) assemblyline.PortableJob {
	t.Helper()
	const query = "select one repository change owner"
	binding, err := repositoryretrieval.NewQueryBinding(repositoryretrieval.OperationSemanticExcerpts, query)
	if err != nil {
		t.Fatal(err)
	}
	symbols := make([]repositoryretrieval.EvidenceSymbol, 20)
	for index := range symbols {
		symbols[index] = repositoryretrieval.EvidenceSymbol{
			ID:   fmt.Sprintf("SYMBOL_%02d_%s", index, strings.Repeat("x", 1700)),
			Kind: "function", Name: fmt.Sprintf("Symbol%02d", index),
			Signature: fmt.Sprintf("func Symbol%02d()", index),
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
	job, err := assemblyline.NewRepositoryChangeSurfaceJob(assemblyline.RepositoryChangeSurfaceInput{
		ResearchNeed: "Update the selected behavior.",
		Requirements: []string{"selected behavior"}, Evidence: pack,
	})
	if err != nil {
		t.Fatal(err)
	}
	return job
}

func stationResponseSchemaAuthority() model.StepAttemptAuthority {
	return model.StepAttemptAuthority{
		JobID: 41, Generation: 1, StepID: 7, Attempt: 1, WorkerID: "schema-resource-test",
	}
}
