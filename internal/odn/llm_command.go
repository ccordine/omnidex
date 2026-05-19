package odn

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

const defaultCommandDecisionTimeout = 3 * time.Minute
const defaultCommandDecisionMaxSteps = 10
const defaultStructuredObservationChars = 2400

type CommandDecisionClient interface {
	ChatRaw(ctx context.Context, req OllamaChatRequest) (OllamaChatResponse, error)
}

type StructuredCommandPayload struct {
	Command  string `json:"command"`
	Done     bool   `json:"done"`
	Answer   string `json:"answer"`
	Ask      bool   `json:"ask,omitempty"`
	Question string `json:"question,omitempty"`
}

type CommandDecisionResult struct {
	Command      string
	ExitCode     int
	Answer       string
	Observations []StructuredCommandObservation
}

type StructuredCommandObservation struct {
	Step         int    `json:"step"`
	Command      string `json:"command"`
	ExitCode     int    `json:"exit_code"`
	Stdout       string `json:"stdout"`
	Stderr       string `json:"stderr"`
	Question     string `json:"question,omitempty"`
	UserResponse string `json:"user_response,omitempty"`
}

type StructuredCommandEvent struct {
	Type    string
	Summary string
	Details map[string]string
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
	if strings.TrimSpace(prompt) == "" {
		return CommandDecisionResult{}, fmt.Errorf("prompt is empty")
	}
	if client == nil {
		return CommandDecisionResult{}, fmt.Errorf("llm client is required")
	}

	ctx, cancel := context.WithTimeout(ctx, defaultCommandDecisionTimeout)
	defer cancel()

	result := CommandDecisionResult{}
	for step := 1; step <= defaultCommandDecisionMaxSteps; step++ {
		emitStructuredCommandEvent(onEvent, "structured_llm_request_started", "Requesting next structured command decision", map[string]string{
			"step": fmt.Sprintf("%d", step),
		})
		resp, err := client.ChatRaw(ctx, buildStructuredCommandRequest(prompt, history, result.Observations))
		if err != nil {
			return result, err
		}

		payload, err := ParseStructuredCommandPayload(resp.Content)
		if err != nil {
			return result, err
		}
		emitStructuredCommandEvent(onEvent, "structured_llm_payload_received", "Structured command payload received", map[string]string{
			"step":    fmt.Sprintf("%d", step),
			"done":    fmt.Sprintf("%t", payload.Done),
			"ask":     fmt.Sprintf("%t", payload.Ask),
			"command": truncateStructuredTimelineValue(payload.Command),
		})
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
					if err := validateStructuredCommandString(payload.Command); err != nil {
						emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
							"step":   fmt.Sprintf("%d", step),
							"reason": err.Error(),
						})
						result.Observations = append(result.Observations, StructuredCommandObservation{
							Step:     step,
							ExitCode: 1,
							Stderr:   "command rejected: " + err.Error(),
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
				if err := validateStructuredCommandString(payload.Command); err != nil {
					emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
						"step":   fmt.Sprintf("%d", step),
						"reason": err.Error(),
					})
					result.Observations = append(result.Observations, StructuredCommandObservation{
						Step:     step,
						ExitCode: 1,
						Stderr:   "command rejected: " + err.Error(),
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
			result.Answer = strings.TrimSpace(payload.Answer)
			emitStructuredCommandEvent(onEvent, "structured_done_accepted", "Structured command loop accepted final answer", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"answer": truncateStructuredTimelineValue(result.Answer),
			})
			return result, nil
		}
		if strings.TrimSpace(payload.Command) == "" {
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": "empty command with done=false",
			})
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:     step,
				ExitCode: 1,
				Stderr:   "command rejected: empty command with done=false",
			})
			continue
		}
		if err := validateStructuredCommandString(payload.Command); err != nil {
			emitStructuredCommandEvent(onEvent, "structured_command_rejected", "Command rejected by structured payload validation", map[string]string{
				"step":   fmt.Sprintf("%d", step),
				"reason": err.Error(),
			})
			result.Observations = append(result.Observations, StructuredCommandObservation{
				Step:     step,
				ExitCode: 1,
				Stderr:   "command rejected: " + err.Error(),
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

func validateStructuredCommandString(command string) error {
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

func latestRealCommandSucceeded(observations []StructuredCommandObservation) bool {
	for i := len(observations) - 1; i >= 0; i-- {
		if strings.TrimSpace(observations[i].Command) == "" {
			continue
		}
		return observations[i].ExitCode == 0
	}
	return false
}

func buildStructuredCommandRequest(prompt string, history []Message, observations []StructuredCommandObservation) OllamaChatRequest {
	return OllamaChatRequest{
		Messages: []OllamaMessage{
			{
				Role: "system",
				Content: strings.Join([]string{
					"Return JSON only.",
					"Schema: {\"command\":\"shell command to execute\",\"done\":false,\"answer\":\"\"}",
					"To stop, return {\"command\":\"\",\"done\":true,\"answer\":\"brief result from observed evidence\"}.",
					"To ask the user for needed help, return {\"command\":\"\",\"done\":false,\"answer\":\"\",\"ask\":true,\"question\":\"brief specific question\"}.",
					"If must_return_command is true, done=true is invalid; return a non-empty command.",
					"If must_return_command is true, ask=true is invalid; inspect or try a command first.",
					"If the latest real command succeeded, ask=true is invalid; continue, verify, or finish from evidence.",
					"Do not return done=true until at least one command has exit_code 0.",
					"If the latest command failed, return a different command instead of done=true.",
					"Use shell commands to satisfy requests; do not answer from memory when command evidence is required.",
					"Never return an empty command when done=false.",
					"If a command fails, the failure is recorded in observations; use that context to pivot to a different command, source, or tool.",
					"If the user already answered a question, do not ask the same question again; use the observed user_response.",
					"If you ask for approval and include an approval-gated command, that command may run after the user answers.",
					"Use recent_conversation to resolve follow-up references before asking the user.",
					"If recent_conversation contains the missing subject, location, file, project, or preference, use it in the command instead of asking.",
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
					"Prefer simple curl commands that print readable evidence over fragile HTML parsing.",
					"For current time, prefer shell time/date commands or public no-key time sources.",
					"For location-specific time, produce local-time evidence for that location; do not answer from UTC unless UTC was requested.",
					"Do not use weather services as time sources.",
					"If using shell date for a location, choose an IANA timezone and prefix the command with TZ=Area/City before date.",
					"Do not pass TZ=Area/City as an argument to date.",
					"Prefer concise command output; use format/query options instead of large pages when available.",
					"No markdown.",
				}, "\n"),
			},
			{Role: "user", Content: buildStructuredCommandUserMessage(prompt, history, observations)},
		},
		Format: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command":  map[string]interface{}{"type": "string"},
				"done":     map[string]interface{}{"type": "boolean"},
				"answer":   map[string]interface{}{"type": "string"},
				"ask":      map[string]interface{}{"type": "boolean"},
				"question": map[string]interface{}{"type": "string"},
			},
			"required": []string{"command", "done", "answer"},
		},
		Options: map[string]interface{}{
			"temperature": 0,
		},
	}
}

func buildStructuredCommandUserMessage(prompt string, history []Message, observations []StructuredCommandObservation) string {
	payload := struct {
		Prompt                      string                         `json:"prompt"`
		RecentConversation          []Message                      `json:"recent_conversation,omitempty"`
		CurrentWorkingDirectory     string                         `json:"current_working_directory"`
		MustReturnCommand           bool                           `json:"must_return_command"`
		RealCommandObservationCount int                            `json:"real_command_observation_count"`
		SuccessfulCommandCount      int                            `json:"successful_command_count"`
		FailedCommandCount          int                            `json:"failed_command_count"`
		AttemptBudgetRemaining      int                            `json:"attempt_budget_remaining"`
		Observations                []StructuredCommandObservation `json:"observations"`
	}{
		Prompt:                      prompt,
		RecentConversation:          recentStructuredConversation(history),
		CurrentWorkingDirectory:     currentWorkingDirectoryForStructuredPrompt(),
		MustReturnCommand:           !hasRealCommandObservation(observations),
		RealCommandObservationCount: realCommandObservationCount(observations),
		SuccessfulCommandCount:      successfulCommandObservationCount(observations),
		FailedCommandCount:          failedCommandObservationCount(observations),
		AttemptBudgetRemaining:      maxInt(0, defaultCommandDecisionMaxSteps-len(observations)),
		Observations:                observations,
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return prompt
	}
	return string(blob)
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
