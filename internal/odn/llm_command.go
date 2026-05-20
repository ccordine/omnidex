package odn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gryph/omnidex/internal/specialist"
)

const defaultCommandDecisionTimeout = 3 * time.Minute
const defaultCommandDecisionMaxSteps = 10
const defaultStructuredObservationChars = 2400
const defaultStructuredLLMRequestAttempts = 3
const defaultStructuredEvaluatorTimeout = 20 * time.Second

type CommandDecisionClient interface {
	ChatRaw(ctx context.Context, req OllamaChatRequest) (OllamaChatResponse, error)
}

type StructuredCommandPayload struct {
	Command  string `json:"command"`
	Done     bool   `json:"done"`
	Answer   string `json:"answer"`
	Ask      bool   `json:"ask,omitempty"`
	Question string `json:"question,omitempty"`
	Tool     string `json:"tool,omitempty"`
	ToolTask string `json:"tool_task,omitempty"`
}

type CommandDecisionResult struct {
	Command      string
	ExitCode     int
	Answer       string
	Observations []StructuredCommandObservation
}

type StructuredCommandObservation struct {
	Step                 int    `json:"step"`
	Command              string `json:"command"`
	RejectedCommand      string `json:"rejected_command,omitempty"`
	RejectedResponse     string `json:"rejected_response,omitempty"`
	EvaluationConfidence int    `json:"evaluation_confidence,omitempty"`
	EvaluationFeedback   string `json:"evaluation_feedback,omitempty"`
	CapabilityMemory     string `json:"capability_memory,omitempty"`
	ExitCode             int    `json:"exit_code"`
	Stdout               string `json:"stdout"`
	Stderr               string `json:"stderr"`
	Question             string `json:"question,omitempty"`
	UserResponse         string `json:"user_response,omitempty"`
}

type StructuredCommandEvent struct {
	Type    string
	Summary string
	Details map[string]string
}

type StructuredLLMEvaluationInput struct {
	Step            int
	UserPrompt      string
	PlannerJob      string
	LLMResponse     string
	Observations    []StructuredCommandObservation
	SessionMemories []SessionMemory
}

type StructuredLLMEvaluation struct {
	Confidence int
	Feedback   string
}

type StructuredLLMResponseEvaluator interface {
	EvaluateStructuredLLMResponse(ctx context.Context, input StructuredLLMEvaluationInput) (StructuredLLMEvaluation, error)
}

type ShellCommandSpecialistInput struct {
	Step            int
	UserPrompt      string
	ToolTask        string
	Observations    []StructuredCommandObservation
	SessionMemories []SessionMemory
}

type ShellCommandProposal struct {
	Command   string
	Rationale string
}

type ShellCommandSpecialist interface {
	ProposeShellCommand(ctx context.Context, input ShellCommandSpecialistInput) (ShellCommandProposal, error)
}

type OllamaShellCommandSpecialist struct {
	Client CommandDecisionClient
}

func NewOllamaShellCommandSpecialist(client CommandDecisionClient) OllamaShellCommandSpecialist {
	return OllamaShellCommandSpecialist{Client: client}
}

func (s OllamaShellCommandSpecialist) ProposeShellCommand(ctx context.Context, input ShellCommandSpecialistInput) (ShellCommandProposal, error) {
	if s.Client == nil {
		return ShellCommandProposal{}, fmt.Errorf("shell specialist client is required")
	}
	resp, err := s.Client.ChatRaw(ctx, buildShellCommandSpecialistRequest(input))
	if err != nil {
		return ShellCommandProposal{}, err
	}
	return ParseShellCommandProposal(resp.Content)
}

type OllamaStructuredResponseEvaluator struct {
	Client CommandDecisionClient
}

func NewOllamaStructuredResponseEvaluator(client CommandDecisionClient) OllamaStructuredResponseEvaluator {
	return OllamaStructuredResponseEvaluator{Client: client}
}

func (e OllamaStructuredResponseEvaluator) EvaluateStructuredLLMResponse(ctx context.Context, input StructuredLLMEvaluationInput) (StructuredLLMEvaluation, error) {
	if e.Client == nil {
		return StructuredLLMEvaluation{}, fmt.Errorf("structured response evaluator client is required")
	}
	evalCtx, cancel := context.WithTimeout(ctx, defaultStructuredEvaluatorTimeout)
	defer cancel()
	resp, err := e.Client.ChatRaw(evalCtx, buildStructuredLLMEvaluationRequest(input))
	if err != nil {
		return StructuredLLMEvaluation{}, err
	}
	return ParseStructuredLLMEvaluation(resp.Content)
}

type ExitCodeError struct {
	Code int
}

func (e ExitCodeError) Error() string {
	return fmt.Sprintf("command exited with code %d", e.Code)
}

func IsExitCodeError(err error) (int, bool) {
	if exitErr, ok := err.(ExitCodeError); ok {
		return exitErr.Code, true
	}
	return 0, false
}

type CommandDecisionExhaustedError struct {
	MaxSteps int
}

func (e CommandDecisionExhaustedError) Error() string {
	return fmt.Sprintf("structured command loop exhausted after %d step(s) without accepted completion", e.MaxSteps)
}

type UserInputRequiredError struct {
	Question string
}

func (e UserInputRequiredError) Error() string {
	if strings.TrimSpace(e.Question) == "" {
		return "user input required"
	}
	return "user input required: " + e.Question
}

type StructuredCommandAskFunc func(ctx context.Context, question string) (string, error)

func RunStructuredCommandDecision(ctx context.Context, prompt string, client CommandDecisionClient, stdout, stderr io.Writer) (CommandDecisionResult, error) {
	return RunStructuredCommandDecisionWithEvents(ctx, prompt, client, stdout, stderr, nil)
}

func RunStructuredCommandDecisionWithEvents(ctx context.Context, prompt string, client CommandDecisionClient, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent)) (CommandDecisionResult, error) {
	return RunStructuredCommandDecisionWithEventsAndAsk(ctx, prompt, client, stdout, stderr, onEvent, nil)
}

func RunStructuredCommandDecisionWithEventsAndAsk(ctx context.Context, prompt string, client CommandDecisionClient, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), onAsk StructuredCommandAskFunc) (CommandDecisionResult, error) {
	return RunStructuredCommandDecisionWithHistoryEventsAndAsk(ctx, prompt, nil, client, stdout, stderr, onEvent, onAsk)
}

func RunStructuredCommandDecisionWithHistoryEventsAndAsk(ctx context.Context, prompt string, history []Message, client CommandDecisionClient, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), onAsk StructuredCommandAskFunc) (CommandDecisionResult, error) {
	return runStructuredCommandDecisionWithConfig(ctx, prompt, history, client, stdout, stderr, onEvent, onAsk, structuredCommandDecisionRunConfig{})
}

type structuredCommandDecisionRunConfig struct {
	SessionMemories    []SessionMemory
	Evaluator          StructuredLLMResponseEvaluator
	EvaluatorThreshold int
	ShellSpecialist    ShellCommandSpecialist
}

