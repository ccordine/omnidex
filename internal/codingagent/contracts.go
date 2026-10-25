// Package codingagent defines the portable boundary between Omnidex task
// authority and replaceable agents. Agents propose results; code owns acceptance.
package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/gryph/omnidex/internal/model"
)

type Mode string

const (
	Agents Mode = "agents"
)

type Role string

const (
	Plan              Role = "plan"
	PlanningInitial   Role = "planning_initial"
	PlanningMutation  Role = "planning_mutation"
	Execute           Role = "execute"
	ExecutionResponse Role = "execution_response"
	Review            Role = "review"
	ReviewResponse    Role = "review_response"
)

func (role Role) IsPlanning() bool {
	switch role {
	case Plan, PlanningInitial, PlanningMutation:
		return true
	default:
		return false
	}
}

func (role Role) IsNative() bool { return role == Plan || role == Execute || role == Review }

type Route struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Effort      string `json:"effort,omitempty"`
	ServiceTier string `json:"service_tier,omitempty"`
}
type Configuration struct {
	Mode         Mode  `json:"mode"`
	Planner      Route `json:"planner"`
	LedgerParser Route `json:"ledger_parser"`
	Executor     Route `json:"executor"`
	Reviewer     Route `json:"reviewer"`
}

func (config Configuration) Validate() error {
	if config.Mode != Agents {
		return fmt.Errorf("unknown coding mode %q", config.Mode)
	}
	if config.LedgerParser != (Route{}) {
		if err := validateRoute(config.LedgerParser); err != nil {
			return fmt.Errorf("response parser route: %w", err)
		}
	}
	return (RouteSet{Planner: config.Planner, Executor: config.Executor, Reviewer: config.Reviewer}).Validate()
}

type Task struct {
	Objective            string                         `json:"objective,omitempty"`
	OwnerContext         []model.TaskContext            `json:"owner_context,omitempty"`
	Constraints          []string                       `json:"constraints,omitempty"`
	FileResponsibilities []model.TaskFileResponsibility `json:"file_responsibilities,omitempty"`
	Acceptance           []model.TaskAcceptance         `json:"acceptance,omitempty"`
	ID                   string                         `json:"id"`
	Goal                 string                         `json:"goal"`
	ReadFiles            []string                       `json:"read_files"`
	WriteFiles           []string                       `json:"write_files"`
	DeleteFiles          []string                       `json:"delete_files"`
}

// LedgerTask maps the portable task back to its single authoritative payload shape.
func (task Task) LedgerTask() model.LedgerTask {
	return model.LedgerTask{LedgerItem: model.LedgerItem{ID: model.LedgerItemID(task.ID)}, Goal: task.Goal,
		Objective: task.Objective, OwnerContext: task.OwnerContext, Constraints: task.Constraints,
		FileResponsibilities: task.FileResponsibilities, Acceptance: task.Acceptance,
		ReadFiles: task.ReadFiles, WriteFiles: task.WriteFiles, DeleteFiles: task.DeleteFiles}
}
func (task Task) Instructions() string { return task.LedgerTask().Instructions() }

type Objective struct {
	Instruction    string   `json:"instruction"`
	Clarifications []string `json:"clarifications,omitempty"`
}
type ConstructionFact struct {
	Kind  string `json:"kind"`
	Quote string `json:"quote"`
}
type Finding struct {
	TaskID      string `json:"task_id"`
	File        string `json:"file,omitempty"`
	Observation string `json:"observation"`
}
type ReviewResult struct {
	Findings     []Finding `json:"findings"`
	Verification string    `json:"verification,omitempty"`
}

// ReviewResponseResult is extracted by the configured parser, never authored by the
// native reviewer as a transport contract. Code owns the resulting transition.
type ReviewResponseResult struct {
	Kind ExecutionResponseKind `json:"kind"`
	ReviewResult
}

