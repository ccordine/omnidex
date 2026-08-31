package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

type chatRenderer struct {
	out io.Writer
	err io.Writer
}

func (renderer chatRenderer) banner(channel model.Channel) {
	fmt.Fprintf(renderer.out, "Omnidex chat\nproject: %s\nsession: %s\n", channel.WorkspaceRoot, channel.ID)
	fmt.Fprintln(renderer.out, "Type /help for controls. Exit with /exit or Ctrl-D; active server work continues.")
}

func (renderer chatRenderer) transcript(messages []model.ChannelMessage) {
	if len(messages) == 0 {
		return
	}
	fmt.Fprintf(renderer.out, "resumed %d persisted messages\n", len(messages))
	for _, message := range messages {
		label := "you"
		if message.Role == model.ChannelMessageRoleAssistant {
			label = "omni"
		}
		renderBlock(renderer.out, label+">", message.Content)
	}
}

func (renderer chatRenderer) prompt(details *model.JobDetails) {
	label := "you> "
	if details != nil {
		label = fmt.Sprintf("redirect #%d> ", details.Job.ID)
		if details.Job.Status == model.JobStatusWaiting && !interruptedBoundary(details.Steps) {
			label = fmt.Sprintf("feedback #%d> ", details.Job.ID)
		}
	}
	fmt.Fprint(renderer.out, label)
}

func (renderer chatRenderer) system(format string, values ...any) {
	fmt.Fprintf(renderer.err, "\n[omni] "+format+"\n", values...)
}

func (renderer chatRenderer) job(details model.JobDetails, previous *model.JobDetails) {
	if previous == nil || previous.Job.Status != details.Job.Status ||
		previous.Job.CurrentGeneration != details.Job.CurrentGeneration {
		renderer.system("job %d · generation %d · %s", details.Job.ID, details.Job.CurrentGeneration, details.Job.Status)
	}
	prior := map[int64]string{}
	if previous != nil {
		for _, step := range previous.Steps {
			prior[step.ID] = step.Status
		}
	}
	for _, step := range details.Steps {
		if prior[step.ID] != step.Status {
			renderer.system("stage %s · %s", step.Action, step.Status)
		}
	}
}

func (renderer chatRenderer) realtime(event client.RealtimeEvent) {
	if event.EventName == client.RealtimeJobProgress {
		renderer.system("job %d · %s", event.JobID, event.Summary)
		return
	}
	if event.FilePath != "" {
		renderer.system("file %s · %s", event.FileOperation, event.FilePath)
		return
	}
	detail := strings.TrimSpace(event.Detail)
	if detail == "" {
		detail = event.RuntimeEvent
	}
	renderer.system("work %s · %s", event.RuntimeEvent, detail)
}

func (renderer chatRenderer) terminal(details model.JobDetails) {
	switch details.Job.Status {
	case model.JobStatusCompleted:
		if strings.TrimSpace(details.Job.Result) != "" {
			renderBlock(renderer.out, "omni>", details.Job.Result)
		}
	case model.JobStatusFailed, model.JobStatusCanceled:
		renderBlock(renderer.err, "error>", details.Job.Error)
	}
}

func renderBlock(destination io.Writer, label, value string) {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		fmt.Fprintln(destination, label)
		return
	}
	fmt.Fprintf(destination, "%s %s\n", label, lines[0])
	for _, line := range lines[1:] {
		fmt.Fprintf(destination, "      %s\n", line)
	}
}