func runStructuredCommandDecisionWithConfig(ctx context.Context, prompt string, history []Message, client CommandDecisionClient, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), onAsk StructuredCommandAskFunc, cfg structuredCommandDecisionRunConfig) (CommandDecisionResult, error) {
	if strings.TrimSpace(prompt) == "" {
		return CommandDecisionResult{}, fmt.Errorf("prompt is empty")
	}
	if client == nil {
		return CommandDecisionResult{}, fmt.Errorf("llm client is required")
	}

	ctx, cancel := context.WithTimeout(ctx, defaultCommandDecisionTimeout)
	defer cancel()

	evaluator := cfg.Evaluator
	evaluatorThreshold := normalizeStructuredEvaluatorThreshold(cfg.EvaluatorThreshold)
	result := CommandDecisionResult{}
	for step := 1; step <= defaultCommandDecisionMaxSteps; step++ {
		emitStructuredCommandEvent(onEvent, "structured_llm_request_started", "Requesting next structured command decision", map[string]string{
			"step": fmt.Sprintf("%d", step),
		})
		resp, err := requestStructuredCommandPayload(ctx, client, buildStructuredCommandRequestWithMemories(prompt, history, cfg.SessionMemories, result.Observations), step, onEvent)
		if err != nil {
			if result.ExitCode == 0 {
				result.ExitCode = 1
			}
			return result, err
		}

		if evaluator != nil {
			evaluation, evalErr := evaluator.EvaluateStructuredLLMResponse(ctx, StructuredLLMEvaluationInput{
				Step:            step,
				UserPrompt:      prompt,
				PlannerJob:      structuredCommandPlannerJobSummary(),
				LLMResponse:     resp.Content,
				Observations:    result.Observations,
				SessionMemories: cfg.SessionMemories,
			})
			if evalErr != nil {
				emitStructuredCommandEvent(onEvent, "structured_response_evaluator_failed", "Structured response evaluator failed; continuing with deterministic validation", map[string]string{
					"step":  fmt.Sprintf("%d", step),
					"error": truncateStructuredTimelineValue(evalErr.Error()),
				})
				evaluator = nil
			} else if consistencyErr := validateStructuredEvaluationConsistency(evaluation); consistencyErr != nil {
				emitStructuredCommandEvent(onEvent, "structured_response_evaluator_failed", "Structured response evaluator returned inconsistent scoring; continuing with deterministic validation", map[string]string{
					"step":       fmt.Sprintf("%d", step),
					"confidence": fmt.Sprintf("%d", evaluation.Confidence),
					"feedback":   truncateStructuredTimelineValue(evaluation.Feedback),
					"error":      truncateStructuredTimelineValue(consistencyErr.Error()),
				})
				evaluator = nil
			} else {
				emitStructuredCommandEvent(onEvent, "structured_response_evaluated", "Structured response evaluator scored planner output", map[string]string{
					"step":       fmt.Sprintf("%d", step),
					"confidence": fmt.Sprintf("%d", evaluation.Confidence),
					"threshold":  fmt.Sprintf("%d", evaluatorThreshold),
					"feedback":   truncateStructuredTimelineValue(evaluation.Feedback),
				})
				if evaluation.Confidence < evaluatorThreshold {
					memory := structuredCapabilityMemoryForRejectedResponse(resp.Content, evaluation.Feedback)
					emitStructuredCommandEvent(onEvent, "structured_response_rejected", "Structured response rejected by evaluator", map[string]string{
						"step":       fmt.Sprintf("%d", step),
						"confidence": fmt.Sprintf("%d", evaluation.Confidence),
						"threshold":  fmt.Sprintf("%d", evaluatorThreshold),
						"feedback":   truncateStructuredTimelineValue(evaluation.Feedback),
					})
					result.Observations = append(result.Observations, StructuredCommandObservation{
						Step:                 step,
						RejectedResponse:     truncateStructuredObservation(resp.Content),
						EvaluationConfidence: evaluation.Confidence,
						EvaluationFeedback:   truncateStructuredObservation(evaluation.Feedback),
						CapabilityMemory:     memory,
						ExitCode:             1,
						Stderr:               structuredEvaluationRetryMessage(evaluation, evaluatorThreshold),
					})
					continue
				}
			}
		}

		payload, err := ParseStructuredCommandPayload(resp.Content)
		if err != nil {
			return result, err
		}
		emitStructuredCommandEvent(onEvent, "structured_llm_payload_received", "Structured command payload received", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"done":    fmt.Sprintf("%t", payload.Done),
			"ask":     fmt.Sprintf("%t", payload.Ask),
			"tool":    truncateStructuredTimelineValue(payload.Tool),
			"command": truncateStructuredTimelineValue(payload.Command),
		})
		if isShellToolDelegation(payload) {
			if cfg.ShellSpecialist == nil {
				emitStructuredCommandEvent(onEvent, "structured_tool_delegation_rejected", "Shell tool delegation rejected", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": "shell specialist is not configured",
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "tool delegation rejected: shell specialist is not configured; return a concrete command instead",
				})
				continue
			}
			emitStructuredCommandEvent(onEvent, "structured_tool_delegation_started", "Planner delegated shell command selection", map[string]string{
				"step":      fmt.Sprintf("%d", step),
				"tool_task": truncateStructuredTimelineValue(payload.ToolTask),
			})
			proposal, err := cfg.ShellSpecialist.ProposeShellCommand(ctx, ShellCommandSpecialistInput{
				Step:            step,
				UserPrompt:      prompt,
				ToolTask:        payload.ToolTask,
				Observations:    result.Observations,
				SessionMemories: cfg.SessionMemories,
			})
			if err != nil {
				emitStructuredCommandEvent(onEvent, "structured_tool_delegation_failed", "Shell specialist failed", map[string]string{
					"step":  fmt.Sprintf("%d", step),
					"error": truncateStructuredTimelineValue(err.Error()),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "shell specialist failed: " + err.Error(),
				})
				continue
			}
			emitStructuredCommandEvent(onEvent, "structured_tool_delegation_finished", "Shell specialist proposed command", map[string]string{
				"step":      fmt.Sprintf("%d", step),
				"command":   truncateStructuredTimelineValue(proposal.Command),
				"rationale": truncateStructuredTimelineValue(proposal.Rationale),
			})
			if err := validateStructuredCommandForObservations(proposal.Command, result.Observations); err != nil {
				emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": err.Error(),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:             step,
					RejectedCommand:  truncateStructuredObservation(proposal.Command),
					CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(proposal.Command, err.Error()),
					ExitCode:         1,
					Stderr:           "shell specialist command rejected: " + err.Error() + "; planner should delegate a narrower shell task or choose a different tool",
				})
				continue
			}
			if err := runStructuredPayloadCommand(ctx, step, proposal.Command, stdout, stderr, onEvent, &result); err != nil {
				return result, err
			}
			continue
		}
		if payload.Ask {
			question := strings.TrimSpace(payload.Question)
			command := strings.TrimSpace(payload.Command)
			if !hasRealCommandObservation(result.Observations) && command == "" {
				emitStructuredCommandEvent(onEvent, "structured_ask_rejected", "Ask rejected before real command evidence", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "ask rejected: no real command observation exists; inspect or try a command first",
				})
				continue
			}
			if latestRealCommandSucceeded(result.Observations) && command == "" {
				emitStructuredCommandEvent(onEvent, "structured_ask_rejected", "Ask rejected after latest command success", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "ask rejected: latest real command succeeded; continue with observed evidence, verify with another command, or finish",
				})
				continue
			}
			if question == "" && command != "" {
				emitStructuredCommandEvent(onEvent, "structured_ask_ignored", "Ask flag ignored for non-empty command", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": "empty question with ask=true; executing non-empty command",
				})
				if err := validateStructuredCommandForObservations(payload.Command, result.Observations); err != nil {
					emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
						"step":   fmt.Sprintf("%d", step),
						"reason": err.Error(),
					})
					result.Observations = append(result.Observations, StructuredCommandObservation{
						Step:             step,
						RejectedCommand:  truncateStructuredObservation(payload.Command),
						CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(payload.Command, err.Error()),
						ExitCode:         1,
						Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
					})
					continue
				}
				if err := runStructuredPayloadCommand(ctx, step, payload.Command, stdout, stderr, onEvent, &result); err != nil {
					return result, err
				}
				continue
			}
			previousAnswer, alreadyAnswered := previousUserResponseForQuestion(result.Observations, question)
			if alreadyAnswered {
				emitStructuredCommandEvent(onEvent, "structured_user_input_reused", "Structured loop reused prior user input", map[string]string{
					"step":     fmt.Sprintf("%d", step),
					"question": truncateStructuredTimelineValue(question),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:         step,
					ExitCode:     0,
					Question:     question,
					UserResponse: previousAnswer,
				})
				if command != "" {
					if err := validateStructuredCommandForObservations(payload.Command, result.Observations); err != nil {
						emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
							"step":   fmt.Sprintf("%d", step),
							"reason": err.Error(),
						})
						result.Observations = append(result.Observations, StructuredCommandObservation{
							Step:             step,
							RejectedCommand:  truncateStructuredObservation(payload.Command),
							CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(payload.Command, err.Error()),
							ExitCode:         1,
							Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
						})
						continue
					}
					if err := runStructuredPayloadCommand(ctx, step, payload.Command, stdout, stderr, onEvent, &result); err != nil {
						return result, err
					}
				}
				continue
			}
			if question == "" {
				emitStructuredCommandEvent(onEvent, "structured_ask_rejected", "Ask rejected by structured payload validation", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": "empty question with ask=true",
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "ask rejected: empty question with ask=true",
				})
				continue
			}
			if onAsk == nil {
				return result, UserInputRequiredError{Question: question}
			}
			emitStructuredCommandEvent(onEvent, "structured_user_input_requested", "Structured loop requested user input", map[string]string{
				"step":     fmt.Sprintf("%d", step),
				"question": truncateStructuredTimelineValue(question),
			})
			answer, err := onAsk(ctx, question)
			if err != nil {
				return result, err
			}
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:         step,
				ExitCode:     0,
				Question:     question,
				UserResponse: truncateStructuredObservation(answer),
			})
			emitStructuredCommandEvent(onEvent, "structured_user_input_received", "Structured loop received user input", map[string]string{
				"step": fmt.Sprintf("%d", step),
			})
			if command != "" {
				if err := validateStructuredCommandForObservations(payload.Command, result.Observations); err != nil {
					emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
						"step":   fmt.Sprintf("%d", step),
						"reason": err.Error(),
					})
					result.Observations = append(result.Observations, StructuredCommandObservation{
						Step:             step,
						RejectedCommand:  truncateStructuredObservation(payload.Command),
						CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(payload.Command, err.Error()),
						ExitCode:         1,
						Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
					})
					continue
				}
				if err := runStructuredPayloadCommand(ctx, step, payload.Command, stdout, stderr, onEvent, &result); err != nil {
					return result, err
				}
			}
			continue
		}
		if payload.Done {
			if strings.TrimSpace(payload.Command) != "" {
				if latest, ok := latestSuccessfulCommandObservation(result.Observations); ok && latestRealCommandSucceeded(result.Observations) {
					result.Answer = finalStructuredAnswer(payload.Answer, latest)
					if rejectDoneForFinalAnswer(step, result.Answer, onEvent, &result) {
						continue
					}
					emitStructuredCommandEvent(onEvent, "structured_done_accepted", "Structured command loop accepted final answer with repeated command", map[string]string{
						"step":    fmt.Sprintf("%d", step),
						"command": truncateStructuredTimelineValue(payload.Command),
						"answer":  truncateStructuredTimelineValue(result.Answer),
						"reason":  "non-empty command ignored because successful command evidence already exists",
					})
					return result, nil
				}
				emitStructuredCommandEvent(onEvent, "structured_done_ignored", "Done flag ignored for non-empty command", map[string]string{
					"step":   fmt.Sprintf("%d", step),
					"reason": "done=true requires an empty command; validating non-empty command instead",
				})
				if err := validateStructuredCommandForObservations(payload.Command, result.Observations); err != nil {
					emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
						"step":   fmt.Sprintf("%d", step),
						"reason": err.Error(),
					})
					result.Observations = append(result.Observations, StructuredCommandObservation{
						Step:             step,
						RejectedCommand:  truncateStructuredObservation(payload.Command),
						CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(payload.Command, err.Error()),
						ExitCode:         1,
						Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
					})
					continue
				}
				if err := runStructuredPayloadCommand(ctx, step, payload.Command, stdout, stderr, onEvent, &result); err != nil {
					return result, err
				}
				continue
			}
			if !hasRealCommandObservation(result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected before real command evidence", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "done rejected: no real command observation exists",
				})
				continue
			}
			if !hasSuccessfulCommandObservation(result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected before successful command evidence", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "done rejected: no successful command observation exists",
				})
				continue
			}
			if !latestRealCommandSucceeded(result.Observations) {
				emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected after latest command failure", map[string]string{
					"step": fmt.Sprintf("%d", step),
				})
				result.Observations = append(result.Observations, StructuredCommandObservation{
					Step:     step,
					ExitCode: 1,
					Stderr:   "done rejected: latest real command failed; try a different command or source",
				})
				continue
			}
			latest, _ := latestSuccessfulCommandObservation(result.Observations)
			result.Answer = finalStructuredAnswer(payload.Answer, latest)
			if rejectDoneForFinalAnswer(step, result.Answer, onEvent, &result) {
				continue
			}
			emitStructuredCommandEvent(onEvent, "structured_done_accepted", "Structured command loop accepted final answer", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"answer": truncateStructuredTimelineValue(result.Answer),
			})
			return result, nil
		}
		if strings.TrimSpace(payload.Command) == "" {
			if cfg.ShellSpecialist != nil {
				toolTask := strings.TrimSpace(payload.ToolTask)
				if toolTask == "" {
					toolTask = prompt
				}
				handled, err := runDelegatedShellSpecialist(ctx, step, prompt, toolTask, cfg, stdout, stderr, onEvent, &result)
				if err != nil {
					return result, err
				}
				if handled {
					continue
				}
			}
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": "empty command with done=false",
			})
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:     step,
				ExitCode: 1,
				Stderr:   "command rejected: empty command with done=false; choose an evidence-gathering command from tool_inventory",
			})
			continue
		}
		if err := validateStructuredCommandForObservations(payload.Command, result.Observations); err != nil {
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": err.Error(),
			})
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:             step,
				RejectedCommand:  truncateStructuredObservation(payload.Command),
				CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(payload.Command, err.Error()),
				ExitCode:         1,
				Stderr:           "command rejected: " + err.Error() + "; choose a different evidence-gathering command from tool_inventory",
			})
			continue
		}

		if err := runStructuredPayloadCommand(ctx, step, payload.Command, stdout, stderr, onEvent, &result); err != nil {
			return result, err
		}
	}

	emitStructuredCommandEvent(onEvent, "structured_loop_exhausted", "Structured command loop exhausted attempts", map[string]string{
		"max_steps": fmt.Sprintf("%d", defaultCommandDecisionMaxSteps),
	})
	if result.ExitCode == 0 {
		result.ExitCode = 1
	}
	return result, CommandDecisionExhaustedError{MaxSteps: defaultCommandDecisionMaxSteps}
}

