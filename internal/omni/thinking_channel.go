package omni

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gryph/omnidex/internal/websearch"
)

const (
	defaultThinkingMaxSteps   = 8
	defaultThinkingNumCtx     = 8192
	defaultThinkingModel      = defaultOllamaThinkingModel
	thoughtTimelineDetailLimit = 2000
)

// ThoughtMessage is one entry in an isolated internal thought channel.
// It is never appended to Session.Messages.
type ThoughtMessage struct {
	ChannelID string `json:"channel_id"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	ToolName  string `json:"tool_name,omitempty"`
	CreatedAt string `json:"created_at"`
}

// ThoughtChannel is a self-contained reasoning session the pilot can fork at will.
type ThoughtChannel struct {
	ID              string           `json:"id"`
	TurnID          string           `json:"turn_id,omitempty"`
	Trigger         string           `json:"trigger"`
	ParentChannelID string           `json:"parent_channel_id,omitempty"`
	Messages        []ThoughtMessage `json:"messages"`
	Concluded       bool             `json:"concluded"`
	Conclusion      string           `json:"conclusion,omitempty"`
	RecoveryToolTask string          `json:"recovery_tool_task,omitempty"`
	ResearchQuery   string           `json:"research_query,omitempty"`
	CreatedAt       string           `json:"created_at"`
	UpdatedAt       string           `json:"updated_at"`
}

type ThinkingInput struct {
	TurnID          string
	Step            int
	Trigger         string
	UserPrompt      string
	ToolTask        string
	WorkingDir      string
	GateReason      string
	LatestRejection string
	Observations    []StructuredCommandObservation
	LoopState       StructuredLoopState
	ObjectiveLedger []StructuredObjective
	SessionMemories []SessionMemory
	PrepContext     PrepContextBundle
	ProjectFileMap  ProjectFileMap
	ActivePrompt    ActivePromptContext
}

type ThinkingResult struct {
	ChannelID         string
	Conclusion        string
	Action            ThinkingAction
	DirectAnswer      string
	ExecutionPrompt   string
	ExecutionToolTask string
	RecoveryToolTask  string
	ResearchQuery     string
	Messages          []ThoughtMessage
}

type ThinkingService interface {
	OrchestrateTurn(ctx context.Context, input ThinkingInput, onEvent func(StructuredCommandEvent)) (ThinkingTurnOutcome, error)
	Reason(ctx context.Context, input ThinkingInput, onEvent func(StructuredCommandEvent)) (ThinkingResult, error)
}

type OllamaThinkingService struct {
	Client   CommandDecisionClient
	Store    *ThoughtChannelStore
	MaxSteps int
	Deps     ThinkingToolDeps
}

func NewOllamaThinkingService(client CommandDecisionClient, store *ThoughtChannelStore, maxSteps int) OllamaThinkingService {
	if maxSteps <= 0 {
		maxSteps = defaultThinkingMaxSteps
	}
	return OllamaThinkingService{Client: client, Store: store, MaxSteps: maxSteps}
}

func (s OllamaThinkingService) runChannel(ctx context.Context, input ThinkingInput, mode thinkingChannelMode, maxSteps int, onEvent func(StructuredCommandEvent)) (ThinkingResult, error) {
	if s.Client == nil {
		return ThinkingResult{}, fmt.Errorf("thinking service client is required")
	}
	if maxSteps <= 0 {
		maxSteps = s.maxSteps()
	}
	channel := ThoughtChannel{
		ID:        newThoughtChannelID(input.Trigger),
		TurnID:    strings.TrimSpace(input.TurnID),
		Trigger:   strings.TrimSpace(input.Trigger),
		CreatedAt: nowUTC(),
		UpdatedAt: nowUTC(),
	}
	if s.Store != nil {
		_ = s.Store.AppendChannel(channel)
	}
	startSummary := "Internal thought channel opened"
	if mode == thinkingModeEntry {
		startSummary = "Thinking pilot opened turn entry channel"
	}
	emitStructuredCommandEvent(onEvent, "thinking_channel_started", startSummary, map[string]string{
		"channel_id": channel.ID,
		"trigger":    channel.Trigger,
		"step":       fmt.Sprintf("%d", input.Step),
		"turn_id":    channel.TurnID,
		"mode":       string(mode),
	})

	dialogue := []OllamaMessage{{Role: "system", Content: thinkingSystemPrompt(mode)}}
	dialogue = append(dialogue, OllamaMessage{Role: "user", Content: buildThinkingContextPayload(input, mode)})

	var lastRecovery, lastResearch string
	var lastAction ThinkingAction
	var lastDirectAnswer, lastExecutionPrompt, lastExecutionToolTask string
	for step := 0; step < maxSteps; step++ {
		resp, err := s.Client.ChatRaw(ctx, OllamaChatRequest{
			Messages: dialogue,
			Format: map[string]interface{}{
				"type":       "object",
				"properties": thinkingResponseFormatProperties(),
				"required":   []string{"thought", "done"},
			},
			Options: map[string]interface{}{
				"temperature": 0.2,
				"num_predict": 640,
			},
		})
		if err != nil {
			return ThinkingResult{ChannelID: channel.ID}, err
		}

		if native := strings.TrimSpace(resp.Thinking); native != "" {
			emitStructuredCommandEvent(onEvent, "thinking_model_native", "Model native thinking captured", map[string]string{
				"channel_id": channel.ID,
				"thinking":   truncateStructuredTimelineValue(native),
			})
		}

		payload, err := parseThinkingStepPayload(resp.Content)
		if err != nil {
			payload = thinkingStepPayload{Thought: resp.Content, Done: step+1 >= maxSteps}
		}

		thoughtMsg := ThoughtMessage{
			ChannelID: channel.ID,
			Role:      "assistant",
			Content:   strings.TrimSpace(payload.Thought),
			CreatedAt: nowUTC(),
		}
		channel.Messages = append(channel.Messages, thoughtMsg)
		channel.UpdatedAt = nowUTC()
		if s.Store != nil {
			_ = s.Store.AppendMessage(thoughtMsg)
		}
		emitStructuredCommandEvent(onEvent, "thinking_step", "Internal reasoning step", map[string]string{
			"channel_id": channel.ID,
			"step":       fmt.Sprintf("%d", step+1),
			"thought":    truncateThoughtForTimeline(thoughtMsg.Content),
			"tool":       strings.TrimSpace(payload.Tool),
			"action":     strings.TrimSpace(payload.Action),
		})

		dialogue = append(dialogue, OllamaMessage{Role: "assistant", Content: resp.Content})

		if strings.TrimSpace(payload.RecoveryToolTask) != "" {
			lastRecovery = strings.TrimSpace(payload.RecoveryToolTask)
		}
		if strings.TrimSpace(payload.ResearchQuery) != "" {
			lastResearch = strings.TrimSpace(payload.ResearchQuery)
			emitStructuredCommandEvent(onEvent, "thinking_research_requested", "Thinking layer requested research", map[string]string{
				"channel_id": channel.ID,
				"query":      truncateStructuredTimelineValue(lastResearch),
			})
		}
		action, directAnswer, executionPrompt, executionToolTask := outcomeFieldsFromPayload(mode, payload, lastRecovery, lastResearch)
		if action != "" {
			lastAction = action
		}
		if directAnswer != "" {
			lastDirectAnswer = directAnswer
		}
		if executionPrompt != "" {
			lastExecutionPrompt = executionPrompt
		}
		if executionToolTask != "" {
			lastExecutionToolTask = executionToolTask
		}

		if payload.Done {
			channel.Concluded = true
			channel.Conclusion = strings.TrimSpace(payload.Conclusion)
			channel.RecoveryToolTask = lastExecutionToolTask
			channel.ResearchQuery = lastResearch
			break
		}

		toolName := strings.TrimSpace(payload.Tool)
		if toolName == "" {
			continue
		}
		toolResult := executeThinkingTool(ctx, s.Deps, toolName, payload.ToolInput, input, onEvent, channel.ID)
		toolMsg := ThoughtMessage{
			ChannelID: channel.ID,
			Role:      "tool",
			Content:   toolResult,
			ToolName:  toolName,
			CreatedAt: nowUTC(),
		}
		channel.Messages = append(channel.Messages, toolMsg)
		if s.Store != nil {
			_ = s.Store.AppendMessage(toolMsg)
		}
		emitStructuredCommandEvent(onEvent, "thinking_tool_result", "Thinking layer consulted "+toolName, map[string]string{
			"channel_id": channel.ID,
			"tool":       toolName,
			"result":     truncateThoughtForTimeline(toolResult),
		})
		dialogue = append(dialogue, OllamaMessage{Role: "user", Content: fmt.Sprintf("tool_result(%s): %s", toolName, toolResult)})
	}

	if !channel.Concluded {
		channel.Concluded = true
		channel.Conclusion = "thinking step budget exhausted without explicit done=true"
		channel.RecoveryToolTask = lastExecutionToolTask
		channel.ResearchQuery = lastResearch
	}
	channel.UpdatedAt = nowUTC()
	if s.Store != nil {
		_ = s.Store.UpdateChannel(channel)
	}

	concludeSummary := "Internal thought channel concluded"
	if mode == thinkingModeEntry {
		concludeSummary = "Thinking pilot concluded turn entry channel"
	}
	emitStructuredCommandEvent(onEvent, "thinking_channel_concluded", concludeSummary, map[string]string{
		"channel_id":           channel.ID,
		"conclusion":           truncateThoughtForTimeline(channel.Conclusion),
		"action":               string(lastAction),
		"direct_answer":        truncateThoughtForTimeline(lastDirectAnswer),
		"execution_prompt":     truncateStructuredTimelineValue(lastExecutionPrompt),
		"execution_tool_task":  truncateStructuredTimelineValue(lastExecutionToolTask),
		"recovery_tool_task":   truncateStructuredTimelineValue(channel.RecoveryToolTask),
		"research_query":       truncateStructuredTimelineValue(channel.ResearchQuery),
		"message_count":        fmt.Sprintf("%d", len(channel.Messages)),
	})

	return ThinkingResult{
		ChannelID:         channel.ID,
		Conclusion:        channel.Conclusion,
		Action:            lastAction,
		DirectAnswer:      lastDirectAnswer,
		ExecutionPrompt:   lastExecutionPrompt,
		ExecutionToolTask: lastExecutionToolTask,
		RecoveryToolTask:  channel.RecoveryToolTask,
		ResearchQuery:     channel.ResearchQuery,
		Messages:          append([]ThoughtMessage(nil), channel.Messages...),
	}, nil
}

type thinkingStepPayload struct {
	Thought            string `json:"thought"`
	Tool               string `json:"tool"`
	ToolInput          string `json:"tool_input"`
	Done               bool   `json:"done"`
	Action             string `json:"action"`
	DirectAnswer       string `json:"direct_answer"`
	ExecutionPrompt    string `json:"execution_prompt"`
	ExecutionToolTask  string `json:"execution_tool_task"`
	Conclusion         string `json:"conclusion"`
	RecoveryToolTask   string `json:"recovery_tool_task"`
	ResearchQuery      string `json:"research_query"`
}

func parseThinkingStepPayload(raw string) (thinkingStepPayload, error) {
	var payload thinkingStepPayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return thinkingStepPayload{}, err
	}
	return payload, nil
}

func buildThinkingContextPayload(input ThinkingInput, mode thinkingChannelMode) string {
	payload := struct {
		Trigger          string                         `json:"trigger"`
		Step             int                            `json:"step"`
		Mode             string                         `json:"mode"`
		ActivePrompt     ActivePromptContext            `json:"active_prompt"`
		GateReason       string                         `json:"gate_reason,omitempty"`
		LatestRejection  string                         `json:"latest_rejection,omitempty"`
		LoopState        StructuredLoopState            `json:"loop_state"`
		ObjectiveLedger  []StructuredObjective          `json:"objective_ledger,omitempty"`
		WorkingDir       string                         `json:"working_directory"`
		ToolTask         string                         `json:"tool_task,omitempty"`
		ObservationCount int                            `json:"observation_count"`
		ProjectMapOpen   []string                       `json:"project_map_open_changes,omitempty"`
		MemoryCount      int                            `json:"memory_count"`
		Instructions     []string                       `json:"instructions"`
	}{
		Trigger:          input.Trigger,
		Step:             input.Step,
		Mode:             string(mode),
		ActivePrompt:     input.ActivePrompt,
		GateReason:       input.GateReason,
		LatestRejection:  input.LatestRejection,
		LoopState:        input.LoopState,
		ObjectiveLedger:  input.ObjectiveLedger,
		WorkingDir:       input.WorkingDir,
		ToolTask:         input.ToolTask,
		ObservationCount: len(input.Observations),
		ProjectMapOpen:   input.ProjectFileMap.OpenChanges,
		MemoryCount:      len(input.SessionMemories),
		Instructions:     thinkingContextInstructions(mode),
	}
	blob, _ := json.Marshal(payload)
	return string(blob)
}

func executeThinkingTool(ctx context.Context, deps ThinkingToolDeps, toolName, toolInput string, input ThinkingInput, onEvent func(StructuredCommandEvent), channelID string) string {
	switch strings.ToLower(strings.TrimSpace(toolName)) {
	case "web_search":
		query := strings.TrimSpace(firstNonEmpty(toolInput, input.UserPrompt))
		if deps.WebSearch == nil {
			return "web_search unavailable"
		}
		emitStructuredCommandEvent(onEvent, "thinking_web_search_started", "Thinking pilot started web search", map[string]string{
			"channel_id": channelID,
			"query":      truncateStructuredTimelineValue(query),
		})
		results, err := deps.WebSearch.SearchAll(ctx, query)
		if err != nil {
			return "web_search error: " + err.Error()
		}
		contextText := websearchBuildContextSafe(results, 5000)
		emitStructuredCommandEvent(onEvent, "thinking_web_search_completed", "Thinking pilot completed web search", map[string]string{
			"channel_id": channelID,
			"query":      truncateStructuredTimelineValue(query),
			"results":    fmt.Sprintf("%d", len(results)),
		})
		return contextText
	case "memory_search":
		query := strings.TrimSpace(firstNonEmpty(toolInput, input.UserPrompt))
		if deps.MemorySearch == nil {
			return "memory_search unavailable"
		}
		emitStructuredCommandEvent(onEvent, "thinking_memory_search_started", "Thinking pilot started memory search", map[string]string{
			"channel_id": channelID,
			"query":      truncateStructuredTimelineValue(query),
		})
		text, err := deps.MemorySearch(ctx, query)
		if err != nil {
			return "memory_search error: " + err.Error()
		}
		emitStructuredCommandEvent(onEvent, "thinking_memory_search_completed", "Thinking pilot completed memory search", map[string]string{
			"channel_id": channelID,
			"query":      truncateStructuredTimelineValue(query),
		})
		return text
	case "active_prompt":
		blob, _ := json.Marshal(input.ActivePrompt)
		return string(blob)
	case "observations":
		obs := compactStructuredObservationsForContext(input.Observations, 10, 800)
		blob, _ := json.Marshal(obs)
		return string(blob)
	case "project_map":
		blob, _ := json.Marshal(input.ProjectFileMap)
		return string(blob)
	case "memories":
		memories := compactSessionMemoriesForStructuredContext(input.SessionMemories, 8, 600)
		blob, _ := json.Marshal(memories)
		return string(blob)
	case "prep_context":
		blob, _ := json.Marshal(CompactPrepContextBundle(input.PrepContext, defaultPrepContextBudgetLimit/2))
		return string(blob)
	case "loop_state":
		blob, _ := json.Marshal(input.LoopState)
		return string(blob)
	case "objectives":
		blob, _ := json.Marshal(input.ObjectiveLedger)
		return string(blob)
	default:
		return fmt.Sprintf("unknown tool %q; available: active_prompt, observations, project_map, memories, memory_search, prep_context, web_search, loop_state, objectives", toolName)
	}
}

func websearchBuildContextSafe(results []websearch.Result, limit int) string {
	if len(results) == 0 {
		return "web_search returned no results"
	}
	return websearch.BuildContext(results, limit)
}

func truncateThoughtForTimeline(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= thoughtTimelineDetailLimit {
		return value
	}
	return value[:thoughtTimelineDetailLimit] + "..."
}

func newThoughtChannelID(trigger string) string {
	slug := strings.NewReplacer(" ", "_", "/", "_", ":", "_").Replace(strings.ToLower(strings.TrimSpace(trigger)))
	if slug == "" {
		slug = "thought"
	}
	return fmt.Sprintf("%s_%d", slug, time.Now().UnixNano())
}

func thinkingModelFromEnv() string {
	return firstNonEmpty(
		os.Getenv("OMNI_THINKING_MODEL"),
		os.Getenv("OLLAMA_MODEL_THINKING"),
		os.Getenv("OLLAMA_MODEL_REASONING"),
		defaultThinkingModel,
	)
}

func thinkingNumCtxFromEnv() int {
	return envIntOrDefault("OMNI_THINKING_NUM_CTX", defaultThinkingNumCtx)
}

func thinkingMaxStepsFromEnv() int {
	steps := envIntOrDefault("OMNI_THINKING_MAX_STEPS", defaultThinkingMaxSteps)
	if steps <= 0 {
		return defaultThinkingMaxSteps
	}
	return steps
}

// ThoughtChannelStore persists thought channels outside the user chat session.
type ThoughtChannelStore struct {
	mu       sync.Mutex
	rootDir  string
	turnID   string
	filePath string
}

func NewThoughtChannelStore(rootDir, workspaceHash, turnID string) (*ThoughtChannelStore, error) {
	if strings.TrimSpace(workspaceHash) == "" {
		return nil, fmt.Errorf("workspace hash is required for thought channel store")
	}
	base := strings.TrimSpace(rootDir)
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil || strings.TrimSpace(home) == "" {
			base = ".omni/thoughts"
		} else {
			base = filepath.Join(home, ".omni", "thoughts")
		}
	}
	dir := filepath.Join(base, workspaceHash)
	if strings.TrimSpace(turnID) != "" {
		dir = filepath.Join(dir, turnID)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create thought channel directory: %w", err)
	}
	filePath := filepath.Join(dir, "channels.jsonl")
	f, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open thought channel log: %w", err)
	}
	_ = f.Close()
	return &ThoughtChannelStore{rootDir: dir, turnID: turnID, filePath: filePath}, nil
}

type thoughtStoreRecord struct {
	Kind    string          `json:"kind"`
	Channel ThoughtChannel  `json:"channel,omitempty"`
	Message ThoughtMessage  `json:"message,omitempty"`
}

func (s *ThoughtChannelStore) AppendChannel(channel ThoughtChannel) error {
	return s.appendRecord(thoughtStoreRecord{Kind: "channel_open", Channel: channel})
}

func (s *ThoughtChannelStore) UpdateChannel(channel ThoughtChannel) error {
	return s.appendRecord(thoughtStoreRecord{Kind: "channel_close", Channel: channel})
}

func (s *ThoughtChannelStore) AppendMessage(message ThoughtMessage) error {
	return s.appendRecord(thoughtStoreRecord{Kind: "message", Message: message})
}

func (s *ThoughtChannelStore) appendRecord(record thoughtStoreRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	blob, err := json.Marshal(record)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(s.filePath, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(blob, '\n'))
	return err
}

func runThinkingForRecovery(ctx context.Context, step int, prompt string, decision ProgressionDecision, cfg structuredCommandDecisionRunConfig, worksiteSurvey WorksiteSurvey, result *CommandDecisionResult, onEvent func(StructuredCommandEvent)) (string, bool) {
	if cfg.ThinkingService == nil {
		return "", false
	}
	active := NewActivePromptContext(prompt, decision.RecoveryToolTask, explicitReactAppAcceptanceCriteria(prompt, decision.RecoveryToolTask))
	projectMap := activeProjectFileMapFromResult(prompt, decision.RecoveryToolTask, cfg.CurrentWorkingDirectory, worksiteSurvey, result.Observations)
	thought, err := cfg.ThinkingService.Reason(ctx, ThinkingInput{
		TurnID:          cfg.ThoughtTurnID,
		Step:            step,
		Trigger:         "progression_gate_" + string(decision.Action),
		UserPrompt:      prompt,
		ToolTask:        decision.RecoveryToolTask,
		WorkingDir:      cfg.CurrentWorkingDirectory,
		GateReason:      decision.Reason,
		LatestRejection: latestStructuredRepairFeedback(result.Observations),
		Observations:    result.Observations,
		LoopState:       structuredLoopStateFromState(nil, result.Observations),
		ObjectiveLedger: result.ObjectiveLedger,
		SessionMemories: cfg.SessionMemories,
		PrepContext:     cfg.PrepContext,
		ProjectFileMap:  projectMap,
		ActivePrompt:    active,
	}, onEvent)
	if err != nil {
		emitStructuredCommandEvent(onEvent, "thinking_channel_failed", "Internal thought channel failed", map[string]string{
			"step":  fmt.Sprintf("%d", step),
			"error": truncateStructuredTimelineValue(err.Error()),
		})
		return "", false
	}
	if strings.TrimSpace(thought.ExecutionToolTask) == "" && strings.TrimSpace(thought.RecoveryToolTask) == "" {
		return "", false
	}
	return firstNonEmpty(thought.ExecutionToolTask, thought.RecoveryToolTask), true
}

func isThinkingTimelineEvent(eventType string) bool {
	switch eventType {
	case "thinking_pilot_started", "thinking_pilot_decision", "thinking_channel_started", "thinking_step", "thinking_tool_result", "thinking_research_requested", "thinking_web_search_started", "thinking_web_search_completed", "thinking_memory_search_started", "thinking_memory_search_completed", "thinking_channel_concluded", "thinking_channel_failed", "thinking_model_native", "thinking_recovery_adopted":
		return true
	default:
		return strings.HasPrefix(eventType, "thinking_")
	}
}
