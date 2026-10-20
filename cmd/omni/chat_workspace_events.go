package main

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/client"
	"github.com/gryph/omnidex/internal/codingagent"
	"github.com/gryph/omnidex/internal/model"
)

func (console *chatConsole) setWorkspaceProject(root string) error {
	if console.workspace == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	command := exec.CommandContext(ctx, "git", "symbolic-ref", "--quiet", "--short", "HEAD")
	command.Dir = root
	branch, err := command.Output()
	if err != nil {
		branch = nil
	} // A non-Git directory has no branch to display.
	return console.workspace.SetProject(root, strings.TrimSpace(string(branch)))
}

func (console *chatConsole) observeWorkspaceEvent(event client.RealtimeEvent) error {
	if console.workspace == nil {
		return nil
	}
	if activity := event.AgentCallActivity; activity != nil {
		text := fmt.Sprintf("%s · %s", agentRoleLabel(activity.Role), activity.Status)
		return console.workspace.observeRole(activity.Role, text)
	}
	if activity := event.AgentActivity; activity != nil {
		text := ""
		switch activity.Kind {
		case model.AgentActivityCommand:
			text = fmt.Sprintf("%s · %s\n%s", agentActivityRoleLabel(*activity), activity.Status, activity.Command)
			if activity.ExitCode != nil {
				text += fmt.Sprintf("\nexit %d", *activity.ExitCode)
			}
		case model.AgentActivityFileEdit:
			text = string(activity.Status) + "\n" + strings.Join(activity.Paths, "\n")
		}
		return console.workspace.observeRole(activity.Role, text)
	}
	if activity := event.PlanningActivity; activity != nil {
		return console.workspace.Observe("Planning", activity.Model, activity.Question)
	}
	if activity, ok := workerActivityForEvent(event); ok {
		return console.workspace.Observe(activity.action, activity.model, activity.action+" · "+activity.status)
	}
	return nil
}

func (console *chatConsole) observeWorkspaceJob(details *model.JobDetails) error {
	if console.workspace == nil || details == nil {
		return nil
	}
	configuration, err := codingagent.ConfigurationFromMetadata(details.Job.Metadata)
	if err != nil || configuration.Mode == "" {
		return nil
	} // Non-coding jobs do not have native coding routes.
	view := console.workspace
	view.mu.Lock()
	defer view.mu.Unlock()
	if view.routeJob != details.Job.ID {
		view.role, view.model = "", ""
		view.routeJob = details.Job.ID
	}
	view.routes = map[string]string{}
	for role, route := range map[string]codingagent.Route{"plan": configuration.Planner, "execute": configuration.Executor, "review": configuration.Reviewer, "planning_initial": configuration.LedgerParser, "planning_mutation": configuration.LedgerParser, "execution_response": configuration.LedgerParser, "review_response": configuration.LedgerParser} {
		view.routes[role] = safeConsoleText(route.Model + " · " + route.Effort)
	}
	return view.drawLocked()
}

func (view *chatWorkspace) observeRole(role, text string) error {
	view.mu.Lock()
	route := view.routes[role]
	view.mu.Unlock()
	return view.Observe(agentRoleLabel(role), route, text)
}