func runStructuredPayloadCommand(ctx context.Context, step int, command string, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) error {
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	emitStructuredCommandEvent(onEvent, "structured_command_started", "Executing structured command", map[string]string{
		"step":    fmt.Sprintf("%d", step),
		"command": truncateStructuredTimelineValue(command),
	})
	exitCode, err := ExecuteStructuredCommand(ctx, command, io.MultiWriter(stdout, &stdoutBuf), io.MultiWriter(stderr, &stderrBuf))
	result.Command = command
	result.ExitCode = exitCode
	result.Observations = append(result.Observations, StructuredCommandObservation{
		Step:     step,
		Command:  command,
		ExitCode: exitCode,
		Stdout:   truncateStructuredObservation(stdoutBuf.String()),
		Stderr:   truncateStructuredObservation(stderrBuf.String()),
	})
	emitStructuredCommandEvent(onEvent, "structured_command_finished", "Structured command finished", map[string]string{
		"step":      fmt.Sprintf("%d", step),
		"command":   truncateStructuredTimelineValue(command),
		"exit_code": fmt.Sprintf("%d", exitCode),
		"stdout":    truncateStructuredTimelineValue(stdoutBuf.String()),
		"stderr":    truncateStructuredTimelineValue(stderrBuf.String()),
	})
	return err
}

func runDelegatedShellSpecialist(ctx context.Context, step int, prompt, toolTask string, cfg structuredCommandDecisionRunConfig, stdout, stderr io.Writer, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) (bool, error) {
	if cfg.ShellSpecialist == nil {
		return false, nil
	}
	emitStructuredCommandEvent(onEvent, "structured_tool_delegation_started", "Planner delegated shell command selection", map[string]string{
		"step":      fmt.Sprintf("%d", step),
		"tool_task": truncateStructuredTimelineValue(toolTask),
	})
	proposal, err := cfg.ShellSpecialist.ProposeShellCommand(ctx, ShellCommandSpecialistInput{
		Step:            step,
		UserPrompt:      prompt,
		ToolTask:        toolTask,
		Observations:    result.Observations,
		SessionMemories: cfg.SessionMemories,
	})
	if err != nil {
		emitStructuredCommandEvent(onEvent, "structured_tool_delegation_failed", "Shell specialist failed", map[string]string{
			"step":  fmt.Sprintf("%d", step),
			"error": truncateStructuredTimelineValue(err.Error()),
		})
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   "shell specialist failed: " + err.Error(),
		})
		return true, nil
	}
	emitStructuredCommandEvent(onEvent, "structured_tool_delegation_finished", "Shell specialist proposed command", map[string]string{
		"step":      fmt.Sprintf("%d", step),
		"command":   truncateStructuredTimelineValue(proposal.Command),
		"rationale": truncateStructuredTimelineValue(proposal.Rationale),
	})
	if err := validateStructuredCommandForObservations(proposal.Command, result.Observations); err != nil {
		emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
			"step":   fmt.Sprintf("%d", step),
			"reason": err.Error(),
		})
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:             step,
			RejectedCommand:  truncateStructuredObservation(proposal.Command),
			CapabilityMemory: structuredCapabilityMemoryForRejectedResponse(proposal.Command, err.Error()),
			ExitCode:         1,
			Stderr:           "shell specialist command rejected: " + err.Error() + "; planner should delegate a narrower shell task or choose a different tool",
		})
		return true, nil
	}
	if err := runStructuredPayloadCommand(ctx, step, proposal.Command, stdout, stderr, onEvent, result); err != nil {
		return true, err
	}
	return true, nil
}

