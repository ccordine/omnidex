package worker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/llm"
	"github.com/gryph/omnidex/internal/objectiveadvisory"
	"github.com/gryph/omnidex/internal/station"
)

type fixedObjectiveAdvisoryEmbedder struct{}

func (fixedObjectiveAdvisoryEmbedder) Embedding(context.Context, string) ([]float64, error) {
	return []float64{1, 0}, nil
}

func TestObjectiveAdvisoryRunnerOffSkipsAllRoutingAndTransports(t *testing.T) {
	runner, err := (&nativeRuntimeV3{svc: &Service{
		objectiveAdvisoryMode: objectiveadvisory.ModeOff,
	}}).newObjectiveAdvisoryRunner()
	if err != nil {
		t.Fatal(err)
	}
	if runner != nil {
		t.Fatalf("off advisory constructed a runtime: %#v", runner)
	}
}

func TestObjectiveAdvisoryRunnerUsesImmutableJobLocalModelAndLogsMetadataOnly(t *testing.T) {
	client := &objectiveAdvisoryProviderTestClient{}
	var logs bytes.Buffer
	shared := ModelRouting{Stations: map[station.ID]string{
		station.ObjectiveAdvisory: "shared-model",
	}}
	runtime := &nativeRuntimeV3{
		svc: &Service{
			stationClient: client, embeddings: fixedObjectiveAdvisoryEmbedder{},
			inferenceContextTokens: 32768,
			models:                 shared, objectiveAdvisoryMode: objectiveadvisory.ModeActive,
			objectiveAdvisoryProvider: llm.ExactPreparedProviderBackend,
			logger:                    log.New(&logs, "", 0),
		},
		routing: ModelRouting{Stations: map[station.ID]string{
			station.ObjectiveAdvisory: "qwen3.5:9b-q4_K_M",
		}},
	}
	runner, err := runtime.newObjectiveAdvisoryRunner()
	if err != nil {
		t.Fatal(err)
	}
	request := validObjectiveAdvisoryGenerateRequest(t)
	report, err := runner.Run(context.Background(), request.Projection.Input, objectiveadvisory.SemanticGap{
		ObjectiveID: request.Projection.Input.ObjectiveID,
		Generation:  request.Projection.Input.Generation,
		Requirement: request.Projection.Input.Objective,
		Candidate:   "The candidate answer cites the registered workflow.",
		Evidence:    request.Projection.Input.GroundedEvidence,
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Mode != objectiveadvisory.ModeActive || len(report.ActiveCapsules) != 1 {
		t.Fatalf("active advisory report=%+v", report)
	}
	if client.selection.Model != "qwen3.5:9b-q4_K_M" {
		t.Fatalf("provider used model %q", client.selection.Model)
	}
	if shared.Stations[station.ObjectiveAdvisory] != "shared-model" {
		t.Fatal("job-local objective advisory routing mutated shared routing")
	}
	logText := logs.String()
	if strings.Count(logText, "\n") != 1 ||
		!strings.Contains(logText, `mode="active"`) ||
		!strings.Contains(logText, `artifact_statuses="succeeded"`) ||
		!strings.Contains(logText, `requested_models="qwen3.5:9b-q4_K_M"`) ||
		!strings.Contains(logText, "potential_capsule_content_tokens=") ||
		!strings.Contains(logText, "selected_capsule_content_bytes=") ||
		!strings.Contains(logText, "selected_capsule_content_tokens=") {
		t.Fatalf("advisory log=%q", logText)
	}
	if strings.Contains(logText, "Check the boundary condition") {
		t.Fatalf("advisory log exposed raw advisory text: %q", logText)
	}
}

func TestObjectiveAdvisoryProviderFailureIsReportedAndLoggedWithoutStoppingRuntime(t *testing.T) {
	client := &objectiveAdvisoryProviderTestClient{callError: errors.New("provider unavailable")}
	var logs bytes.Buffer
	runtime := &nativeRuntimeV3{
		svc: &Service{
			stationClient: client, embeddings: fixedObjectiveAdvisoryEmbedder{},
			inferenceContextTokens:    32768,
			objectiveAdvisoryMode:     objectiveadvisory.ModeShadow,
			objectiveAdvisoryProvider: llm.ExactPreparedProviderBackend,
			logger:                    log.New(&logs, "", 0),
		},
		routing: ModelRouting{Stations: map[station.ID]string{
			station.ObjectiveAdvisory: "qwen3.5:9b-q4_K_M",
		}},
	}
	runner, err := runtime.newObjectiveAdvisoryRunner()
	if err != nil {
		t.Fatal(err)
	}
	request := validObjectiveAdvisoryGenerateRequest(t)
	report, err := runner.Run(context.Background(), request.Projection.Input, objectiveadvisory.SemanticGap{
		ObjectiveID: request.Projection.Input.ObjectiveID,
		Generation:  request.Projection.Input.Generation,
		Requirement: request.Projection.Input.Objective,
		Candidate:   "Candidate answer.", Evidence: request.Projection.Input.GroundedEvidence,
	})
	if err != nil {
		t.Fatalf("provider failure stopped baseline: %v", err)
	}
	if len(report.Artifacts) != 1 || report.Artifacts[0].Status != objectiveadvisory.StatusFailed ||
		report.ReductionStatus != objectiveadvisory.StatusFailed {
		t.Fatalf("provider failure report=%+v", report)
	}
	if !strings.Contains(logs.String(), "provider unavailable") ||
		!strings.Contains(logs.String(), `artifact_statuses="failed"`) {
		t.Fatalf("provider failure log=%q", logs.String())
	}
}

func TestObjectiveAdvisoryEnabledRunnerRejectsInvalidConfiguration(t *testing.T) {
	for _, service := range []*Service{
		{objectiveAdvisoryMode: objectiveadvisory.ModeActive},
		{
			objectiveAdvisoryMode:     objectiveadvisory.ModeActive,
			objectiveAdvisoryProvider: "openai",
			stationClient:             &objectiveAdvisoryProviderTestClient{},
			embeddings:                fixedObjectiveAdvisoryEmbedder{}, logger: log.New(io.Discard, "", 0),
		},
	} {
		if runner, err := (&nativeRuntimeV3{svc: service}).newObjectiveAdvisoryRunner(); err == nil || runner != nil {
			t.Fatalf("invalid enabled configuration returned runner=%#v err=%v", runner, err)
		}
	}
}
