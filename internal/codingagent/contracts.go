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
	ID                     string         `json:"id"`
	Goal                   string         `json:"goal"`
	Guidance               []TaskGuidance `json:"guidance"`
	KnownAffectedSystems   []string       `json:"known_affected_systems"`
	Acceptance             []string       `json:"acceptance"`
	ReadFiles              []string       `json:"read_files"`
	WriteFiles             []string       `json:"write_files"`
	DeleteFiles            []string       `json:"delete_files"`
	Dependencies           []string       `json:"dependencies"`
	RequirementIDs         []string       `json:"requirement_ids"`
	AcceptanceCriterionIDs []string       `json:"acceptance_criterion_ids,omitempty"`
	Checks                 []Check        `json:"checks"`
}

type TaskGuidance struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Text string `json:"text"`
}
type Check struct {
	ID                     string   `json:"id"`
	Argv                   []string `json:"argv"`
	RequirementIDs         []string `json:"requirement_ids,omitempty"`
	AcceptanceCriterionIDs []string `json:"acceptance_criterion_ids,omitempty"`
}

type Requirement struct {
	ID      string `json:"id"`
	Outcome string `json:"outcome"`
}

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
	Requirement string `json:"requirement"`
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

type CheckResult struct {
	ID       string `json:"id"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
}

type Assignment struct {
	JobID                int64               `json:"job_id"`
	Generation           int64               `json:"generation"`
	Role                 Role                `json:"role"`
	Task                 *Task               `json:"task,omitempty"`
	Requirements         []Requirement       `json:"requirements,omitempty"`
	Objective            *Objective          `json:"objective,omitempty"`
	ConstructionFacts    []ConstructionFact  `json:"construction_facts,omitempty"`
	Findings             []Finding           `json:"findings,omitempty"`
	Checks               []CheckResult       `json:"checks,omitempty"`
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
	PlanningResponse     string              `json:"planning_response,omitempty"`
	Response             string              `json:"response,omitempty"`
	Session              *ExecutionSession   `json:"session,omitempty"`
	Reply                string              `json:"reply,omitempty"`
	Context              string              `json:"context,omitempty"`
	Continuation         *TaskContinuation   `json:"continuation,omitempty"`
	Clarifications       []TaskClarification `json:"clarifications,omitempty"`
	CompletedTaskIDs     []string            `json:"completed_task_ids,omitempty"`
	ParserStep           *ParserStep         `json:"parser_step,omitempty"`
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