func requestStructuredCommandPayload(ctx context.Context, client CommandDecisionClient, req OllamaChatRequest, step int, onEvent func(StructuredCommandEvent)) (OllamaChatResponse, error) {
	var lastErr error
	for attempt := 1; attempt <= defaultStructuredLLMRequestAttempts; attempt++ {
		resp, err := client.ChatRaw(ctx, req)
		if err == nil {
			if attempt > 1 {
				emitStructuredCommandEvent(onEvent, "structured_llm_request_recovered", "Structured LLM request recovered after retry", map[string]string{
					"step":    fmt.Sprintf("%d", step),
					"attempt": fmt.Sprintf("%d", attempt),
				})
			}
			return resp, nil
		}
		lastErr = err
		emitStructuredCommandEvent(onEvent, "structured_llm_request_failed", "Structured LLM request failed", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"attempt": fmt.Sprintf("%d", attempt),
			"error":   truncateStructuredTimelineValue(err.Error()),
		})
		if !isTransientStructuredLLMError(err) || attempt == defaultStructuredLLMRequestAttempts {
			return OllamaChatResponse{}, err
		}
		emitStructuredCommandEvent(onEvent, "structured_llm_backend_unstable", "Ollama backend appears unstable; retrying request", map[string]string{
			"step":       fmt.Sprintf("%d", step),
			"attempt":    fmt.Sprintf("%d", attempt),
			"diagnosis":  classifyStructuredLLMFailure(err),
			"mitigation": "check journalctl -u ollama; prefer cpu_avx2 or reduce Ollama context/keep_alive if ROCm is crashing",
		})
		select {
		case <-ctx.Done():
			return OllamaChatResponse{}, ctx.Err()
		case <-time.After(time.Duration(attempt) * 2 * time.Second):
		}
	}
	return OllamaChatResponse{}, lastErr
}

func normalizeStructuredEvaluatorThreshold(value int) int {
	if value <= 0 {
		return defaultEvaluatorThreshold
	}
	if value > 100 {
		return 100
	}
	return value
}

func structuredEvaluationRetryMessage(evaluation StructuredLLMEvaluation, threshold int) string {
	feedback := strings.TrimSpace(evaluation.Feedback)
	if feedback == "" {
		feedback = "planner response was not sufficiently aligned with the active task"
	}
	return fmt.Sprintf("self-evaluation rejected response: confidence=%d threshold=%d; feedback=%s; try again using the active prompt, planner job, observations, and capability memory", evaluation.Confidence, threshold, feedback)
}

func structuredCommandPlannerJobSummary() string {
	return strings.Join([]string{
		"Return strict JSON for the next command-planning step.",
		"Use schema {\"command\":\"shell command to execute\",\"done\":false,\"answer\":\"\"}.",
		"Use {\"command\":\"\",\"done\":true,\"answer\":\"brief result from observed evidence\"} only after successful command evidence.",
		"Commands must gather evidence, inspect state, create requested output, or verify results.",
		"Do not simulate final answers with echo/printf apologies or claims that real-time information is unavailable.",
		"Use shell commands and public unauthenticated sources for current facts when needed.",
	}, " ")
}

