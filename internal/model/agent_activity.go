package model

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

type AgentActivityKind string
type AgentActivityStatus string
type AgentActivitySource string

const (
	AgentActivityNative       AgentActivitySource = "native"
	AgentActivityVerification AgentActivitySource = "verification"

	AgentActivityMessage          AgentActivityKind = "message"
	AgentActivityReasoningSummary AgentActivityKind = "reasoning_summary"
	AgentActivityCommand          AgentActivityKind = "command"
	AgentActivityCommandOutput    AgentActivityKind = "command_output"
	AgentActivityFileRead         AgentActivityKind = "file_read"
	AgentActivityFileEdit         AgentActivityKind = "file_edit"

	AgentActivityStarted   AgentActivityStatus = "started"
	AgentActivityDelta     AgentActivityStatus = "delta"
	AgentActivityCompleted AgentActivityStatus = "completed"
	AgentActivityFailed    AgentActivityStatus = "failed"
	AgentActivityDeclined  AgentActivityStatus = "declined"

	MaxAgentActivityTextBytes = 4096
)

// AgentActivity is public, task-bound execution evidence. It carries observed
// text and command/file metadata, without private reasoning or execution authority.
type AgentActivity struct {
	Schema    int                 `json:"schema"`
	Source    AgentActivitySource `json:"source"`
	Kind      AgentActivityKind   `json:"kind"`
	Status    AgentActivityStatus `json:"status"`
	ThreadID  string              `json:"threadID,omitempty"`
	TurnID    string              `json:"turnID,omitempty"`
	ItemID    string              `json:"itemID,omitempty"`
	CheckID   string              `json:"checkID,omitempty"`
	TaskID    string              `json:"taskID,omitempty"`
	Role      string              `json:"role"`
	Text      string              `json:"text,omitempty"`
	Command   string              `json:"command,omitempty"`
	Argv      []string            `json:"argv,omitempty"`
	CWD       string              `json:"cwd,omitempty"`
	Paths     []string            `json:"paths,omitempty"`
	ExitCode  *int                `json:"exitCode,omitempty"`
	Truncated bool                `json:"truncated,omitempty"`
}

func (activity AgentActivity) Validate() error {
	if activity.Schema != 1 {
		return fmt.Errorf("agent activity has unsupported schema %d", activity.Schema)
	}
	switch activity.Source {
	case AgentActivityNative:
		if activity.CheckID != "" || len(activity.Argv) != 0 {
			return fmt.Errorf("native activity carries verification fields")
		}
		for _, identity := range []string{activity.ThreadID, activity.TurnID, activity.ItemID} {
			if identity == "" || !activityText(identity, 256) || strings.TrimSpace(identity) != identity {
				return fmt.Errorf("agent activity requires exact native identities")
			}
		}
	case AgentActivityVerification:
		if activity.ThreadID != "" || activity.TurnID != "" || activity.ItemID != "" || activity.CheckID == "" || !activityText(activity.CheckID, 256) || strings.TrimSpace(activity.CheckID) != activity.CheckID {
			return fmt.Errorf("verification activity requires its actual check identity without native identities")
		}
		if activity.Kind != AgentActivityCommand && activity.Kind != AgentActivityCommandOutput {
			return fmt.Errorf("verification activity cannot carry native message or file projections")
		}
	default:
		return fmt.Errorf("agent activity has unregistered source %q", activity.Source)
	}
	switch activity.Role {
	case "plan", "planning_initial", "planning_mutation":
		if activity.TaskID != "" {
			return fmt.Errorf("global agent activity cannot carry a task identity")
		}
	case "execute", "execution_response", "review", "review_response":
		if activity.TaskID == "" || !activityText(activity.TaskID, 256) || strings.TrimSpace(activity.TaskID) != activity.TaskID {
			return fmt.Errorf("task agent activity lacks its exact task identity")
		}
	default:
		return fmt.Errorf("agent activity has unregistered role %q", activity.Role)
	}
	if !activityText(activity.Text, MaxAgentActivityTextBytes) || !activityText(activity.Command, 4096) || !activityText(activity.CWD, 4096) || len(activity.Paths) > 8 {
		return fmt.Errorf("agent activity exceeds its UTF-8 field bounds")
	}
	for _, file := range activity.Paths {
		if file == "" || !activityText(file, 4096) || strings.TrimSpace(file) != file {
			return fmt.Errorf("agent activity carries an invalid observed path")
		}
	}
	switch activity.Kind {
	case AgentActivityMessage, AgentActivityReasoningSummary:
		if (activity.Status != AgentActivityDelta && activity.Status != AgentActivityCompleted) || activity.Status == AgentActivityDelta && activity.Text == "" {
			return fmt.Errorf("agent text activity requires public text and delta or completion status")
		}
	case AgentActivityCommandOutput:
		if activity.Status != AgentActivityDelta || activity.Text == "" {
			return fmt.Errorf("command output requires a nonempty ordered delta")
		}
	case AgentActivityCommand:
		if activity.CWD == "" || activity.Text != "" || len(activity.Paths) != 0 {
			return fmt.Errorf("command activity lacks its observed command and directory")
		}
		if activity.Source == AgentActivityNative && activity.Command == "" {
			return fmt.Errorf("native command lacks its exact observed command")
		}
		if activity.Source == AgentActivityVerification {
			if activity.Command != "" || len(activity.Argv) == 0 || len(activity.Argv) > 128 || activity.Argv[0] == "" {
				return fmt.Errorf("verification command requires its exact argv")
			}
			for _, argument := range activity.Argv {
				if !activityText(argument, 4096) {
					return fmt.Errorf("verification argv exceeds its UTF-8 boundary")
				}
			}
			if activity.Status == AgentActivityCompleted && activity.ExitCode == nil {
				return fmt.Errorf("completed verification lacks its actual exit status")
			}
		}
		switch activity.Status {
		case AgentActivityStarted, AgentActivityCompleted, AgentActivityFailed, AgentActivityDeclined:
		default:
			return fmt.Errorf("command activity has invalid status %q", activity.Status)
		}
	case AgentActivityFileRead, AgentActivityFileEdit:
		if activity.Status != AgentActivityCompleted || len(activity.Paths) == 0 || activity.Text != "" {
			return fmt.Errorf("file activity requires completed observed paths")
		}
	default:
		return fmt.Errorf("agent activity has unregistered kind %q", activity.Kind)
	}
	if activity.Kind != AgentActivityCommand && (activity.Command != "" || len(activity.Argv) != 0 || activity.CWD != "" || activity.ExitCode != nil) {
		return fmt.Errorf("non-command activity carries command fields")
	}
	if activity.Kind != AgentActivityFileRead && activity.Kind != AgentActivityFileEdit && len(activity.Paths) != 0 {
		return fmt.Errorf("non-file activity carries paths")
	}
	if activity.Status == AgentActivityStarted && activity.ExitCode != nil {
		return fmt.Errorf("running command cannot carry an exit code")
	}
	encoded, err := json.Marshal(activity)
	if err != nil || len(encoded) > 48*1024 {
		return fmt.Errorf("agent activity exceeds its encoded transport bound")
	}
	return nil
}

func activityText(value string, limit int) bool {
	return len(value) <= limit && utf8.ValidString(value) && !strings.ContainsRune(value, '\x00')
}
