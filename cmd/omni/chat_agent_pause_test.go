package main

import (
	"strings"
	"testing"

	"github.com/gryph/omnidex/internal/model"
)

func TestTerminalRendersPersistedAgentQuestionsOnResumeAndOncePerPause(t *testing.T) {
	t.Parallel()
	console, output := planReviewTestConsole(t)
	renderer := chatRenderer{console: console}
	details := model.JobDetails{
		Job:   model.Job{ID: 41, CurrentGeneration: 2, Status: model.JobStatusWaiting},
		Steps: []model.Step{{ID: 11, JobID: 41, Action: "v3_coding", Status: model.StepStatusWaiting}},
		AgentPause: &model.CodingAgentPause{
			Schema: "omnidex-coding-agent-pause-v1", Generation: 2, StepID: 11, WaitingAttempt: 3,
			Questions: []string{"Which approved format should this task use?", "Which supplied value should be retained?"},
		},
	}
	if err := renderer.job(details, nil); err != nil {
		t.Fatal(err)
	}
	for _, question := range details.AgentPause.Questions {
		if !strings.Contains(output.String(), question) {
			t.Errorf("terminal did not show persisted question %q: %q", question, output.String())
		}
	}
	before := output.Len()
	if err := renderer.job(details, &details); err != nil {
		t.Fatal(err)
	}
	if output.Len() != before {
		t.Fatalf("unchanged persisted pause repeated terminal output: %q", output.String()[before:])
	}
	if err := renderer.status(details); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String()[before:], details.AgentPause.Questions[0]) {
		t.Fatal("/status omitted the active question")
	}
	next := details
	pause := *details.AgentPause
	pause.WaitingAttempt++
	pause.Questions = []string{"Which next approved detail should this task use?"}
	next.AgentPause = &pause
	before = output.Len()
	if err := renderer.job(next, &details); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String()[before:], pause.Questions[0]) {
		t.Fatal("a later waiting attempt with unchanged step status omitted its question")
	}
}
