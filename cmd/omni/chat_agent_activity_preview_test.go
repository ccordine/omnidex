package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestAgentCommandTruncationIsExplicitForNativeAndVerification(t *testing.T) {
	for _, source := range []model.AgentActivitySource{model.AgentActivityNative, model.AgentActivityVerification} {
		for _, status := range []model.AgentActivityStatus{model.AgentActivityStarted, model.AgentActivityCompleted, model.AgentActivityFailed, model.AgentActivityDeclined} {
			t.Run(string(source)+"/"+string(status), func(t *testing.T) {
				console, output := planReviewTestConsole(t)
				renderer := chatRenderer{console: console}
				if status == model.AgentActivityFailed || status == model.AgentActivityDeclined {
					renderer.verbosity = chatQuiet
				}
				activity := nativeRenderActivity(model.AgentActivityCommand, status, "")
				activity.Source, activity.Command, activity.CWD, activity.Truncated = source, "command-preview", "/client/project", true
				if source == model.AgentActivityVerification {
					activity.Command, activity.Argv, activity.CheckID = "", []string{"command-preview", "argument-preview"}, "check-preview"
					activity.ThreadID, activity.TurnID, activity.ItemID = "", "", ""
				}
				if status == model.AgentActivityCompleted {
					exit := 0
					activity.ExitCode = &exit
				}
				if err := activity.Validate(); err != nil {
					t.Fatal(err)
				}
				if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(output.String(), "command-preview") || strings.Count(output.String(), "Command preview truncated.") != 1 {
					t.Fatalf("clipped command looked complete for %s/%s: %q", source, status, output.String())
				}
			})
		}
	}
}
