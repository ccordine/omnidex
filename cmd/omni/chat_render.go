package main

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/queue"
)

type chatRenderer struct {
	console *chatConsole
}

func (renderer chatRenderer) banner(channel model.Channel) error {
	return renderer.console.WriteOutput(fmt.Sprintf(
		"Omnidex chat\nproject: %s\nsession: %s\nType /help for controls. Exit with /exit or Ctrl-D; active server work continues.\n",
		channel.WorkspaceRoot,
		channel.ID,
	))
}

func (renderer chatRenderer) resumed(
	activity int,
	messagesTruncated bool,
	turnsTruncated bool,
	controlsTruncated bool,
) error {
	if activity == 0 && !messagesTruncated && !turnsTruncated && !controlsTruncated {
		return nil
	}
	var rendered strings.Builder
	fmt.Fprintf(&rendered, "[omni] resumed %d persisted session entries\n", activity)
	if messagesTruncated || turnsTruncated || controlsTruncated {
		fmt.Fprintln(&rendered, "[omni] older persisted session history exists outside this bounded view")
	}
	return renderer.console.WriteError(rendered.String())
}

func (renderer chatRenderer) message(message model.ChannelMessage) error {
	var rendered strings.Builder
	label := "you>"
	if message.Role == model.ChannelMessageRoleAssistant {
		label = "omni>"
	}
	renderBlock(&rendered, label, message.Content)
	return renderer.console.WriteOutput(rendered.String())
}

func (renderer chatRenderer) messageTurnState(message model.ChannelMessage) error {
	if message.Turn == nil {
		return fmt.Errorf("job-turn presentation requires persisted turn authority")
	}
	if message.Turn.Status == model.JobStatusCompleted {
		return renderer.system("job %d · completed", message.Turn.JobID)
	}
	var rendered strings.Builder
	fmt.Fprintf(&rendered, "[omni] job %d · %s\n", message.Turn.JobID, message.Turn.Status)
	renderBlock(&rendered, "error>", message.Turn.Error)
	return renderer.console.WriteError(rendered.String())
}

func (renderer chatRenderer) sessionTurn(turn queue.ChannelSessionTurn) error {
	label := "you>"
	switch turn.Disposition {
	case queue.ChannelSessionTurnReplanned:
		label = "you redirect>"
	case queue.ChannelSessionTurnFeedback:
		label = "you follow-up>"
	case queue.ChannelSessionTurnEnqueued:
	default:
		return fmt.Errorf("cannot render session turn disposition %q", turn.Disposition)
	}
	var rendered strings.Builder
	renderBlock(&rendered, label, turn.Text)
	return renderer.console.WriteOutput(rendered.String())
}

func (renderer chatRenderer) sessionControl(control queue.ChannelSessionControl) error {
	label := "you control>"
	switch control.Kind {
	case queue.ChannelSessionControlInterrupt:
		label = "you /interrupt>"
	case queue.ChannelSessionControlReplan:
		label = "you /redirect>"
	case queue.ChannelSessionControlCancel:
		label = "you /cancel>"
	default:
		return fmt.Errorf("cannot render session control kind %q", control.Kind)
	}
	var rendered strings.Builder
	renderBlock(&rendered, label, control.Text)
	return renderer.console.WriteOutput(rendered.String())
}

func (renderer chatRenderer) prompt(details *model.JobDetails) error {
	label := "you> "
	if details != nil {
		label = fmt.Sprintf("you #%d> ", details.Job.ID)
	}
	return renderer.console.SetPrompt(label)
}

func (renderer chatRenderer) system(format string, values ...any) error {
	return renderer.console.WriteError(fmt.Sprintf("[omni] "+format+"\n", values...))
}

func (renderer chatRenderer) job(details model.JobDetails, previous *model.JobDetails) error {
	var rendered strings.Builder
	if previous == nil || previous.Job.Status != details.Job.Status ||
		previous.Job.CurrentGeneration != details.Job.CurrentGeneration {
		fmt.Fprintf(
			&rendered,
			"[omni] job %d · generation %d · %s\n",
			details.Job.ID,
			details.Job.CurrentGeneration,
			details.Job.Status,
		)
	}
	prior := map[int64]string{}
	if previous != nil {
		for _, step := range previous.Steps {
			prior[step.ID] = step.Status
		}
	}
	for _, step := range details.Steps {
		if prior[step.ID] != step.Status {
			fmt.Fprintf(
				&rendered,
				"[omni] job %d · step %d · stage %s · %s\n",
				details.Job.ID,
				step.ID,
				step.Action,
				step.Status,
			)
		}
	}
	if rendered.Len() == 0 {
		return nil
	}
	return renderer.console.WriteError(rendered.String())
}

func (renderer chatRenderer) status(details model.JobDetails) error {
	var rendered strings.Builder
	fmt.Fprintf(
		&rendered,
		"[omni] job %d · generation %d · %s\n",
		details.Job.ID,
		details.Job.CurrentGeneration,
		details.Job.Status,
	)
	for _, step := range details.Steps {
		fmt.Fprintf(
			&rendered,
			"[omni] job %d · step %d · stage %s · %s\n",
			details.Job.ID,
			step.ID,
			step.Action,
			step.Status,
		)
	}
	return renderer.console.WriteError(rendered.String())
}

func (renderer chatRenderer) realtime(event client.RealtimeEvent) error {
	if event.EventName == client.RealtimeJobProgress {
		return renderer.system("job %d · %s", event.JobID, event.Summary)
	}
	if event.FilePath != "" {
		if event.FileOperation == "move" && event.FileSourcePath != "" {
			return renderer.system(
				"job %d · step %d · file move · %q → %q",
				event.JobID,
				event.StepID,
				event.FileSourcePath,
				event.FilePath,
			)
		}
		return renderer.system(
			"job %d · step %d · file %s · %q",
			event.JobID,
			event.StepID,
			event.FileOperation,
			event.FilePath,
		)
	}
	detail := strings.TrimSpace(event.Detail)
	if detail == "" {
		detail = event.RuntimeEvent
	}
	return renderer.system(
		"job %d · step %d · work %s · %s",
		event.JobID,
		event.StepID,
		event.RuntimeEvent,
		detail,
	)
}

func renderBlock(destination *strings.Builder, label, value string) {
	lines := strings.Split(value, "\n")
	if len(lines) == 0 || (len(lines) == 1 && lines[0] == "") {
		fmt.Fprintln(destination, label)
		return
	}
	fmt.Fprintf(destination, "%s %s\n", label, lines[0])
	for _, line := range lines[1:] {
		fmt.Fprintf(destination, "      %s\n", line)
	}
}