// ExecutionReport retains task-local progress for continuation and the completed
// execution's original native evidence for independent review.
type ExecutionReport struct {
	CallID   int64  `json:"call_id"`
	Round    int64  `json:"round"`
	Response string `json:"response"`
}

type Assignment struct {
	ExecutionReport      *ExecutionReport    `json:"execution_report,omitempty"`
	JobID                int64               `json:"job_id"`
	Generation           int64               `json:"generation"`
	Role                 Role                `json:"role"`
	Task                 *Task               `json:"task,omitempty"`
	Objective            *Objective          `json:"objective,omitempty"`
	ConstructionFacts    []ConstructionFact  `json:"construction_facts,omitempty"`
	Findings             []Finding           `json:"findings,omitempty"`
	ValidationNotes      []string            `json:"validation_notes,omitempty"`
	Root                 string              `json:"root"`
	ReadFiles            []string            `json:"read_files"`
	WriteFiles           []string            `json:"write_files"`
	DeleteFiles          []string            `json:"delete_files"`
	Route                Route               `json:"route"`
	PlanningQuestionID   string              `json:"planning_question_id,omitempty"`
	PlanningBaseRevision int64               `json:"planning_base_revision,omitempty"`
	PlanningLedger       *EngineeringLedger  `json:"planning_ledger,omitempty"`
	PlanningConversation []PlanningMessage   `json:"planning_conversation,omitempty"`
	NewerOwnerDirection  *PlanningDirection  `json:"newer_owner_direction,omitempty"`
	PlanningResponse     string              `json:"planning_response,omitempty"`
	Response             string              `json:"response,omitempty"`
	Session              *ExecutionSession   `json:"session,omitempty"`
	Reply                string              `json:"reply,omitempty"`
	Context              string              `json:"context,omitempty"`
	Continuation         *TaskContinuation   `json:"continuation,omitempty"`
	Clarifications       []TaskClarification `json:"clarifications,omitempty"`
	CompletedTaskIDs     []string            `json:"completed_task_ids,omitempty"`
}

type PlanningMessage struct {
	Role string `json:"role"`
	Text string `json:"text"`
}
type Result struct {
	CallID    int64             `json:"call_id,omitempty"`
	Provider  string            `json:"provider"`
	Model     string            `json:"model"`
	ThreadID  string            `json:"thread_id"`
	TurnID    string            `json:"turn_id"`
	Completed bool              `json:"completed"`
	Content   json.RawMessage   `json:"content"`
	Evidence  []json.RawMessage `json:"evidence"`
}

func (result Result) ValidateFor(role Role, route Route) error {
	if result.Provider != route.Provider || result.Model != route.Model || !result.Completed || !json.Valid(result.Content) {
		return fmt.Errorf("provider returned an unbound or incomplete result")
	}
	if role.IsNative() && (result.ThreadID == "" || result.TurnID == "") {
		return fmt.Errorf("native execution requires its original thread and turn identities")
	}
	return nil
}

type Provider interface {
	Run(context.Context, Assignment, func(NativeEvent) error) (Result, error)
}

// NativeEvent preserves the original journal receipt separately from its
// validated public projection. An activity-only event is never a new receipt.
type NativeEvent struct {
	Frame    json.RawMessage
	Activity *model.AgentActivity
	Session  *ExecutionSession
	Input    *NativeInput
}

func ValidatePath(value string) error {
	if value == "" || value == "." || path.IsAbs(value) || path.Clean(value) != value || strings.ContainsAny(value, "\\\x00\r\n") || value == ".." || strings.HasPrefix(value, "../") {
		return fmt.Errorf("agent file must be one exact relative path: %q", value)
	}
	for _, part := range strings.Split(value, "/") {
		if part == ".git" || part == ".codex" || part == ".env" || strings.HasPrefix(part, ".env.") || part == "node_modules" || part == ".agent-cache" || part == "bin" {
			return fmt.Errorf("agent scope excludes credentials, authority and generated resources")
		}
	}
	return nil
}
