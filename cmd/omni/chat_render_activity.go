package main

import (
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/model"
)

func activityForJob(details model.JobDetails) chatActivity {
	activity := chatActivity{
		JobID: details.Job.ID, Generation: details.Job.CurrentGeneration,
		Label: "Working", StartedAt: details.Job.CreatedAt,
	}
	step := currentActivityStep(details)
	if step != nil {
		activity.StepID = step.ID
		if step.StartedAt != nil {
			activity.StartedAt = *step.StartedAt
		}
	}
	switch details.Job.Status {
	case model.JobStatusPending:
		activity.Label = "Queued"
	case model.JobStatusRunning:
		activity.Working = true
		if step != nil {
			switch step.Action {
			case "objective_resolve":
				activity.Label = "Understanding request"
			case "v3_coding_plan":
				activity.Label = "Planning"
			}
		}
	case model.JobStatusWaiting:
		activity.Label = "Waiting for you"
	case model.JobStatusCompleted:
		activity.Label = "Done"
	case model.JobStatusFailed:
		activity.Label = "Failed"
	case model.JobStatusCanceled:
		activity.Label = "Canceled"
	}
	return activity
}

func agentActivityRoleLabel(activity model.AgentActivity) string {
	if activity.Source == model.AgentActivityVerification {
		return "Verifying checks"
	}
	return agentRoleLabel(activity.Role)
}

func agentRoleLabel(role string) string {
	switch role {
	case "plan":
		return "Planning"
	case "execute":
		return "Executing task"
	case "review":
		return "Reviewing"
	case "overall_review":
		return "Overall review"
	default:
		return ""
	}
}

func currentActivityStep(details model.JobDetails) *model.Step {
	var current *model.Step
	for index := range details.Steps {
		step := &details.Steps[index]
		if step.SupersededAtGeneration != nil || step.Generation != 0 && step.Generation != details.Job.CurrentGeneration {
			continue
		}
		if step.Status != model.StepStatusRunning && step.Status != model.StepStatusWaiting {
			continue
		}
		if current == nil || step.SortIndex < current.SortIndex || step.SortIndex == current.SortIndex && step.ID < current.ID {
			current = step
		}
	}
	return current
}

func renderJobActivity(destination *strings.Builder, details model.JobDetails, previous *model.JobDetails) {
	activity := activityForJob(details)
	if previous == nil || previous.Job.ID != details.Job.ID || activityForJob(*previous).Label != activity.Label {
		fmt.Fprintf(destination, "[omni] job %d · %s\n", activity.JobID, activity.Label)
	}
	if (previous == nil || previous.Job.ID != details.Job.ID) && details.Job.Instruction != "" {
		preview := strings.TrimSpace(strings.SplitN(details.Job.Instruction, "\n", 2)[0])
		characters := []rune(preview)
		const previewRunes = 120
		if len(characters) > previewRunes {
			preview = string(characters[:previewRunes-1]) + "…"
		}
		renderBlock(destination, "task>", preview)
	}
}

func renderJobErrors(destination *strings.Builder, details model.JobDetails, previous *model.JobDetails) {
	if details.Job.Error != "" && (previous == nil || previous.Job.Error != details.Job.Error) {
		renderBlock(destination, "error>", details.Job.Error)
	}
	prior := make(map[int64]string)
	if previous != nil {
		for _, step := range previous.Steps {
			prior[step.ID] = step.Error
		}
	}
	for _, step := range details.Steps {
		if step.Error != "" && step.Error != prior[step.ID] && step.Error != details.Job.Error {
			renderBlock(destination, "error>", step.Error)
		}
	}
}

func renderDiagnosticJob(destination *strings.Builder, details model.JobDetails, previous *model.JobDetails) {
	if previous == nil || previous.Job.Status != details.Job.Status || previous.Job.CurrentGeneration != details.Job.CurrentGeneration {
		fmt.Fprintf(destination, "[omni] job %d · generation %d · %s\n", details.Job.ID, details.Job.CurrentGeneration, details.Job.Status)
	}
	prior := make(map[int64]string)
	if previous != nil {
		for _, step := range previous.Steps {
			prior[step.ID] = step.Status
		}
	}
	for _, step := range details.Steps {
		if prior[step.ID] != step.Status {
			fmt.Fprintf(destination, "[omni] job %d · step %d · stage %s · %s\n", details.Job.ID, step.ID, step.Action, step.Status)
		}
	}
}

func (renderer chatRenderer) realtimeActivity(event client.RealtimeEvent) error {
	if event.EventName != client.RealtimeJobRuntimeEvent {
		return nil
	}
	if activity, known := workerActivityForEvent(event); known {
		return renderer.workerActivity(activity)
	}
	switch event.RuntimeEvent {
	case "workspace_directory_read", "workspace_file_read":
		if renderer.verbosity == chatQuiet {
			return nil
		}
		return renderer.system("%s · %q", event.FileOperation, event.FilePath)
	case "workspace_file_changed":
		if renderer.verbosity == chatQuiet {
			return nil
		}
		if event.FileOperation == "move" {
			return renderer.system("file move · %q → %q", event.FileSourcePath, event.FilePath)
		}
		return renderer.system("file %s · %q", event.FileOperation, event.FilePath)
	case "step_failed", "step_authority_lost":
		if strings.TrimSpace(event.Detail) != "" {
			rendered := &strings.Builder{}
			renderBlock(rendered, "error>", event.Detail)
			return renderer.console.WriteError(rendered.String())
		}
	}
	return nil
}
