package main

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gryph/omnidex/internal/model"
)

func TestPlannerLongPartialTextKeepsShowingNewestPublicFragment(t *testing.T) {
	console, output := planReviewTestConsole(t)
	if err := console.SetActivity(chatActivity{JobID: 41, StepID: 12, Label: "Planning", Working: true}); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	renderer := chatRenderer{console: console}
	activity := nativeRenderActivity(model.AgentActivityMessage, model.AgentActivityDelta, strings.Repeat("界", 140))
	activity.Role, activity.TaskID = "plan", ""
	if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
		t.Fatal(err)
	}
	before := console.activityDetail
	activity.Text = " 最新の変更"
	if err := renderer.realtime(nativeRenderEvent(activity)); err != nil {
		t.Fatal(err)
	}
	if console.activityDetail == before || !strings.HasSuffix(console.activityDetail, activity.Text) || !utf8.ValidString(console.activityDetail) {
		t.Fatalf("long planner partial stopped showing live text: before %q, after %q", before, console.activityDetail)
	}
	if strings.Contains(output.String(), "omni>") {
		t.Fatalf("partial updates printed transcript lines before newline/completion: %q", output.String())
	}
}