func buildStructuredLLMEvaluationRequest(input StructuredLLMEvaluationInput) OllamaChatRequest {
	payload := struct {
		Step            int                            `json:"step"`
		Job             string                         `json:"planner_job"`
		UserPrompt      string                         `json:"user_prompt"`
		LLMResponse     string                         `json:"llm_response"`
		Observations    []StructuredCommandObservation `json:"observations"`
		SessionMemories []SessionMemory                `json:"session_memories,omitempty"`
	}{
		Step:            input.Step,
		Job:             input.PlannerJob,
		UserPrompt:      input.UserPrompt,
		LLMResponse:     input.LLMResponse,
		Observations:    input.Observations,
		SessionMemories: input.SessionMemories,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{}`)
	}
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are a tiny strict evaluator.",
					"Return JSON only with schema {\"confidence\":0,\"feedback\":\"\"}.",
					"confidence must be an integer from 0 to 100.",
					"Score whether llm_response is on track for planner_job and user_prompt.",
					"Scoring rubric: 90-100 clearly on track or complete, 70-89 mostly on track, 40-69 uncertain or incomplete, 0-39 off track.",
					"If feedback says on track, successfully completed, or correctly answered, confidence must be at least 80.",
					"If confidence is below 70, feedback must state what is missing or wrong and must not say the response is on track.",
					"Do not solve the user's task.",
					"Do not penalize a proposed command merely because it has not executed yet; the runtime executes accepted commands.",
					"Give low confidence when the response ignores the active prompt, answers from memory, refuses a capability that shell/public sources provide, returns done without evidence, or emits a command that only prints an answer/apology.",
					"Give low confidence for obviously invalid shell command syntax or repeated commands already shown failing in observations.",
					"feedback must be one concise sentence explaining how the planner should retry.",
				}, " "),
			},
			{Role: "user", Content: string(blob)},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"confidence": map[string]interface{}{"type": "integer", "minimum": 0, "maximum": 100},
				"feedback":   map[string]interface{}{"type": "string"},
			},
			"required": []string{"confidence", "feedback"},
		},
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 128,
		},
	}
}

func ParseStructuredLLMEvaluation(raw string) (StructuredLLMEvaluation, error) {
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.UseNumber()
	var decoded map[string]interface{}
	if err := decoder.Decode(&decoded); err != nil {
		return StructuredLLMEvaluation{}, fmt.Errorf("parse structured response evaluation: %w", err)
	}
	confidence, err := parseStructuredEvaluationConfidence(decoded["confidence"])
	if err != nil {
		return StructuredLLMEvaluation{}, err
	}
	feedback, _ := decoded["feedback"].(string)
	return StructuredLLMEvaluation{
		Confidence: confidence,
		Feedback:   strings.TrimSpace(feedback),
	}, nil
}

func parseStructuredEvaluationConfidence(raw interface{}) (int, error) {
	switch value := raw.(type) {
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return validateStructuredEvaluationConfidence(int(parsed))
		}
		floatValue, err := strconv.ParseFloat(value.String(), 64)
		if err != nil {
			return 0, fmt.Errorf("structured response evaluation confidence is not numeric")
		}
		return validateStructuredEvaluationConfidence(int(floatValue))
	case float64:
		return validateStructuredEvaluationConfidence(int(value))
	case int:
		return validateStructuredEvaluationConfidence(value)
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(value))
		if err != nil {
			return 0, fmt.Errorf("structured response evaluation confidence is not numeric")
		}
		return validateStructuredEvaluationConfidence(parsed)
	default:
		return 0, fmt.Errorf("structured response evaluation missing confidence")
	}
}

func validateStructuredEvaluationConfidence(value int) (int, error) {
	if value < 0 || value > 100 {
		return 0, fmt.Errorf("structured response evaluation confidence out of range")
	}
	return value, nil
}

func validateStructuredEvaluationConsistency(evaluation StructuredLLMEvaluation) error {
	if evaluation.Confidence >= defaultEvaluatorThreshold {
		return nil
	}
	if structuredEvaluationFeedbackClaimsSuccess(evaluation.Feedback) {
		return fmt.Errorf("low confidence contradicts positive feedback")
	}
	return nil
}

func structuredEvaluationFeedbackClaimsSuccess(feedback string) bool {
	lower := strings.ToLower(feedback)
	if strings.Contains(lower, "not on track") || strings.Contains(lower, "off track") {
		return false
	}
	for _, phrase := range []string{
		"on track",
		"successfully completed",
		"correctly answered",
		"answered correctly",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

const structuredRealtimeCapabilityMemory = "ODN can use shell commands and public unauthenticated sources to gather current facts. For location-specific time, use TZ=Area/City date or another evidence command; do not claim no real-time access when command evidence can be gathered."

func structuredCapabilityMemoryForRejectedResponse(response, feedback string) string {
	if structuredTextSuggestsFalseCapabilityLimit(response) || structuredTextSuggestsFalseCapabilityLimit(feedback) {
		return structuredRealtimeCapabilityMemory
	}
	return ""
}

func structuredTextSuggestsFalseCapabilityLimit(text string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range []string{
		"as an ai",
		"i am unable",
		"i'm unable",
		"i cannot",
		"i can't",
		"i do not have access",
		"i don't have access",
		"do not have access to real-time",
		"don't have access to real-time",
		"cannot access real-time",
		"can't access real-time",
		"no access to real-time",
		"do not have internet access",
		"don't have internet access",
		"no internet access",
		"cannot browse",
		"can't browse",
		"unable to browse",
		"not able to browse",
		"check a weather website",
		"check the current time",
		"time zone app",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func structuredTextDefersEvidenceToFutureCommand(text string) bool {
	lower := strings.ToLower(text)
	for _, phrase := range []string{
		"can be identified by running",
		"can be determined by running",
		"can be found by running",
		"can be checked by running",
		"run the command",
		"using the uname",
		"using uname",
	} {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

func previousUserResponseForQuestion(observations []StructuredCommandObservation, question string) (string, bool) {
	question = strings.TrimSpace(question)
	if question == "" {
		return "", false
	}
	for i := len(observations) - 1; i >= 0; i-- {
		if strings.TrimSpace(observations[i].Question) == question && strings.TrimSpace(observations[i].UserResponse) != "" {
			return observations[i].UserResponse, true
		}
	}
	return "", false
}

func isTransientStructuredLLMError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "model runner has unexpectedly stopped") ||
		strings.Contains(text, "ollama returned status 500") ||
		strings.Contains(text, "unexpected eof") ||
		strings.Contains(text, "connection reset by peer")
}

func classifyStructuredLLMFailure(err error) string {
	if err == nil {
		return ""
	}
	text := strings.ToLower(err.Error())
	if strings.Contains(text, "model runner has unexpectedly stopped") ||
		strings.Contains(text, "unexpected eof") ||
		strings.Contains(text, "connection reset by peer") {
		return "ollama_model_runner_crash_or_restart"
	}
	if strings.Contains(text, "ollama returned status 500") {
		return "ollama_internal_error"
	}
	if strings.Contains(text, "context deadline exceeded") || strings.Contains(text, "client.timeout") {
		return "ollama_request_timeout"
	}
	return "ollama_request_failure"
}

func validateStructuredCommandString(command string) error {
	if startsWithShellRedirectionToken(command) {
		return fmt.Errorf("command starts with shell redirection token")
	}
	lower := strings.ToLower(command)
	for _, placeholder := range []string{
		"<location>", "<query>", "<file>", "<filename>", "<path>", "<url>", "<number>", "<name>", "<project>",
		"<city>", "<country>", "<timezone>", "<api_key>", "<token>", "<placeholder>",
	} {
		if strings.Contains(lower, placeholder) {
			return fmt.Errorf("placeholder angle-bracket value in command")
		}
	}
	if strings.Contains(lower, "your_api_key") || strings.Contains(lower, "api_key_here") {
		return fmt.Errorf("placeholder angle-bracket value in command")
	}
	if isPureEchoCommand(command) {
		return fmt.Errorf("pure echo command is not command evidence")
	}
	if err := validateWTTRCommand(command); err != nil {
		return err
	}
	if err := validateDateCommand(command); err != nil {
		return err
	}
	return nil
}

func validateStructuredCommandForObservations(command string, observations []StructuredCommandObservation) error {
	if err := validateStructuredCommandString(command); err != nil {
		return err
	}
	if repeatedFailedStructuredCommand(command, observations) {
		return fmt.Errorf("command repeats a previous failed command; choose a different command, source, or local tool")
	}
	return nil
}

func repeatedFailedStructuredCommand(command string, observations []StructuredCommandObservation) bool {
	normalized := normalizeStructuredCommandForComparison(command)
	if normalized == "" {
		return false
	}
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) == "" || obs.ExitCode == 0 {
			continue
		}
		if normalizeStructuredCommandForComparison(obs.Command) == normalized {
			return true
		}
	}
	return false
}

func normalizeStructuredCommandForComparison(command string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(command)), " ")
}

func startsWithShellRedirectionToken(command string) bool {
	fields := strings.Fields(strings.TrimSpace(command))
	if len(fields) == 0 {
		return false
	}
	return isShellRedirectToken(fields[0])
}

func isPureEchoCommand(command string) bool {
	trimmed := strings.TrimSpace(command)
	lower := strings.ToLower(trimmed)
	if lower != "echo" && !strings.HasPrefix(lower, "echo ") {
		return false
	}
	for _, marker := range []string{"|", ">", "<", "$(", "`", "&&", "||", ";"} {
		if strings.Contains(trimmed, marker) {
			return false
		}
	}
	return true
}

func validateWTTRCommand(command string) error {
	lower := strings.ToLower(command)
	if !strings.Contains(lower, "wttr.in") {
		return nil
	}
	if !strings.Contains(lower, "wttr.in/") {
		return fmt.Errorf("wttr.in command must include an explicit location path")
	}
	if strings.Contains(lower, "wttr.in/?") || strings.Contains(lower, "wttr.in/ ") || strings.HasSuffix(strings.TrimSpace(lower), "wttr.in/") {
		return fmt.Errorf("wttr.in command must include a non-empty location path")
	}
	if !strings.Contains(lower, "format=") {
		return fmt.Errorf("wttr.in command must use a concise format query")
	}
	return nil
}

func validateDateCommand(command string) error {
	trimmed := strings.TrimSpace(command)
	lower := strings.ToLower(trimmed)
	if !strings.Contains(lower, "date") {
		return nil
	}
	fields := strings.Fields(lower)
	for i, field := range fields {
		if field == "date" || strings.HasSuffix(field, "/date") {
			if i+1 < len(fields) && fields[i+1] == "-t" {
				return fmt.Errorf("date command must not use invalid -t timezone option; prefix with TZ=Area/City before date")
			}
		}
	}
	if strings.Contains(lower, "date ") && strings.Contains(lower, "tz=") && !strings.HasPrefix(lower, "tz=") && !strings.Contains(lower, " tz=") {
		return fmt.Errorf("date command must prefix TZ=Area/City before date, not pass TZ as a date argument")
	}
	if strings.Contains(lower, "date ") && strings.Contains(lower, "-d") && strings.Contains(lower, "tz=") && !strings.HasPrefix(lower, "tz=") {
		return fmt.Errorf("date command must prefix TZ=Area/City before date, not pass TZ through -d")
	}
	return nil
}

func emitStructuredCommandEvent(onEvent func(StructuredCommandEvent), eventType, summary string, details map[string]string) {
	if onEvent == nil {
		return
	}
	onEvent(StructuredCommandEvent{Type: eventType, Summary: summary, Details: details})
}

func hasRealCommandObservation(observations []StructuredCommandObservation) bool {
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" {
			return true
		}
	}
	return false
}

func hasSuccessfulCommandObservation(observations []StructuredCommandObservation) bool {
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" && obs.ExitCode == 0 {
			return true
		}
	}
	return false
}

func latestSuccessfulCommandObservation(observations []StructuredCommandObservation) (StructuredCommandObservation, bool) {
	for i := len(observations) - 1; i >= 0; i-- {
		if strings.TrimSpace(observations[i].Command) != "" && observations[i].ExitCode == 0 {
			return observations[i], true
		}
	}
	return StructuredCommandObservation{}, false
}

func finalStructuredAnswer(payloadAnswer string, latest StructuredCommandObservation) string {
	if answer := strings.TrimSpace(payloadAnswer); answer != "" {
		return answer
	}
	if stdout := strings.TrimSpace(latest.Stdout); stdout != "" {
		return stdout
	}
	return strings.TrimSpace(latest.Stderr)
}

