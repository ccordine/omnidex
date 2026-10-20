package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func TestNativeAgentMessageDeltasComposeAndCompletionAppends(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	for _, activity := range []model.AgentActivity{
		nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityDelta, "Inspect "),
		nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityDelta, "the settings.\nNext "),
		nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityCompleted, "check permissions."),
	} {
		if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
			t.Fatal(err)
		}
	}
	for _, required := range []string{"omni> Inspect the settings.", "      Next check permissions."} {
		if strings.Count(output.String(), required) != 1 {
			t.Errorf("composed native message omitted or repeated %q: %q", required, output.String())
		}
	}
	if strings.Contains(output.String(), "agent_activity") {
		t.Fatalf("native activity exposed transport diagnostics: %q", output.String())
	}
}

func TestNativeAgentPartialTextIsLiveWithoutTokenLineFlood(t *testing.T) {
	console, output := planReviewTestConsole(t)
	if err := console.SetActivity(chatActivity{JobID: 41, StepID: 12, Label: "Working", Working: true}); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	renderer := chatRenderer{console: console}
	for _, fragment := range []string{"Inspect", "ing", " settings"} {
		if err := renderer.realtime(nativeRenderEvent(nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityDelta, fragment))); err != nil {
			t.Fatal(err)
		}
	}
	if console.activityDetail != "Executing task · Inspecting settings" || strings.Contains(output.String(), "omni>") {
		t.Fatalf("partial text was hidden or printed per token: detail %q, output %q", console.activityDetail, output.String())
	}
}

func TestNativeAgentVerbositySeparatesPublicSummaryAndOutput(t *testing.T) {
	for _, verbosity := range []chatVerbosity{chatQuiet, chatNormal, chatVerbose, chatDebug} {
		t.Run(verbosity.String(), func(t *testing.T) {
			console, output := planReviewTestConsole(t)
			renderer := chatRenderer{console: console, verbosity: verbosity}
			for _, activity := range []model.AgentActivity{
				nativeRenderActivity(model.AgentActivityReasoningSummary, model.AgentActivityDelta, "Checking permissions\n"),
				nativeRenderActivity(model.AgentActivityReasoningSummary, model.AgentActivityCompleted, "Public summary detail."),
				nativeRenderActivity(model.AgentActivityCommandOutput, model.AgentActivityDelta, "actual stdout line\n"),
			} {
				if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
					t.Fatal(err)
				}
			}
			if strings.Contains(output.String(), "Checking permissions") != (verbosity != chatQuiet) {
				t.Errorf("summary heading visibility does not follow %s: %q", verbosity, output.String())
			}
			wantDetails := verbosity == chatVerbose || verbosity == chatDebug
			if strings.Contains(output.String(), "Public summary detail.") != wantDetails || strings.Contains(output.String(), "actual stdout line") != wantDetails {
				t.Errorf("public detail visibility does not follow %s: %q", verbosity, output.String())
			}
		})
	}
}

func TestNativeAgentCommandShowsObservedStartExitAndFailure(t *testing.T) {
	console, output := planReviewTestConsole(t)
	if err := console.SetActivity(chatActivity{JobID: 41, StepID: 12, Label: "Working", Working: true}); err != nil {
		t.Fatal(err)
	}
	renderer := chatRenderer{console: console}
	started := nativeRenderActivity(model.AgentActivityCommand, model.AgentActivityStarted, "")
	started.Command, started.CWD = "go test ./cmd/omni", "/client/project"
	if err := renderer.realtime(nativeRenderEvent(started)); err != nil {
		t.Fatal(err)
	}
	if console.activityDetail != "Running: go test ./cmd/omni" {
		t.Fatalf("running command was not live: %q", console.activityDetail)
	}
	completed, exit := started, 0
	completed.Status, completed.ExitCode = model.AgentActivityCompleted, &exit
	if err := renderer.realtime(nativeRenderEvent(completed)); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"go test ./cmd/omni", "completed", "exit 0"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("native command presentation omitted %q: %q", required, output.String())
		}
	}
	output.Reset()
	renderer.verbosity = chatQuiet
	failed := started
	failed.Status, exit, failed.ExitCode = model.AgentActivityFailed, 7, &exit
	if err := renderer.realtime(nativeRenderEvent(failed)); err != nil {
		t.Fatal(err)
	}
	declined := started
	declined.Status = model.AgentActivityDeclined
	if err := renderer.realtime(nativeRenderEvent(declined)); err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"failed", "exit 7", "declined"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("quiet mode hid command error %q: %q", required, output.String())
		}
	}
}

