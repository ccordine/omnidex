package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestPlanningNormalSuppressesOnlyAlreadyRepresentedWorkerBoundaries(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	if err := renderer.realtime(semanticPlanningJSONEvent(t, "semantic_started", "")); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := renderer.realtime(workerRenderEvent("coding_worker_started", semanticPlanningStation)); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("rich check was repeated as a generic started header: %q", output.String())
	}
	if err := renderer.realtime(semanticPlanningJSONEvent(t, "semantic_completed", semanticPlanningResult)); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := renderer.realtime(workerRenderEvent("coding_worker_completed", semanticPlanningStation)); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("rich result was repeated as a generic completed header: %q", output.String())
	}
}

func TestPlanningNormalKeepsUnmatchedWorkerBoundaries(t *testing.T) {
	for _, changed := range []string{"job", "step", "attempt", "station", "model", "phase"} {
		t.Run(changed, func(t *testing.T) {
			console, output := planReviewTestConsole(t)
			renderer := chatRenderer{console: console}
			if err := renderer.realtime(semanticPlanningJSONEvent(t, "semantic_started", "")); err != nil {
				t.Fatal(err)
			}
			event := workerRenderEvent("coding_worker_started", semanticPlanningStation)
			switch changed {
			case "job":
				event.JobID++
			case "step":
				event.StepID++
			case "attempt":
				event.Attempt++
			case "station":
				event.Detail = strings.ReplaceAll(event.Detail, semanticPlanningStation, "application_requirement_candidate_authorization")
			case "model":
				event.Detail = strings.ReplaceAll(event.Detail, "fixture-model", "different-model")
			case "phase":
				event.RuntimeEvent = "coding_worker_completed"
			}
			output.Reset()
			if err := renderer.realtime(event); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "fixture-model") && !strings.Contains(output.String(), "different-model") {
				t.Fatalf("unmatched actual check was hidden: %q", output.String())
			}
		})
	}
}

func TestPlanningCorrelationKeepsFailureAndReleasesIt(t *testing.T) {
	for _, status := range []string{"failed", "rejected"} {
		t.Run(status, func(t *testing.T) {
			console, output := planReviewTestConsole(t)
			renderer := chatRenderer{console: console}
			if err := renderer.realtime(semanticPlanningJSONEvent(t, "semantic_started", "")); err != nil {
				t.Fatal(err)
			}
			output.Reset()
			if err := renderer.realtime(workerRenderEvent("coding_worker_"+status, semanticPlanningStation)); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), status) {
				t.Fatalf("rich trace hid actual failure/rejection: %q", output.String())
			}
			output.Reset()
			if err := renderer.realtime(workerRenderEvent("coding_worker_started", semanticPlanningStation)); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "started") {
				t.Fatalf("retired trace hid an unmatched later boundary: %q", output.String())
			}
		})
	}
}

func TestPlanningCorrelationRetiresWhenCurrentPromptScopeChanges(t *testing.T) {
	for _, changed := range []string{"idle", "waiting", "other_step", "other_job", "same_scope"} {
		t.Run(changed, func(t *testing.T) {
			console, output := planReviewTestConsole(t)
			renderer := chatRenderer{console: console}
			if err := renderer.realtime(semanticPlanningJSONEvent(t, "semantic_started", "")); err != nil {
				t.Fatal(err)
			}
			details := &model.JobDetails{
				Job:   model.Job{ID: 41, Status: model.JobStatusRunning},
				Steps: []model.Step{{ID: 12, Status: model.StepStatusRunning}},
			}
			switch changed {
			case "idle":
				details = nil
			case "waiting":
				details.Job.Status = model.JobStatusWaiting
			case "other_step":
				details.Steps[0].ID++
			case "other_job":
				details.Job.ID++
			}
			if err := renderer.prompt(details); err != nil {
				t.Fatal(err)
			}
			output.Reset()
			if err := renderer.realtime(workerRenderEvent("coding_worker_started", semanticPlanningStation)); err != nil {
				t.Fatal(err)
			}
			visible := strings.Contains(output.String(), "started")
			if visible == (changed == "same_scope") {
				t.Fatalf("prompt scope %q retained or retired the wrong trace: %q", changed, output.String())
			}
		})
	}
}

func TestPlanningCorrelatedWorkerDiagnosticsRemainVisible(t *testing.T) {
	for _, verbosity := range []chatVerbosity{chatVerbose, chatDebug} {
		t.Run(verbosity.String(), func(t *testing.T) {
			console, output := planReviewTestConsole(t)
			renderer := chatRenderer{console: console, verbosity: verbosity}
			if err := renderer.realtime(semanticPlanningJSONEvent(t, "semantic_started", "")); err != nil {
				t.Fatal(err)
			}
			output.Reset()
			if err := renderer.realtime(workerRenderEvent("coding_worker_started", semanticPlanningStation)); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(output.String(), "fixture-model") || !strings.Contains(output.String(), "started") {
				t.Fatalf("correlation hid requested diagnostic detail: %q", output.String())
			}
		})
	}
}