func rejectDoneForFinalAnswer(step int, answer string, onEvent func(StructuredCommandEvent), result *CommandDecisionResult) bool {
	answer = strings.TrimSpace(answer)
	if structuredTextSuggestsFalseCapabilityLimit(answer) {
		emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected for false capability limitation", map[string]string{
			"step":   fmt.Sprintf("%d", step),
			"answer": truncateStructuredTimelineValue(answer),
		})
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:             step,
			ExitCode:         1,
			CapabilityMemory: structuredRealtimeCapabilityMemory,
			Stderr:           "done rejected: final answer claims inability despite successful command evidence; answer from observed evidence or run another command",
		})
		result.Answer = ""
		return true
	}
	if structuredTextDefersEvidenceToFutureCommand(answer) {
		emitStructuredCommandEvent(onEvent, "structured_done_rejected", "Done rejected for deferred evidence", map[string]string{
			"step":   fmt.Sprintf("%d", step),
			"answer": truncateStructuredTimelineValue(answer),
		})
		result.Observations = append(result.Observations, StructuredCommandObservation{
			Step:     step,
			ExitCode: 1,
			Stderr:   "done rejected: final answer describes commands that should be run instead of using observed evidence; run the missing command or summarize only observed evidence",
		})
		result.Answer = ""
		return true
	}
	return false
}

func latestRealCommandSucceeded(observations []StructuredCommandObservation) bool {
	for i := len(observations) - 1; i >= 0; i-- {
		if strings.TrimSpace(observations[i].Command) == "" {
			continue
		}
		return observations[i].ExitCode == 0
	}
	return false
}

func isShellToolDelegation(payload StructuredCommandPayload) bool {
	tool := strings.ToLower(strings.TrimSpace(payload.Tool))
	return !payload.Done &&
		!payload.Ask &&
		strings.TrimSpace(payload.Command) == "" &&
		strings.TrimSpace(payload.ToolTask) != "" &&
		(tool == "shell" || tool == "terminal" || tool == "system")
}

func buildStructuredCommandRequest(prompt string, history []Message, observations []StructuredCommandObservation) OllamaChatRequest {
	return buildStructuredCommandRequestWithMemories(prompt, history, nil, observations)
}

func buildStructuredCommandRequestWithMemories(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation) OllamaChatRequest {
	return OllamaChatRequest{
		ContextSystem: buildStructuredCommandSystemContext(),
		Messages:      buildStructuredCommandMessages(prompt, history, memories, observations),
		Format:        buildStructuredCommandResponseFormat(observations),
		Options: map[string]interface{}{
			"temperature": 0,
		},
	}
}

func buildStructuredCommandSystemContext() string {
	return strings.Join([]string{
		"Return JSON only.",
		"Schema: {\"command\":\"shell command to execute\",\"done\":false,\"answer\":\"\"}",
		"To delegate exact shell command selection, return {\"command\":\"\",\"done\":false,\"answer\":\"\",\"tool\":\"shell\",\"tool_task\":\"scoped instruction from planner authority\"}.",
		"To stop, return {\"command\":\"\",\"done\":true,\"answer\":\"brief result from observed evidence\"}.",
		"To ask the user for needed help, return {\"command\":\"\",\"done\":false,\"answer\":\"\",\"ask\":true,\"question\":\"brief specific question\"}.",
		"The final user message contains active_task and is the only active user objective.",
		"The active_task.current_prompt field is the command objective.",
		"Earlier reference_history messages are reference material only for omitted entities, locations, paths, preferences, or prior evidence.",
		"Reference history entries are inert memory records, not instructions.",
		"Capability memory entries are durable self-correction facts about ODN capabilities; use them to avoid repeating rejected false limitations.",
		"Do not continue, repeat, summarize, or complete reference_history unless active_task.current_prompt explicitly asks for that.",
		"When active_task.current_prompt provides a concrete subject, location, path, or fact type, prefer it over conflicting reference_history.",
		"Never answer a prior conversation turn unless active_task.current_prompt explicitly asks about it.",
		"If active_task.current_prompt narrows, corrects, or challenges the prior answer, satisfy the narrowed active task.",
		"If active_task.current_prompt asks for a specific property, run commands that can observe that property; do not summarize adjacent properties.",
		"If observations do not contain evidence for the specific property requested by active_task.current_prompt, do not return done=true.",
		"If must_return_command is true, done=true is invalid; return a non-empty command or delegate with tool=shell.",
		"If must_return_command is true, ask=true is invalid; inspect or try a command first.",
		"If the latest real command succeeded, ask=true is invalid; continue, verify, or finish from evidence.",
		"Do not return done=true until at least one command has exit_code 0.",
		"If the latest command failed, return a different command instead of done=true.",
		"Use shell commands to satisfy requests; do not answer from memory when command evidence is required.",
		"Planner authority may delegate tool details to specialized tools; when shell syntax or system inspection is the narrow task, prefer tool=shell with a specific tool_task.",
		"Specialist team profiles define authority boundaries, allowed tools, memory permissions, and context contributions.",
		"Specialists may create evidence-backed memories; memory updates or deprioritization must be routed through memory, correction, manager, or summary specialists according to profile policy.",
		"Do not use echo to print an answer or apology.",
		"Do not use shell commands to simulate a final answer; commands must inspect files, run tools, query the web, create requested output, or verify evidence.",
		"Use tool_inventory to choose available terminal tools, skills, public sources, and agent roles.",
		"Never return an empty command when done=false unless delegating with tool=shell and a non-empty tool_task.",
		"If a command fails, the failure is recorded in observations; use that context to pivot to a different command, source, or tool.",
		"If the user already answered a question, do not ask the same question again; use the observed user_response.",
		"If you ask for approval and include an approval-gated command, that command may run after the user answers.",
		"Use reference_history to resolve follow-up references before asking the user.",
		"If reference_history contains the missing subject, location, file, project, or preference, use it in the command instead of asking.",
		"Ask the user only when progress requires permission, credentials, sudo, destructive approval, or a choice that cannot be inferred from evidence.",
		"Do not ask for help when another non-destructive command, public source, or local inspection can be tried.",
		"After each command, inspect stdout/stderr/exit_code and decide whether another command is needed.",
		"The command must be a single shell command.",
		"Each command runs in a fresh shell; cd does not persist to the next step.",
		"Use absolute paths or include cd in the same command that needs it.",
		"A command that only changes directory does not help later steps; combine cd with the file creation, build, test, or verification command that needs that directory.",
		"Use current_working_directory for project creation unless the user explicitly provided another path.",
		"Do not create demo projects in the home directory unless the user explicitly asked for home.",
		"Available terminal tools may include bash, curl, python3, sed, awk, grep, jq, date, uname, and package managers; discover with commands when uncertain.",
		"To identify the operating system, inspect command evidence such as uname and /etc/os-release.",
		"For OS identification requests, gather distro, kernel, architecture, and package-manager evidence before done=true; prefer one command that prints /etc/os-release, uname -srmo, and command -v pacman apt dnf yum zypper apk.",
		"For identification tasks, inspect available package managers only; do not ask for permission to proceed with a package manager.",
		"Before OS-specific package or install advice, verify OS, distro, version, architecture, and available package managers with commands.",
		"If a needed tool is missing, identify install options from verified OS/package-manager evidence.",
		"Do not install missing tools unless the user explicitly asked to install or approved installation.",
		"When installation is not approved, answer with the proposed install command and ask for approval.",
		"For desktop/browser tasks, inspect running processes and the GUI session with commands before acting.",
		"For browser window tasks, discover available tools such as firefox, xdg-open, wmctrl, xdotool, gdbus, or gio with commands when uncertain.",
		"When asked to use a browser PID or existing browser process, find the running process first, then use window/browser commands based on observed evidence.",
		"If desktop control is impossible because no GUI session, browser process, or needed tool is available, report the missing evidence and ask for the smallest needed user action.",
		"Do not use placeholder credentials.",
		"Do not call APIs that require unavailable keys.",
		"Never put placeholder key text in a command.",
		"Never put placeholder angle-bracket values such as <location>, <query>, <file>, or <url> in a command.",
		"For external facts, use public unauthenticated sources.",
		"For timely public information, use internet commands by default.",
		"For current, recent, latest, today, or now public facts, the first accepted command should gather live evidence from the internet.",
		"For current external facts, run an internet command and use observed output before done.",
		"For filesystem changes, run shell commands that create or modify the requested filesystem state.",
		"For local static web app demos, create files locally and serve them with a local server such as python3 http.server.",
		"For Go CLI demos, use curl to discover the latest Go release from go.dev/dl/?mode=json, install that Go toolchain into a user-writable project directory unless system installation is approved, then build, test, and run the app.",
		"The Go release JSON has version and files[].filename fields; construct downloads as https://go.dev/dl/<filename>.",
		"For Go CLI demos, do not return done=true until go test, go build, and the built executable have all succeeded.",
		"Do not treat null or empty JSON query output as useful evidence.",
		"For npm React TypeScript demos, prefer a minimal Vite project with package.json and src files; do not use create-react-app.",
		"For npm install/build commands in tests, keep output concise when possible.",
		"For Docker app tasks, verify docker is available, create the app and Dockerfile, build the image, run a named container, verify it with curl, inspect container state/restart count, and inspect docker logs before done=true.",
		"For Docker smoke tests, prefer local build contexts that do not require pulling large base images when a static binary or scratch image can satisfy the request.",
		"Do not return done=true for a Docker app until docker build, docker run, live endpoint verification, docker inspect, and docker logs checks have succeeded.",
		"When starting a background server, use nohup or equivalent and write the background process PID with $! if a PID file is requested.",
		"When starting a background server, redirect stdout and stderr away from the command pipe.",
		"Do not background file creation or setup commands; only background the long-running server process.",
		"When chaining commands before a background server, use semicolons before nohup; avoid '&& nohup ... &' because bash may background the setup chain.",
		"After starting a local server, verify it with a short curl retry loop before done=true.",
		"Do not ask for public sources when the task can be completed with local files.",
		"If observed output is empty, denied, or not useful, try a different public source.",
		"If output reports invalid credentials, try a no-key public source before done.",
		"If the shell reports a syntax or quoting error, correct the command or use a simpler command.",
		"Match the command source to the requested fact type.",
		"Public no-key internet sources available: wttr.in, news.google.com/rss/search?q=<query>, duckduckgo.com/html/?q=<query>.",
		"When using wttr.in, include an explicit location path and a concise format query.",
		"Prefer simple curl commands that print readable evidence over fragile HTML parsing.",
		"For current time, prefer shell time/date commands or public no-key time sources.",
		"For location-specific time, produce local-time evidence for that location; do not answer from UTC unless UTC was requested.",
		"Do not use weather services as time sources.",
		"If using shell date for a location, choose an IANA timezone and prefix the command with TZ=Area/City before date.",
		"For Pattaya or any Thailand current-time request, use the IANA timezone Asia/Bangkok, for example TZ=Asia/Bangkok date '+%Y-%m-%d %H:%M:%S %Z'.",
		"Do not pass TZ=Area/City as an argument to date.",
		"Prefer concise command output; use format/query options instead of large pages when available.",
		"No markdown.",
	}, "\n")
}