func TestNativeAgentFilesAreObservedAndQuietSuppressesRoutineActivity(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	for _, kind := range []model.AgentActivityKind{model.AgentActivityFileRead, model.AgentActivityFileEdit} {
		activity := nativeRenderActivity(kind, model.AgentActivityCompleted, "")
		activity.Paths = []string{"actual.go"}
		if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
			t.Fatal(err)
		}
	}
	if strings.Count(output.String(), "actual.go") != 2 || !strings.Contains(output.String(), "read") || !strings.Contains(output.String(), "edit") {
		t.Fatalf("observed file activity was omitted: %q", output.String())
	}
	output.Reset()
	renderer.verbosity = chatQuiet
	if err := renderer.realtime(nativeRenderEvent(nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityCompleted, "routine message"))); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("quiet mode displayed routine native activity: %q", output.String())
	}
}

func TestNativeAgentTextPreservesPublicBytesAndEscapesTerminalControls(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console, verbosity: chatVerbose}
	text := strings.Repeat("界", 2000) + "\x1b[31m unsafe"
	for start := 0; start < len(text); {
		end := min(start+3000, len(text))
		if err := renderer.realtime(nativeRenderEvent(nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityDelta, text[start:end]))); err != nil {
			t.Fatal(err)
		}
		start = end
	}
	if err := renderer.realtime(nativeRenderEvent(nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityCompleted, " remainder"))); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), "界") != 2000 || !strings.Contains(output.String(), `\u001b[31m unsafe remainder`) || strings.Contains(output.String(), "\x1b[31m") {
		t.Fatalf("bounded public text lost bytes or terminal safety: %q", output.String())
	}
}

func TestNativeAgentEmptyCompletionFlushesFullyStreamedText(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	for _, activity := range []model.AgentActivity{
		nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityDelta, "fully streamed message"),
		nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityCompleted, ""),
	} {
		if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(output.String(), "omni> fully streamed message") || len(renderer.agentStream.items) != 0 {
		t.Fatalf("completed native item retained unfinished text: %q", output.String())
	}
}

func TestNativeAgentCommandCompletionFlushesPendingOutputAndClearsDetail(t *testing.T) {
	console, output := planReviewTestConsole(t)
	if err := console.SetActivity(chatActivity{JobID: 41, StepID: 12, Label: "Working", Working: true}); err != nil {
		t.Fatal(err)
	}
	renderer := chatRenderer{console: console, verbosity: chatVerbose}
	if err := renderer.realtime(nativeRenderEvent(nativeRenderActivity(model.AgentActivityCommandOutput, model.AgentActivityDelta, "pending stdout"))); err != nil {
		t.Fatal(err)
	}
	if console.activityDetail != "Executing task · Output: pending stdout" {
		t.Fatalf("command partial output was not live: %q", console.activityDetail)
	}
	completed := nativeRenderActivity(model.AgentActivityCommand, model.AgentActivityCompleted, "")
	completed.Command, completed.CWD = "printf output", "/client/project"
	if err := renderer.realtime(nativeRenderEvent(completed)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "output> pending stdout") || console.activityDetail != "Executing task" || len(renderer.agentStream.items) != 0 {
		t.Fatalf("command completion lost stdout or retained stale detail: %q, detail %q", output.String(), console.activityDetail)
	}
}

