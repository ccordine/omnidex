package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/client"
)

func TestPlanningQuietHidesRoutineSemanticCheckBoundaries(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console, verbosity: chatQuiet}
	for _, namespace := range []string{"coding", "objective"} {
		for _, status := range []string{"started", "completed"} {
			if err := renderer.realtime(workerRenderEvent(namespace+"_worker_"+status, "application_requirement_candidate_authorization")); err != nil {
				t.Fatal(err)
			}
		}
	}
	if output.Len() != 0 {
		t.Fatalf("quiet output exposed routine checks: %q", output.String())
	}
}

func TestPlanningJSONProjectionShowsExactCandidateAndResolvedMeaning(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	for _, payload := range []string{
		`{"schema":1,"kind":"inventory_started","count":0}`,
		`{"schema":1,"kind":"inventory_completed","count":2}`,
		`{"schema":1,"kind":"candidate_started","subject":"Store \"draft\" data locally","count":0}`,
		`{"schema":1,"kind":"candidate_resolved","subject":"Store \"draft\" data locally","outcome":"retained","count":0}`,
		`{"schema":1,"kind":"candidate_resolved","subject":"Publish a public gallery","outcome":"unrequested","count":0}`,
		`{"schema":1,"kind":"selected","count":1}`,
	} {
		if err := renderer.realtime(planningJSONEvent(t, payload)); err != nil {
			t.Fatal(err)
		}
	}
	for _, required := range []string{
		"omni> Identifying outcomes",
		"omni> 2 proposed outcomes found",
		`omni> Considering outcome: Store "draft" data locally`,
		`omni> Included in current work: Store "draft" data locally`,
		"omni> Omitted because it was not requested: Publish a public gallery",
		"omni> 1 outcome selected for current work",
	} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("planning omitted observed meaning %q: %q", required, output.String())
		}
	}
	for _, forbidden := range []string{"fixture-model", "worker_started", "schema", "I think", "I decided"} {
		if strings.Contains(output.String(), forbidden) {
			t.Errorf("planning introduced diagnostics or invented thoughts %q: %q", forbidden, output.String())
		}
	}
}

func TestPlanningCandidateContextSurvivesMatchingGenericCheckStart(t *testing.T) {
	console, _ := planReviewTestConsole(t)
	if err := console.SetActivity(chatActivity{JobID: 41, StepID: 12, Label: "Planning", Working: true}); err != nil {
		t.Fatal(err)
	}
	session := chatSession{renderer: chatRenderer{console: console}}
	if err := session.acceptRealtime(semanticPlanningJSONEvent(t, "semantic_started", "")); err != nil {
		t.Fatal(err)
	}
	if err := session.acceptRealtime(workerRenderEvent("coding_worker_started", semanticPlanningStation)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(console.activityDetail, semanticPlanningSubject) || !strings.Contains(console.activityDetail, semanticPlanningQuestion) {
		t.Fatalf("generic check replaced the candidate being considered: %q", console.activityDetail)
	}
}

func TestPlanningKeepsFailuresAndVerboseDiagnostics(t *testing.T) {
	for _, verbosity := range []chatVerbosity{chatQuiet, chatNormal, chatVerbose, chatDebug} {
		t.Run(verbosity.String(), func(t *testing.T) {
			console, output := planReviewTestConsole(t)
			renderer := chatRenderer{console: console, verbosity: verbosity}
			for _, status := range []string{"failed", "rejected"} {
				if err := renderer.realtime(workerRenderEvent("coding_worker_"+status, "application_requirement_candidate_authorization")); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(output.String(), status) || !strings.Contains(output.String(), "fixture-model") {
					t.Fatalf("observed failure/rejection was hidden: %q", output.String())
				}
			}
			if verbosity != chatVerbose && verbosity != chatDebug {
				return
			}
			output.Reset()
			if err := renderer.realtime(workerRenderEvent("coding_worker_started", "application_requirement_candidate_authorization")); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "started") || !strings.Contains(output.String(), "fixture-model") {
				t.Fatalf("diagnostic verbosity hid the underlying call: %q", output.String())
			}
		})
	}
}

func TestPlanningRejectsInvalidProjectionBeforeShowingIt(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	if err := renderer.realtime(planningJSONEvent(t, `{"schema":999,"kind":"candidate_resolved","subject":"Unvalidated outcome","outcome":"retained"}`)); err == nil {
		t.Fatal("invalid planning schema was silently accepted")
	}
	if output.Len() != 0 {
		t.Fatalf("invalid observation was presented as current work: %q", output.String())
	}
}

func planningJSONEvent(t *testing.T, payload string) client.RealtimeEvent {
	t.Helper()
	var event client.RealtimeEvent
	if err := json.Unmarshal([]byte(`{"eventName":"job-runtime-event","jobID":41,"stepID":12,"attempt":1,"runtimeEvent":"planning_activity","planningActivity":`+payload+`}`), &event); err != nil {
		t.Fatal(err)
	}
	return event
}
