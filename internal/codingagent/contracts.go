// Package codingagent defines the portable boundary between Omnidex task
// authority and replaceable agents. Agents propose results; code owns acceptance.
package codingagent

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"
)

type Mode string

const (
	Normal Mode = "normal"
	Agents Mode = "agents"
)

type Role string

const (
	Plan          Role = "plan"
	Execute       Role = "execute"
	Review        Role = "review"
	OverallReview Role = "overall_review"
)

type Route struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	Effort      string `json:"effort,omitempty"`
	ServiceTier string `json:"service_tier,omitempty"`
}
type Configuration struct {
	Mode     Mode  `json:"mode"`
	Planner  Route `json:"planner"`
	Executor Route `json:"executor"`
	Reviewer Route `json:"reviewer"`
}

func (config Configuration) Validate() error {
	if config.Mode == Normal {
		return nil
	}
	if config.Mode != Agents {
		return fmt.Errorf("unknown coding mode %q", config.Mode)
	}
	for _, route := range []Route{config.Planner, config.Executor, config.Reviewer} {
		if strings.TrimSpace(route.Provider) == "" || strings.TrimSpace(route.Model) == "" {
			return fmt.Errorf("Agents mode requires explicit planner, executor and reviewer routes")
		}
		for _, option := range []string{route.Effort, route.ServiceTier} {
			if len(option) > 64 || strings.TrimSpace(option) != option || strings.ContainsAny(option, "\x00\r\n") {
				return fmt.Errorf("agent inference option must be an exact supported value")
			}
		}
	}
	return nil
}

type Task struct {
	ID             string   `json:"id"`
	Goal           string   `json:"goal"`
	Acceptance     []string `json:"acceptance"`
	ReadFiles      []string `json:"read_files"`
	WriteFiles     []string `json:"write_files"`
	Dependencies   []string `json:"dependencies"`
	RequirementIDs []string `json:"requirement_ids"`
	Checks         []Check  `json:"checks"`
}
type Check struct {
	ID   string   `json:"id"`
	Argv []string `json:"argv"`
}

type Requirement struct {
	ID      string `json:"id"`
	Outcome string `json:"outcome"`
}

type Objective struct {
	Instruction    string   `json:"instruction"`
	Clarifications []string `json:"clarifications,omitempty"`
}
type ArtifactBinding struct {
	Token string `json:"token"`
	Value string `json:"value"`
}
type IntakeContext struct {
	RedactedRequest  string            `json:"redacted_request"`
	ArtifactBindings []ArtifactBinding `json:"artifact_bindings"`
}
type ConstructionFact struct {
	Kind  string `json:"kind"`
	Quote string `json:"quote"`
}
type PlanningEnvironment struct {
	NewFilesAllowed bool     `json:"new_files_allowed"`
	Executables     []string `json:"executables"`
}
type Workload struct {
	Tasks     []Task   `json:"tasks"`
	Checks    []Check  `json:"checks"`
	Questions []string `json:"questions,omitempty"`
}
type Finding struct {
	TaskID      string `json:"task_id"`
	File        string `json:"file,omitempty"`
	Requirement string `json:"requirement"`
	Observation string `json:"observation"`
}
type ReviewResult struct {
	Findings  []Finding `json:"findings"`
	Questions []string  `json:"questions"`
}
type ExecutionResult struct {
	Questions []string `json:"questions"`
}
type CheckResult struct {
	ID       string `json:"id"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output"`
}

type QuestionContext struct {
	Questions []string `json:"questions"`
	Answers   []string `json:"answers"`
}

type Assignment struct {
	JobID               int64                `json:"job_id"`
	Generation          int64                `json:"generation"`
	Role                Role                 `json:"role"`
	Task                *Task                `json:"task,omitempty"`
	Requirements        []Requirement        `json:"requirements,omitempty"`
	Objective           *Objective           `json:"objective,omitempty"`
	ArtifactBindings    []ArtifactBinding    `json:"artifact_bindings,omitempty"`
	PlanningEnvironment *PlanningEnvironment `json:"planning_environment,omitempty"`
	ConstructionFacts   []ConstructionFact   `json:"construction_facts,omitempty"`
	Workload            *Workload            `json:"workload,omitempty"`
	Findings            []Finding            `json:"findings,omitempty"`
	Checks              []CheckResult        `json:"checks,omitempty"`
	QuestionContext     []QuestionContext    `json:"question_context,omitempty"`
	ValidationNotes     []string             `json:"validation_notes,omitempty"`
	Root                string               `json:"root"`
	ReadFiles           []string             `json:"read_files"`
	WriteFiles          []string             `json:"write_files"`
	Route               Route                `json:"route"`
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

type Provider interface {
	Run(context.Context, Assignment, FileWriter, func(json.RawMessage) error) (Result, error)
}

type FileWrite struct {
	Path            string `json:"path"`
	Content         string `json:"content"`
	ExpectedContent string `json:"expected_content"`
}
type FileWriter interface {
	WriteFile(context.Context, FileWrite) error
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
