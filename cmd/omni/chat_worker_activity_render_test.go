package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/client"
)

func TestSemanticWorkerLifecycleShowsActualActionModelAndBoundary(t *testing.T) {
	for _, namespace := range []string{"coding", "objective"} {
		for _, status := range []string{"started", "completed", "rejected", "failed"} {
			t.Run(namespace+"/"+status, func(t *testing.T) {
				console, output := planReviewTestConsole(t)
				renderer := chatRenderer{console: console}
				event := workerRenderEvent(namespace+"_worker_"+status, "application_requirement_candidate_authorization")
				if err := renderer.realtime(event); err != nil {
					t.Fatal(err)
				}
				for _, required := range []string{"Check requirement authorization", "fixture-model", status} {
					if !strings.Contains(output.String(), required) {
						t.Errorf("actual semantic call omitted %q: %q", required, output.String())
					}
				}
				for _, forbidden := range []string{"kind=", "subject=", "model=", "attempt=", "stage ", "worker_"} {
					if strings.Contains(output.String(), forbidden) {
						t.Errorf("semantic action exposed raw header %q: %q", forbidden, output.String())
					}
				}
			})
		}
	}
}

func TestSemanticWorkerLiveActionsDistinguishActualQuestions(t *testing.T) {
	for _, test := range []struct{ subject, want string }{
		{"application_requirement_inventory", "Collect requirements"},
		{"application_requirement_candidate_authorization", "Check requirement authorization"},
		{"application_requirement_candidate_runtime_content_presence", "Classify runtime content"},
		{"application_requirement_candidate_non_runtime_content_presence", "Classify non-runtime content"},
		{"application_requirement_candidate_partition", "Partition requirement outcomes"},
		{"application_requirement_candidate_cardinality", "Check outcome count"},
		{"application_requirement_candidate_derived_value_presence", "Check derived values"},
		{"application_requirement_candidate_outcome_relation", "Check outcome relations"},
	} {
		t.Run(test.subject, func(t *testing.T) {
			detail, changed := chatEventActivity(workerRenderEvent("coding_worker_started", test.subject))
			if !changed || detail != test.want {
				t.Fatalf("distinct semantic question collapsed to %q changed %t; want %q", detail, changed, test.want)
			}
		})
	}
}

func TestQuietWorkerActivityKeepsFailuresAndHidesRoutineCalls(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console, verbosity: chatQuiet}
	for _, status := range []string{"started", "completed"} {
		if err := renderer.realtime(workerRenderEvent("coding_worker_"+status, "application_requirement_inventory")); err != nil {
			t.Fatal(err)
		}
	}
	if output.Len() != 0 {
		t.Fatalf("quiet output exposed routine call boundaries: %q", output.String())
	}
	if err := renderer.realtime(workerRenderEvent("coding_worker_failed", "application_requirement_inventory")); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "Collect requirements") || !strings.Contains(output.String(), "failed") {
		t.Fatalf("quiet output hid actual call failure: %q", output.String())
	}
}

func TestWorkspaceReadObservationsShowActualPathsWithoutDiagnostics(t *testing.T) {
	for _, test := range []struct{ kind, operation, path string }{
		{"workspace_directory_read", "list", "src"},
		{"workspace_file_read", "read", "src/main.go"},
	} {
		console, output := planReviewTestConsole(t)
		renderer := chatRenderer{console: console}
		if err := renderer.realtime(client.RealtimeEvent{EventName: client.RealtimeJobRuntimeEvent, JobID: 41, StepID: 12,
			RuntimeEvent: test.kind, FileOperation: test.operation, FilePath: test.path}); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), test.operation) || !strings.Contains(output.String(), test.path) || strings.Contains(output.String(), test.kind) {
			t.Fatalf("actual workspace observation was hidden or raw: %q", output.String())
		}
	}
}

func workerRenderEvent(kind, subject string) client.RealtimeEvent {
	return client.RealtimeEvent{EventName: client.RealtimeJobRuntimeEvent, JobID: 41, StepID: 12, Attempt: 1,
		RuntimeEvent: kind, Detail: "kind=semantic subject=" + subject + " model=fixture-model attempt=1/3"}
}
