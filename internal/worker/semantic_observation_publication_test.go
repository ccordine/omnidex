package worker

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestSemanticOptionsFitWireWithoutOmittingAnyOfferedChoice(t *testing.T) {
	opened := model.PlanningActivity{Schema: 1, Kind: "semantic_started", WorkKind: "database_query_filter_value", Station: "database_query_filter_value", Model: "fixture-model", Question: "Which available value is required?", Why: "Natural language does not choose an enum value mechanically.", Consumer: "Decode the offered value."}
	for i := 0; i < 30; i++ {
		opened.Options = append(opened.Options, strings.Repeat(string(rune('A'+i)), 2048))
	}
	var emitted []model.PlanningActivity
	runtime := typedWorkerRuntime{ObservePlanning: func(activity model.PlanningActivity) { emitted = append(emitted, activity) }}
	if err := publishSemanticOpening(runtime, opened); err != nil {
		t.Fatal(err)
	}
	if len(emitted) < 2 {
		t.Fatal("oversized options were not split at the existing wire boundary")
	}
	var got []string
	for _, page := range emitted {
		encoded, err := json.Marshal(page)
		if err != nil || page.Validate() != nil || len(encoded) > model.MaxPlanningActivityBytes || page.WorkKind != opened.WorkKind || page.Station != opened.Station || page.Model != opened.Model || page.OptionOffset != len(got) || page.OptionTotal != len(opened.Options) {
			t.Fatal("option page lost original scope, sequence or wire bound")
		}
		got = append(got, page.Options...)
	}
	if !reflect.DeepEqual(got, opened.Options) {
		t.Fatal("not every actually offered choice reached the observer")
	}
	completeSemanticObservation(runtime, &opened, "A", 17)
	last := emitted[len(emitted)-1]
	if last.Kind != "semantic_completed" || last.CallID != 17 || last.Result != "A" || last.Validate() != nil || last.OptionTotal != len(opened.Options) {
		t.Fatal("large option inventory lost its completed result")
	}
}
