package main

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

type chatRenderer struct {
	console       *chatConsole
	verbosity     chatVerbosity
	agentStream   *chatAgentStream
	planningCalls *chatPlanningCalls
}

func (renderer chatRenderer) banner(channel model.Channel) error {
	if err := renderer.console.setWorkspaceProject(channel.WorkspaceRoot); err != nil {
		return err
	}
	return renderer.console.WriteOutput(fmt.Sprintf(
		"Omnidex · Agents\nproject: %s\n/help for controls · /exit or Ctrl-D to leave\n",
		channel.WorkspaceRoot,
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
	renderBlock(&rendered, statusErrorLabel(message.Turn.Status), message.Turn.Error)
	return renderer.console.WriteError(rendered.String())
}

func (renderer chatRenderer) sessionTurn(turn model.ChannelSessionTurn) error {
	label := "you>"
	switch turn.Disposition {
	case model.ChannelSessionTurnReplanned:
		label = "you redirect>"
	case model.ChannelSessionTurnFeedback:
		label = "you follow-up>"
	case model.ChannelSessionTurnEnqueued:
	case model.ChannelSessionTurnQueued:
	default:
		return fmt.Errorf("cannot render session turn disposition %q", turn.Disposition)
	}
	var rendered strings.Builder
	renderBlock(&rendered, label, turn.Text)
	return renderer.console.WriteOutput(rendered.String())
}

func (renderer chatRenderer) sessionControl(control model.ChannelSessionControl) error {
	label := "you control>"
	switch control.Kind {
	case model.ChannelSessionControlInterrupt:
		label = "you /interrupt>"
	case model.ChannelSessionControlReplan:
		label = "you /redirect>"
	case model.ChannelSessionControlCancel:
		label = "you /cancel>"
	default:
		return fmt.Errorf("cannot render session control kind %q", control.Kind)
	}
	var rendered strings.Builder
	renderBlock(&rendered, label, control.Text)
	return renderer.console.WriteOutput(rendered.String())
}

func (renderer chatRenderer) prompt(details *model.JobDetails) error {
	if err := renderer.console.observeWorkspaceJob(details); err != nil {
		return err
	}
	label := "you> "
	activity := chatActivity{}
	if details != nil {
		label = fmt.Sprintf("you #%d> ", details.Job.ID)
		activity = activityForJob(*details)
	}
	renderer.retainPlanningScope(activity)
	if err := renderer.console.SetActivity(activity); err != nil {
		return err
	}
	return renderer.console.SetPrompt(label)
}

func (renderer chatRenderer) system(format string, values ...any) error {
	return renderer.console.WriteError(fmt.Sprintf("[omni] "+format+"\n", values...))
}

func (renderer chatRenderer) job(details model.JobDetails, previous *model.JobDetails) error {
	var rendered strings.Builder
	if renderer.verbosity == chatDebug {
		renderDiagnosticJob(&rendered, details, previous)
	} else if renderer.verbosity != chatQuiet {
		renderJobActivity(&rendered, details, previous)
	}
	renderJobErrors(&rendered, details, previous)
	if rendered.Len() == 0 {
		return nil
	}
	return renderer.console.WriteError(rendered.String())
}

func (renderer chatRenderer) status(details model.JobDetails) error {
	var rendered strings.Builder
	if renderer.verbosity == chatDebug {
		renderDiagnosticJob(&rendered, details, nil)
	} else {
		fmt.Fprintf(&rendered, "[omni] job %d · %s\n", details.Job.ID, activityForJob(details).Label)
	}
	if details.Job.Instruction != "" {
		renderBlock(&rendered, "task>", details.Job.Instruction)
	}
	renderJobErrors(&rendered, details, nil)
	return renderer.console.WriteError(rendered.String())
}

func (renderer *chatRenderer) realtime(event client.RealtimeEvent) error {
	if err := renderer.console.observeWorkspaceEvent(event); err != nil {
		return err
	}
	if event.PlanningActivity != nil {
		return renderer.planningActivity(event)
	}
	if event.AgentCallActivity != nil {
		return renderer.agentCallActivity(*event.AgentCallActivity)
	}
	if event.AgentActivity != nil {
		return renderer.nativeActivity(event)
	}
	observed := renderer.planningWorkerObserved(event)
	renderer.retirePlanningWorker(event)
	if observed && (renderer.verbosity == chatNormal || renderer.verbosity == chatQuiet) {
		return nil
	}
	renderer.observeWorkspaceFile(event)
	if renderer.verbosity != chatDebug {
		return renderer.realtimeActivity(event)
	}
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
		fmt.Fprintf(destination, "%s%s\n", strings.Repeat(" ", len(label)+1), line)
	}
}
