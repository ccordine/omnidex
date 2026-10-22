package main

import (
	"strings"
	"testing"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func TestChatRendererDefaultSuppressesInternalRealtime(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	for _, event := range []client.RealtimeEvent{
		{EventName: client.RealtimeJobProgress, JobID: 41, Summary: "raw progress", Phase: "state_changed"},
		{EventName: client.RealtimeJobRuntimeEvent, JobID: 41, StepID: 12, RuntimeEvent: "station_call", Detail: "raw station"},
		{EventName: client.RealtimeJobRuntimeEvent, JobID: 41, StepID: 12, RuntimeEvent: "worker_heartbeat", Detail: "raw heartbeat"},
	} {
		if err := renderer.realtime(event); err != nil {
			t.Fatal(err)
		}
	}
	if output.Len() != 0 {
		t.Fatalf("internal realtime flooded ordinary chat: %q", output.String())
	}
	for _, event := range []client.RealtimeEvent{
		{EventName: client.RealtimeJobRuntimeEvent, JobID: 41, StepID: 12, RuntimeEvent: "workspace_file_changed", FileOperation: "move", FileSourcePath: "old.go", FilePath: "new.go"},
		{EventName: client.RealtimeJobRuntimeEvent, JobID: 41, StepID: 12, RuntimeEvent: "step_failed", Detail: "permission denied"},
	} {
		if err := renderer.realtime(event); err != nil {
			t.Fatal(err)
		}
	}
	for _, required := range []string{"old.go", "new.go", "permission denied"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("ordinary chat omitted %q: %q", required, output.String())
		}
	}
}

func TestChatRendererJobShowsPhaseWithoutRepeatingRequest(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	details := renderActivityJob()
	if err := renderer.job(details, nil); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Working"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("task presentation omitted %q: %q", required, output.String())
		}
	}
	for _, forbidden := range []string{"generation", "stage", "v3_coding", "step 12", "Preserve this task", "task>"} {
		if strings.Contains(output.String(), forbidden) {
			t.Errorf("ordinary task presentation contains diagnostic %q: %q", forbidden, output.String())
		}
	}
	before := output.Len()
	next := details
	next.Steps = append([]model.Step(nil), details.Steps...)
	next.Steps = append(next.Steps, model.Step{ID: 13, Generation: 2, Action: "v3_coding", Status: model.StepStatusCompleted})
	if err := renderer.job(next, &details); err != nil {
		t.Fatal(err)
	}
	if output.Len() != before {
		t.Fatalf("same human phase repeated on internal step update: %q", output.String()[before:])
	}
	changed := next
	changed.Steps = []model.Step{{ID: 14, Generation: 2, Action: "v3_coding_plan", Status: model.StepStatusRunning}}
	if err := renderer.job(changed, &next); err != nil {
		t.Fatal(err)
	}
	if segment := output.String()[before:]; !strings.Contains(segment, "Planning") || strings.Contains(segment, "Preserve this task") {
		t.Fatalf("phase transition repeated task or omitted planning: %q", segment)
	}
}

func TestChatRendererShowsChangedErrorsWithoutRepeatingThem(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	previous := renderActivityJob()
	next := previous
	next.Steps = append([]model.Step(nil), previous.Steps...)
	next.Steps[0].Error = "permission denied"
	if err := renderer.job(next, &previous); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "permission denied") {
		t.Fatalf("unchanged job status hid changed step error: %q", output.String())
	}
	before := output.Len()
	if err := renderer.job(next, &next); err != nil {
		t.Fatal(err)
	}
	if output.Len() != before {
		t.Fatalf("unchanged error repeated: %q", output.String()[before:])
	}
}

func TestChatRendererAuthorityEndedIsStoppedAndRetainsDebugEvidence(t *testing.T) {
	const detail = "step execution authority was lost: job generation changed"
	for _, level := range []chatVerbosity{chatNormal, chatVerbose, chatDebug} {
		console, output := planReviewTestConsole(t)
		renderer := chatRenderer{console: console, verbosity: level}
		if err := renderer.realtime(client.RealtimeEvent{
			EventName: client.RealtimeJobRuntimeEvent, JobID: 41, StepID: 12,
			RuntimeEvent: "step_authority_lost", Detail: detail,
		}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(output.String(), "error>") {
			t.Fatalf("ended authority displayed as a new failure: %q", output.String())
		}
		if level == chatDebug {
			if !strings.Contains(output.String(), detail) {
				t.Fatalf("debug omitted actual authority evidence: %q", output.String())
			}
		} else if !strings.Contains(output.String(), "Work stopped") {
			t.Fatalf("ended authority lacks truthful stopped state: %q", output.String())
		}
	}
}

func TestChatRendererCanceledReasonIsStatusAndFailureRemainsError(t *testing.T) {
	for _, status := range []string{model.JobStatusCanceled, model.JobStatusFailed} {
		console, output := planReviewTestConsole(t)
		renderer := chatRenderer{console: console}
		details := renderActivityJob()
		details.Job.Status, details.Job.Error = status, "Recorded reason"
		details.Steps[0].Status, details.Steps[0].Error = status, "Recorded reason"
		if err := renderer.job(details, nil); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "Recorded reason") || strings.Contains(output.String(), "error>") != (status == model.JobStatusFailed) {
			t.Fatalf("status %s has incorrect error/reason presentation: %q", status, output.String())
		}
	}
}

