package worker

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestSemanticObservationPreservesEntireValidatedResult(t *testing.T) {
	opened := model.PlanningActivity{Schema: 1, Kind: "semantic_started", WorkKind: "application_requirement_inventory", Station: "application_requirement_inventory", Model: "fixture-model", Question: "Which outcomes are requested?", Why: "Natural language cannot be extracted by byte equality.", Options: []string{"Free-form output: each requested outcome"}, Consumer: "Validate each proposed outcome."}
	result := strings.Repeat("A valid returned outcome with Unicode: café.\n", 200)
	var received strings.Builder
	var pages int
	runtime := typedWorkerRuntime{ObservePlanning: func(activity model.PlanningActivity) {
		if activity.Validate() != nil || activity.CallID != 17 || activity.Truncated {
			t.Fatal("result observation changed scope, lost data or violated its wire contract")
		}
		received.WriteString(activity.Result)
		pages++
	}}
	completeSemanticObservation(runtime, &opened, result, 17)
	if received.String() != strings.TrimSpace(result) || pages < 2 {
		t.Fatal("not every byte of the original validated result reached the terminal observation")
	}
}