func buildStructuredCommandResponseFormat(observations []StructuredCommandObservation) map[string]interface{} {
	properties := map[string]interface{}{
		"command":  map[string]interface{}{"type": "string"},
		"done":     map[string]interface{}{"type": "boolean"},
		"answer":   map[string]interface{}{"type": "string"},
		"ask":      map[string]interface{}{"type": "boolean"},
		"question": map[string]interface{}{"type": "string"},
		"tool":     map[string]interface{}{"type": "string"},
		"tool_task": map[string]interface{}{
			"type": "string",
		},
	}
	if !hasRealCommandObservation(observations) {
		properties["done"] = map[string]interface{}{"type": "boolean", "enum": []bool{false}}
	}
	return map[string]interface{}{
		"type":       "object",
		"properties": properties,
		"required":   []string{"command", "done", "answer"},
	}
}

func buildShellCommandSpecialistRequest(input ShellCommandSpecialistInput) OllamaChatRequest {
	payload := struct {
		Role            string                         `json:"role"`
		UserPrompt      string                         `json:"user_prompt"`
		ToolTask        string                         `json:"tool_task"`
		Observations    []StructuredCommandObservation `json:"observations"`
		SessionMemories []SessionMemory                `json:"session_memories,omitempty"`
		ToolRules       []string                       `json:"tool_rules"`
	}{
		Role:            "shell_execution_specialist",
		UserPrompt:      input.UserPrompt,
		ToolTask:        input.ToolTask,
		Observations:    input.Observations,
		SessionMemories: input.SessionMemories,
		ToolRules: []string{
			"Return JSON only with schema {\"command\":\"...\",\"rationale\":\"...\"}.",
			"Only choose a shell command that directly satisfies tool_task from the planner authority.",
			"Do not answer the user and do not apologize.",
			"Do not use echo or printf to fake final evidence unless the task is explicitly to create/write literal text.",
			"For location-specific current time, prefer TZ=Area/City date '+%Y-%m-%d %H:%M:%S %Z'.",
			"For Thailand or Pattaya current time, use TZ=Asia/Bangkok date '+%Y-%m-%d %H:%M:%S %Z'.",
			"If a prior command failed, choose a different command or corrected syntax.",
		},
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		blob = []byte(`{"role":"shell_execution_specialist","tool_task":""}`)
	}
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are a shell execution specialist subordinate to a planner authority.",
					"You receive a scoped tool_task and return the safest concrete shell command for evidence gathering or requested system interaction.",
					"Return JSON only.",
				}, " "),
			},
			{Role: "user", Content: string(blob)},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command":   map[string]interface{}{"type": "string"},
				"rationale": map[string]interface{}{"type": "string"},
			},
			"required": []string{"command", "rationale"},
		},
		Options: map[string]interface{}{
			"temperature": 0,
			"num_predict": 256,
		},
	}
}

func buildStructuredCommandMessages(prompt string, history []Message, memories []SessionMemory, observations []StructuredCommandObservation) []OllamaMessage {
	messages := []OllamaMessage{}
	if memoryMessage := buildStructuredCommandCapabilityMemoryMessage(memories); memoryMessage != "" {
		messages = append(messages,
			OllamaMessage{Role: "user", Content: memoryMessage},
			OllamaMessage{Role: "assistant", Content: "Capability memory received. I will use it only to avoid repeating false capability limitations."},
		)
	}
	if historyMessage := buildStructuredCommandHistoryMessage(history); historyMessage != "" {
		messages = append(messages,
			OllamaMessage{Role: "user", Content: historyMessage},
			OllamaMessage{Role: "assistant", Content: "Reference history received. I will use it only when the active task needs omitted context."},
		)
	}
	messages = append(messages, OllamaMessage{Role: "user", Content: buildStructuredCommandUserMessage(prompt, observations)})
	return messages
}