func TestNativeAgentConcurrentItemsAndTurnChangesKeepTextSeparate(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	first := nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityDelta, "first ")
	second := first
	second.ThreadID, second.Text = "thread-2", "second "
	for _, activity := range []model.AgentActivity{first, second} {
		if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
			t.Fatal(err)
		}
	}
	first.TurnID, first.Status, first.Text = "turn-2", model.AgentActivityCompleted, "next turn"
	second.Status, second.Text = model.AgentActivityCompleted, "thread"
	for _, activity := range []model.AgentActivity{first, second} {
		if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
			t.Fatal(err)
		}
	}
	for _, required := range []string{"omni> first ", "omni> next turn", "omni> second thread"} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("turn/thread projection lost %q: %q", required, output.String())
		}
	}
	if strings.Contains(output.String(), "first next turn") || len(renderer.agentStream.items) != 0 {
		t.Fatalf("native turn identities merged unfinished fragments: %q", output.String())
	}
}

func TestVerificationActivityKeepsIndependentCheckOutputSeparate(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console, verbosity: chatVerbose}
	first := nativeRenderActivity(model.AgentActivityCommandOutput, model.AgentActivityDelta, "first check output")
	first.Source, first.CheckID = model.AgentActivityVerification, "check-1"
	first.ThreadID, first.TurnID, first.ItemID = "", "", ""
	second := first
	second.CheckID, second.Text = "check-2", "second check output"
	for _, activity := range []model.AgentActivity{first, second} {
		if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
			t.Fatal(err)
		}
	}
	for _, activity := range []model.AgentActivity{first, second} {
		activity.Kind, activity.Status, activity.Text = model.AgentActivityCommand, model.AgentActivityCompleted, ""
		activity.Argv, activity.CWD = []string{"check", activity.CheckID, "argument with spaces"}, "/client/project"
		exit := 0
		activity.ExitCode = &exit
		if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
			t.Fatal(err)
		}
	}
	for _, required := range []string{"output> first check output", "output> second check output", `"argument with spaces"`} {
		if !strings.Contains(output.String(), required) {
			t.Errorf("independent check lost its own output %q: %q", required, output.String())
		}
	}
	if strings.Contains(output.String(), "first check outputsecond") || len(renderer.agentStream.items) != 0 {
		t.Fatalf("independent checks shared native-item state: %q", output.String())
	}
}

func TestNativeFileEditDoesNotRepeatObservedWorkspaceChange(t *testing.T) {
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	if err := renderer.realtime(client.RealtimeEvent{EventName: client.RealtimeJobRuntimeEvent, JobID: 41, StepID: 12,
		RuntimeEvent: "workspace_file_changed", FileOperation: "replace", FilePath: "actual.go"}); err != nil {
		t.Fatal(err)
	}
	activity := nativeRenderActivity(model.AgentActivityFileEdit, model.AgentActivityCompleted, "")
	activity.Paths = []string{"actual.go"}
	if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
		t.Fatal(err)
	}
	if strings.Count(output.String(), "actual.go") != 1 || !strings.Contains(output.String(), "file replace") {
		t.Fatalf("native file echo duplicated actual workspace publication: %q", output.String())
	}
}

func nativeRenderActivity(kind model.AgentActivityKind, status model.AgentActivityStatus, text string) model.AgentActivity {
	return model.AgentActivity{Schema: 1, Source: model.AgentActivityNative, Kind: kind, Status: status, Text: text,
		ThreadID: "thread-1", TurnID: "turn-1", ItemID: "item-1", TaskID: "task-1", Role: "execute"}
}

func nativeRenderEvent(activity model.AgentActivity) client.RealtimeEvent {
	return client.RealtimeEvent{EventName: client.RealtimeJobRuntimeEvent, JobID: 41, StepID: 12,
		RuntimeEvent: "agent_activity", AgentActivity: &activity}
}
