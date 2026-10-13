package main

import (
	"strings"
	"testing"
)

func TestPlanningNormalShowsContinuedValidatedResult(t *testing.T) {
	for _, verbosity := range []chatVerbosity{chatNormal, chatVerbose, chatDebug} {
		console, output := planReviewTestConsole(t)
		renderer := chatRenderer{console: console, verbosity: verbosity}
		event := semanticPlanningJSONEvent(t, "semantic_completed", "the remaining validated content")
		event.PlanningActivity.ResultOffset = 4096
		event.PlanningActivity.ResultTotal = 4096 + len(event.PlanningActivity.Result)
		if err := renderer.realtime(event); err != nil {
			t.Fatal(err)
		}
		for _, required := range []string{"Result continued (bytes 4097", event.PlanningActivity.Result, "Call: 17"} {
			if !strings.Contains(output.String(), required) {
				t.Errorf("continued result omitted %q: %q", required, output.String())
			}
		}
		if strings.Contains(output.String(), "Options:") || strings.Contains(output.String(), "preview truncated") {
			t.Fatal("result continuation invented another check or discarded its content")
		}
	}
}