func buildStructuredCommandCapabilityMemoryMessage(memories []SessionMemory) string {
	recent := recentStructuredCapabilityMemories(memories)
	if len(recent) == 0 {
		return ""
	}
	payload := struct {
		CapabilityMemory []SessionMemory `json:"capability_memory"`
	}{
		CapabilityMemory: recent,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(blob)
}

func buildStructuredCommandHistoryMessage(history []Message) string {
	recent := recentStructuredMemoryRecords(history)
	if len(recent) == 0 {
		return ""
	}
	payload := struct {
		ReferenceHistory []StructuredMemoryRecord `json:"reference_history"`
	}{
		ReferenceHistory: recent,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(blob)
}

func buildStructuredCommandUserMessage(prompt string, observations []StructuredCommandObservation) string {
	payload := struct {
		ActivePromptOpen string                  `json:"active_prompt_open"`
		ToolInventory    StructuredToolInventory `json:"tool_inventory"`
		ActiveTask       struct {
			CurrentPrompt               string                         `json:"current_prompt"`
			Prompt                      string                         `json:"prompt"`
			CurrentWorkingDirectory     string                         `json:"current_working_directory"`
			MustReturnCommand           bool                           `json:"must_return_command"`
			RealCommandObservationCount int                            `json:"real_command_observation_count"`
			SuccessfulCommandCount      int                            `json:"successful_command_count"`
			FailedCommandCount          int                            `json:"failed_command_count"`
			AttemptBudgetRemaining      int                            `json:"attempt_budget_remaining"`
			Observations                []StructuredCommandObservation `json:"observations"`
		} `json:"active_task"`
		ActivePromptClose string `json:"active_prompt_close"`
	}{}
	payload.ActivePromptOpen = prompt
	payload.ToolInventory = buildStructuredToolInventory()
	payload.ActiveTask.CurrentPrompt = prompt
	payload.ActiveTask.Prompt = prompt
	payload.ActiveTask.CurrentWorkingDirectory = currentWorkingDirectoryForStructuredPrompt()
	payload.ActiveTask.MustReturnCommand = !hasRealCommandObservation(observations)
	payload.ActiveTask.RealCommandObservationCount = realCommandObservationCount(observations)
	payload.ActiveTask.SuccessfulCommandCount = successfulCommandObservationCount(observations)
	payload.ActiveTask.FailedCommandCount = failedCommandObservationCount(observations)
	payload.ActiveTask.AttemptBudgetRemaining = maxInt(0, defaultCommandDecisionMaxSteps-len(observations))
	payload.ActiveTask.Observations = observations
	payload.ActivePromptClose = prompt
	blob, err := json.Marshal(payload)
	if err != nil {
		return prompt
	}
	return string(blob)
}

type StructuredToolInventory struct {
	TerminalTools  []string                 `json:"terminal_tools"`
	Skills         []string                 `json:"skills,omitempty"`
	PublicSources  []string                 `json:"public_sources"`
	LLMRoles       []string                 `json:"llm_roles"`
	SpecialistTeam []specialist.TeamProfile `json:"specialist_team"`
	ShellRules     []string                 `json:"shell_rules"`
}

func buildStructuredToolInventory() StructuredToolInventory {
	return StructuredToolInventory{
		TerminalTools: discoveredTerminalTools(),
		Skills:        discoveredSkillNames(),
		PublicSources: []string{
			"wttr.in",
			"news.google.com/rss/search",
			"duckduckgo.com/html",
			"go.dev/dl/?mode=json",
		},
		LLMRoles: []string{
			"command_planner",
			"shell_execution_specialist",
			"final_responder",
			"memory_retriever",
			"memory_reviewer",
			"web_researcher",
			"workspace_researcher",
			"subtask_executor",
			"verifier",
		},
		SpecialistTeam: specialist.DefaultTeam(),
		ShellRules: []string{
			"single fresh bash shell per command",
			"working directory does not persist between commands",
			"use absolute paths or cd within the same command",
			"for Thailand current time use TZ=Asia/Bangkok date '+%Y-%m-%d %H:%M:%S %Z'",
			"stdout stderr and exit code are observed after execution",
		},
	}
}

func discoveredTerminalTools() []string {
	candidates := []string{
		"bash", "sh", "curl", "python3", "sed", "awk", "grep", "rg", "jq", "date", "uname",
		"cat", "find", "ls", "pwd", "mkdir", "touch", "tee", "git", "go", "npm", "node",
		"docker", "ps", "pgrep", "xdg-open", "firefox", "wmctrl", "xdotool",
	}
	tools := make([]string, 0, len(candidates))
	seen := map[string]bool{}
	for _, tool := range candidates {
		if _, err := exec.LookPath(tool); err == nil && !seen[tool] {
			tools = append(tools, tool)
			seen[tool] = true
		}
	}
	sort.Strings(tools)
	return tools
}

func discoveredSkillNames() []string {
	root := findStructuredSkillsRoot()
	if root == "" {
		return nil
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil
	}
	skills := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err == nil {
			skills = append(skills, name)
		}
	}
	sort.Strings(skills)
	return skills
}

func findStructuredSkillsRoot() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(wd, "skills")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		next := filepath.Dir(wd)
		if next == wd {
			return ""
		}
		wd = next
	}
}

type StructuredMemoryRecord struct {
	Turn        int    `json:"turn"`
	Role        string `json:"role"`
	NotPrompt   bool   `json:"not_prompt"`
	MemoryStyle string `json:"memory_style"`
	MemoryNote  string `json:"memory_note"`
}

func recentStructuredCapabilityMemories(memories []SessionMemory) []SessionMemory {
	if len(memories) == 0 {
		return nil
	}
	start := 0
	if len(memories) > maxConversationHistoryMessages {
		start = len(memories) - maxConversationHistoryMessages
	}
	out := []SessionMemory{}
	for _, memory := range memories[start:] {
		if strings.TrimSpace(memory.Content) == "" {
			continue
		}
		kind := strings.TrimSpace(memory.Kind)
		if kind == "" {
			kind = "capability"
		}
		out = append(out, SessionMemory{
			Kind:      kind,
			Content:   truncateStructuredObservation(memory.Content),
			Tags:      sortedCopy(memory.Tags),
			CreatedAt: memory.CreatedAt,
		})
	}
	return out
}

func recentStructuredMemoryRecords(history []Message) []StructuredMemoryRecord {
	recent := recentStructuredConversation(history)
	if len(recent) == 0 {
		return nil
	}
	out := make([]StructuredMemoryRecord, 0, len(recent))
	for i, msg := range recent {
		out = append(out, StructuredMemoryRecord{
			Turn:        i + 1,
			Role:        msg.Role,
			NotPrompt:   true,
			MemoryStyle: "terse_reference_only",
			MemoryNote:  compactStructuredMemoryNote(msg.Content),
		})
	}
	return out
}

func compactStructuredMemoryNote(content string) string {
	note := strings.Join(strings.Fields(content), " ")
	if len(note) <= 320 {
		return note
	}
	return note[:320] + " [truncated]"
}

func recentStructuredConversation(history []Message) []Message {
	if len(history) == 0 {
		return nil
	}
	start := 0
	if len(history) > maxConversationHistoryMessages {
		start = len(history) - maxConversationHistoryMessages
	}
	out := make([]Message, 0, len(history)-start)
	for _, msg := range history[start:] {
		role := strings.TrimSpace(msg.Role)
		content := strings.TrimSpace(msg.Content)
		if role == "" || content == "" {
			continue
		}
		out = append(out, Message{
			Role:      role,
			Content:   truncateStructuredObservation(content),
			CreatedAt: msg.CreatedAt,
		})
	}
	return out
}

func currentWorkingDirectoryForStructuredPrompt() string {
	wd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return wd
}

func realCommandObservationCount(observations []StructuredCommandObservation) int {
	count := 0
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" {
			count++
		}
	}
	return count
}

func successfulCommandObservationCount(observations []StructuredCommandObservation) int {
	count := 0
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" && obs.ExitCode == 0 {
			count++
		}
	}
	return count
}

func failedCommandObservationCount(observations []StructuredCommandObservation) int {
	count := 0
	for _, obs := range observations {
		if strings.TrimSpace(obs.Command) != "" && obs.ExitCode != 0 {
			count++
		}
	}
	return count
}

func truncateStructuredObservation(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) <= defaultStructuredObservationChars {
		return trimmed
	}
	return trimmed[:defaultStructuredObservationChars] + "\n[truncated]"
}

func truncateStructuredTimelineValue(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) <= 400 {
		return trimmed
	}
	return trimmed[:400] + "..."
}

func ParseStructuredCommandPayload(raw string) (StructuredCommandPayload, error) {
	var decoded struct {
		Command  *string `json:"command"`
		Done     *bool   `json:"done"`
		Answer   *string `json:"answer"`
		Ask      bool    `json:"ask"`
		Question string  `json:"question"`
		Tool     string  `json:"tool"`
		ToolTask string  `json:"tool_task"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return StructuredCommandPayload{}, fmt.Errorf("parse structured command payload: %w", err)
	}
	if decoded.Command == nil || decoded.Done == nil || decoded.Answer == nil {
		return StructuredCommandPayload{}, fmt.Errorf("structured command payload missing required fields")
	}
	return StructuredCommandPayload{
		Command:  *decoded.Command,
		Done:     *decoded.Done,
		Answer:   *decoded.Answer,
		Ask:      decoded.Ask,
		Question: decoded.Question,
		Tool:     decoded.Tool,
		ToolTask: decoded.ToolTask,
	}, nil
}

func ParseShellCommandProposal(raw string) (ShellCommandProposal, error) {
	var decoded struct {
		Command   string `json:"command"`
		Rationale string `json:"rationale"`
	}
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		return ShellCommandProposal{}, fmt.Errorf("parse shell specialist response: %w", err)
	}
	command := strings.TrimSpace(decoded.Command)
	if command == "" {
		return ShellCommandProposal{}, fmt.Errorf("shell specialist response missing command")
	}
	return ShellCommandProposal{
		Command:   command,
		Rationale: strings.TrimSpace(decoded.Rationale),
	}, nil
}

func ExecuteStructuredCommand(ctx context.Context, command string, stdout, stderr io.Writer) (int, error) {
	cmd := exec.Command("bash", "-o", "pipefail", "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	if err := cmd.Start(); err != nil {
		return 1, err
	}
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		}
		<-done
		return 1, ctx.Err()
	}
	if err == nil {
		return 0, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		return exitErr.ExitCode(), nil
	}
	return 1, err
}
