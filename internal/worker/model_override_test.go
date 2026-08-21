package worker

import (
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/gryph/omnidex/internal/station"
)

func TestConcurrentJobStationRoutingIsIsolated(t *testing.T) {
	service := &Service{models: ModelRouting{
		Stations: map[station.ID]string{
			station.ConversationObjectiveKind: "base-kind",
		},
	}}
	metadata := []json.RawMessage{
		json.RawMessage(`{"model_config":{"conversation_objective_kind_model":"job-a"}}`),
		json.RawMessage(`{"model_config":{"conversation_objective_kind_model":"job-b"}}`),
	}
	errors := make(chan string, 200)
	var workers sync.WaitGroup
	for index := 0; index < 200; index++ {
		workers.Add(1)
		go func(raw json.RawMessage, want string) {
			defer workers.Done()
			routing, err := modelRoutingFromJobMetadata(raw, service.models)
			if err != nil {
				errors <- err.Error()
				return
			}
			got, err := service.requiredStationModel(routing, station.ConversationObjectiveKind)
			if err != nil || got != want {
				errors <- "unexpected station route"
			}
		}(
			metadata[index%2],
			[]string{"job-a", "job-b"}[index%2],
		)
	}
	workers.Wait()
	close(errors)
	for message := range errors {
		t.Error(message)
	}
	if service.models.Stations[station.ConversationObjectiveKind] != "base-kind" {
		t.Fatal("job-local station routing mutated shared service routing")
	}
}

func TestModelRoutingFromJobMetadataRejectsMalformedConfig(t *testing.T) {
	base := ModelRouting{Stations: map[station.ID]string{station.ConversationResponse: "base"}}
	for _, metadata := range []json.RawMessage{
		json.RawMessage(`{`),
		json.RawMessage(`{"model_config":{"unknown_model":"x"}}`),
		json.RawMessage(`{"model_config":{"default_model":42}}`),
		json.RawMessage(`{"model_plan":42}`),
		json.RawMessage(`{"model_plan":" "}`),
		json.RawMessage(`{"model_execute":"coder"}`),
	} {
		if _, err := modelRoutingFromJobMetadata(metadata, base); err == nil {
			t.Fatalf("metadata %s must fail", metadata)
		}
	}
}

func TestRequiredStationModelFailsLoudly(t *testing.T) {
	service := &Service{}
	if _, err := service.requiredStationModel(ModelRouting{}, station.GroundedAnswer); err == nil ||
		!strings.Contains(err.Error(), "has no configured model") {
		t.Fatalf("missing station model error=%v", err)
	}
	if _, err := stationModel(ModelRouting{}, station.ID("planner_specialist")); err == nil {
		t.Fatal("unregistered persona-shaped route was accepted")
	}
}