func TestChatPersistedTurnCancellationRetainsReasonWithoutFailure(t *testing.T) {
	for _, status := range []string{model.JobStatusCanceled, model.JobStatusFailed} {
		console, output := planReviewTestConsole(t)
		message := model.ChannelMessage{Turn: &model.ChannelMessageTurnState{
			JobID: 41, Status: status, Error: "Recorded reason",
		}}
		if err := (chatRenderer{console: console}).messageTurnState(message); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(output.String(), "Recorded reason") || strings.Contains(output.String(), "error>") != (status == model.JobStatusFailed) {
			t.Fatalf("persisted status %s has incorrect reason/error presentation: %q", status, output.String())
		}
	}
}

func TestChatRendererStatusShowsFullTaskWithoutInternalStages(t *testing.T) {
	console, output := planReviewTestConsole(t)
	details := renderActivityJob()
	details.Job.Instruction += "\nKeep the complete supplied request."
	details.Job.Error = "job failure detail"
	if err := (chatRenderer{console: console}).status(details); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"Working", "Preserve this task", "Keep the complete supplied request.", "job failure detail"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("status omitted %q: %q", required, output.String())
		}
	}
	if strings.Contains(output.String(), "v3_coding") || strings.Contains(output.String(), "generation") {
		t.Fatalf("default status exposed internal stages: %q", output.String())
	}
}

func TestChatRendererBannerKeepsProjectAndControls(t *testing.T) {
	console, output := planReviewTestConsole(t)
	channel := model.Channel{ID: "cli-chat-private-identity", WorkspaceRoot: "/client/project"}
	if err := (chatRenderer{console: console}).banner(channel); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"/client/project", "Agents", "/help", "/exit"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("banner omitted %q: %q", required, output.String())
		}
	}
	if strings.Contains(output.String(), string(channel.ID)) {
		t.Fatalf("banner exposed technical session identity: %q", output.String())
	}
}

func TestChatRendererDiagnosticsRestoresRawEventsAndStages(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console, verbosity: chatDebug}
	if err := renderer.job(renderActivityJob(), nil); err != nil {
		t.Fatal(err)
	}
	if err := renderer.realtime(client.RealtimeEvent{
		EventName: client.RealtimeJobRuntimeEvent, JobID: 41, StepID: 12,
		RuntimeEvent: "station_call", Detail: "raw station detail",
	}); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"generation 2", "stage v3_coding", "station_call", "raw station detail"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("opt-in diagnostics omitted %q: %q", required, output.String())
		}
	}
}

func TestActivityForJobLabelsTruthfulStates(t *testing.T) {
	for _, test := range []struct {
		status, action, want string
		working              bool
	}{
		{model.JobStatusPending, "v3_coding", "Queued", false},
		{model.JobStatusRunning, "objective_resolve", "Understanding request", true},
		{model.JobStatusRunning, "v3_coding_plan", "Planning", true},
		{model.JobStatusRunning, "v3_coding", "Working", true},
		{model.JobStatusRunning, "unknown", "Working", true},
		{model.JobStatusWaiting, "v3_coding", "Waiting for you", false},
		{model.JobStatusCompleted, "v3_coding", "Done", false},
		{model.JobStatusFailed, "v3_coding", "Failed", false},
		{model.JobStatusCanceled, "v3_coding", "Canceled", false},
	} {
		t.Run(test.status+"/"+test.action, func(t *testing.T) {
			details := renderActivityJob()
			details.Job.Status = test.status
			details.Steps[0].Action = test.action
			activity := activityForJob(details)
			if activity.Label != test.want || activity.Working != test.working {
				t.Fatalf("activity = %#v, want label %q working %t", activity, test.want, test.working)
			}
		})
	}
}

func TestActivityForJobUsesCurrentStepAndPersistedStart(t *testing.T) {
	details := renderActivityJob()
	started := details.Job.CreatedAt.Add(3 * time.Minute)
	superseded := int64(2)
	details.Steps = []model.Step{
		{ID: 10, Generation: 1, Action: "v3_coding_plan", Status: model.StepStatusRunning},
		{ID: 11, Generation: 2, Action: "objective_resolve", Status: model.StepStatusRunning, SupersededAtGeneration: &superseded},
		{ID: 12, Generation: 2, Action: "v3_coding", Status: model.StepStatusRunning, StartedAt: &started},
	}
	activity := activityForJob(details)
	if activity.JobID != 41 || activity.Generation != 2 || activity.StepID != 12 || activity.Label != "Working" || !activity.StartedAt.Equal(started) {
		t.Fatalf("activity selected stale step or invented timing: %#v", activity)
	}
	details.Steps[2].Generation = 0
	details.Steps[2].StartedAt = nil
	activity = activityForJob(details)
	if activity.StepID != 12 || !activity.StartedAt.Equal(details.Job.CreatedAt) {
		t.Fatalf("zero-generation fixture or persisted fallback start lost: %#v", activity)
	}
	details.Job.Status = model.JobStatusWaiting
	details.Steps[2].Status = model.StepStatusWaiting
	activity = activityForJob(details)
	if activity.StepID != 12 || activity.Working || activity.Label != "Waiting for you" {
		t.Fatalf("waiting state kept working animation: %#v", activity)
	}
}

func renderActivityJob() model.JobDetails {
	return model.JobDetails{
		Job: model.Job{ID: 41, CurrentGeneration: 2, Status: model.JobStatusRunning,
			Instruction: "Preserve this task", CreatedAt: time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)},
		Steps: []model.Step{{ID: 12, Generation: 2, Action: "v3_coding", Status: model.StepStatusRunning}},
	}
}
