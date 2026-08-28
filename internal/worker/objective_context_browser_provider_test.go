package worker

import (
	"context"
	"testing"

	"github.com/gryph/omnidex/internal/assemblyline"
	"github.com/gryph/omnidex/internal/station"
)

type contextRelevanceExecutorProbe struct {
	model string
	input assemblyline.ContextRelevanceSelectionInput
	value assemblyline.ContextRelevanceSelectionDecision
	calls int
}

func (probe *contextRelevanceExecutorProbe) ExecuteContextRelevance(
	_ context.Context,
	model string,
	input assemblyline.ContextRelevanceSelectionInput,
) (assemblyline.ContextRelevanceSelectionDecision, error) {
	probe.calls++
	probe.model = model
	probe.input = input
	return probe.value, nil
}

func TestContextRelevanceUsesConfiguredBrowserProviderBelowTheStationContract(t *testing.T) {
	input := browserContextRelevanceInputFixture(t)
	probe := &contextRelevanceExecutorProbe{value: assemblyline.ContextRelevanceSelectionDecision{
		CandidateID: "CTX_1",
	}}
	adapter := portableObjectiveContextSieveStations{runtime: &nativeRuntimeV3{
		svc: &Service{browserContextRelevance: probe},
		routing: ModelRouting{Stations: map[station.ID]string{
			station.ContextRelevance: "qualified-browser-model",
		}},
	}}
	decision, receipt, err := adapter.SelectRelevant(t.Context(), input)
	if err != nil {
		t.Fatal(err)
	}
	if probe.calls != 1 || probe.model != "qualified-browser-model" ||
		probe.input.Authority.ExactInstruction != input.ExactInstruction || receipt.Calls != 1 ||
		len(decision.ReferencedCandidateIDs) != 1 || decision.ReferencedCandidateIDs[0] != "CTX_1" {
		t.Fatalf("probe=%#v receipt=%#v decision=%#v", probe, receipt, decision)
	}
}

func TestContextRelevanceRevalidatesBrowserProviderResult(t *testing.T) {
	input := browserContextRelevanceInputFixture(t)
	probe := &contextRelevanceExecutorProbe{value: assemblyline.ContextRelevanceSelectionDecision{
		CandidateID: "CTX_99",
	}}
	adapter := portableObjectiveContextSieveStations{runtime: &nativeRuntimeV3{
		svc: &Service{browserContextRelevance: probe},
		routing: ModelRouting{Stations: map[station.ID]string{
			station.ContextRelevance: "qualified-browser-model",
		}},
	}}
	if _, receipt, err := adapter.SelectRelevant(t.Context(), input); err == nil || receipt.Calls != 1 {
		t.Fatalf("receipt=%#v error=%v", receipt, err)
	}
}

func TestRoleplayContextRelevanceBypassesBrowserAndUsesSemanticRoute(t *testing.T) {
	input := browserContextRelevanceInputFixture(t)
	input.Scope = assemblyline.ContextScopeRoleplaySimulation
	probe := &contextRelevanceExecutorProbe{value: assemblyline.ContextRelevanceSelectionDecision{
		CandidateID: "CTX_1",
	}}
	runtime := &nativeRuntimeV3{
		svc: &Service{browserContextRelevance: probe},
		routing: ModelRouting{
			Stations: map[station.ID]string{
				station.ContextRelevance: "browser-only-model",
			},
			RoleplaySemanticModel: "roleplay-semantic-model",
		},
	}
	model, err := objectiveContextStationModel(
		runtime, input.Scope, station.ContextRelevance,
	)
	if err != nil {
		t.Fatal(err)
	}
	if model != "roleplay-semantic-model" {
		t.Fatalf("roleplay context model=%q", model)
	}
	adapter := portableObjectiveContextSieveStations{runtime: runtime}
	if _, _, err := adapter.SelectRelevant(t.Context(), input); err == nil {
		t.Fatal("roleplay context unexpectedly executed without running-step authority")
	}
	if probe.calls != 0 {
		t.Fatalf("roleplay context dispatched %d browser calls", probe.calls)
	}
}

func browserContextRelevanceInputFixture(t *testing.T) assemblyline.ContextRelevanceInput {
	t.Helper()
	candidate, err := assemblyline.NewContextCandidateAuthority(
		"conversation_exchange", "CTX_1", "The west gate was closed before dusk.",
	)
	if err != nil {
		t.Fatal(err)
	}
	return assemblyline.ContextRelevanceInput{
		ExactInstruction: "Do that again.", RetrievalConcepts: []string{"previous gate action"},
		CandidateAuthorities: []assemblyline.ContextCandidateAuthority{candidate},
		MaxSelections:        1,
	}
}
