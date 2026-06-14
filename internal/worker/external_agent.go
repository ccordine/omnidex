package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
	"github.com/gryph/omnidex/internal/scrum"
)

type externalAgentSessionStarter interface {
	NewExternalAgentSession(input omni.CursorArchitectAgentInput) (omni.ExternalAgentSession, error)
}

func (s *Service) runExternalAgentStep(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string) error {
	cfg := agentconfig.FromJobMetadata(claim.Job.Metadata)
	workspace := codingWorkspaceForJob(claim.Job)
	if externalAgentOptionalForGeneralChat(claim.Job, workspace) {
		s.emitStepEvent(claim.Step.ID, "external_agent_skipped", "general chat has no workspace; using native research agent")
		return s.runNativeV3Step(ctx, claim, contexts, "v3_intent_parse")
	}
	agent, agentName, unavailable := selectExternalAgent(cfg, claim.Job.Metadata)
	if agent == nil {
		msg := unavailable
		if msg == "" {
			msg = cfg.System() + " agent is not configured"
		}
		if externalAgentFailureIsFatal(cfg, claim.Job.Metadata) {
			return fmt.Errorf("strict external agent required: %s", msg)
		}
		s.emitStepEvent(claim.Step.ID, "external_agent_unavailable", msg)
		return s.runNativeV3Step(ctx, claim, contexts, "v3_intent_parse")
	}

	prompt := buildExternalAgentPrompt(claim.Job, contexts)
	mode := externalAgentJobMode(claim.Job)
	packet := omni.CursorImplementationPacket{
		Task:       strings.TrimSpace(claim.Job.Instruction),
		Mode:       mode,
		Workspace:  workspace,
		TargetRoot: workspace,
		Objectives: []string{strings.TrimSpace(claim.Job.Instruction)},
	}
	input := omni.CursorArchitectAgentInput{
		Step:       1,
		UserPrompt: prompt,
		ToolTask:   claim.Job.Instruction,
		Packet:     packet,
		Workspace:  workspace,
	}

	s.emitStepEvent(claim.Step.ID, "external_agent_started", agentName)

	var result omni.CursorArchitectAgentResult
	var err error
	streamLines := make([]string, 0, 64)
	if starter, ok := agent.(externalAgentSessionStarter); ok && s.repo != nil {
		session, sessionErr := starter.NewExternalAgentSession(input)
		if sessionErr != nil {
			err = sessionErr
		} else {
			result, err = omni.StreamExternalAgentSession(ctx, session, omni.ExternalAgentJob{
				SessionID: agentName,
				Agent:     strings.TrimSuffix(agentName, "_sdk"),
				Mode:      "implementation",
				Packet:    packet,
				Prompt:    prompt,
				Workspace: workspace,
			}, func(event omni.AgentEvent) error {
				line := omni.AgentEventJSONLine(event)
				if line == "" {
					return nil
				}
				streamLines = append(streamLines, line)
				appendCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				if appendErr := s.repo.AppendStepOutput(appendCtx, claim.Step.ID, line); appendErr != nil {
					return appendErr
				}
				if s.onJobOutput != nil {
					s.onJobOutput(claim.Job.ID, line+"\n")
				}
				return nil
			})
		}
	} else {
		result, err = agent.RunArchitectTask(ctx, input)
	}

	if err != nil {
		if externalAgentFailureIsFatal(cfg, claim.Job.Metadata) {
			return fmt.Errorf("%s failed: %w", agentName, err)
		}
		s.emitStepEvent(claim.Step.ID, "external_agent_failed", err.Error())
		return s.runNativeV3Step(ctx, claim, contexts, "v3_intent_parse")
	}
	if err := omni.ExternalAgentResultError(result); err != nil {
		if externalAgentFailureIsFatal(cfg, claim.Job.Metadata) {
			return fmt.Errorf("%s failed: %w", agentName, err)
		}
		s.emitStepEvent(claim.Step.ID, "external_agent_failed", err.Error())
		return s.runNativeV3Step(ctx, claim, contexts, "v3_intent_parse")
	}

	output := strings.TrimSpace(firstNonEmptyString(result.Summary, result.Output, "external agent completed"))
	stepOutput := output
	if len(streamLines) > 0 {
		transcript := strings.TrimSpace(strings.Join(streamLines, "\n"))
		if transcript != "" {
			if output != "" && !strings.Contains(transcript, output) {
				stepOutput = transcript + "\n" + output
			} else {
				stepOutput = transcript
			}
		}
	}
	summary, _ := json.Marshal(map[string]any{
		"agent":    agentName,
		"system":   cfg.System(),
		"strict":   cfg.IsStrict(),
		"agent_id": result.AgentID,
		"run_id":   result.RunID,
		"summary":  output,
	})
	completeStep := s.completeStep
	if completeStep == nil {
		if s.repo == nil {
			return fmt.Errorf("external agent step completer is nil")
		}
		completeStep = s.repo.CompleteStep
	}
	s.emitStepEvent(claim.Step.ID, "external_agent_completed", output)
	return completeStep(ctx, claim.Step.ID, stepOutput, "external_agent_execute", string(summary))
}

func externalAgentFailureIsFatal(cfg agentconfig.Config, metadata json.RawMessage) bool {
	return cfg.IsExternal() || cfg.IsStrict() || scrum.IsStrictScrumExternal(metadata)
}

func selectExternalAgent(cfg agentconfig.Config, metadata json.RawMessage) (omni.CursorArchitectAgent, string, string) {
	explicit := cfg.IsExternal()
	switch cfg.System() {
	case agentconfig.SystemCursor:
		agent := omni.NewCursorSDKArchitectAgent(explicit)
		if agent == nil {
			reason := omni.CursorSDKUnavailableReason(explicit)
			if reason == "" {
				reason = "Cursor SDK agent is not available"
			}
			return nil, "cursor_sdk", reason
		}
		agent.ApplyConfig(cfg)
		return agent, "cursor_sdk", ""
	case agentconfig.SystemCodex:
		agent := omni.NewCodexSDKArchitectAgent(explicit)
		if agent == nil {
			reason := omni.CodexSDKUnavailableReason(explicit)
			if reason == "" {
				reason = "Codex SDK agent is not available"
			}
			return nil, "codex_sdk", reason
		}
		agent.ApplyConfig(cfg)
		return agent, "codex_sdk", ""
	default:
		return nil, "", "not an external agent"
	}
}

func buildExternalAgentPrompt(job model.Job, contexts map[string]string) string {
	if externalAgentJobMode(job) != "scrum_task" {
		return buildGenericExternalAgentPrompt(job, contexts)
	}
	lines := []string{
		"You are executing a bounded scrum card task inside an Omnidex-managed project workspace.",
		"Use the card context below. Do not ask the user to run Omnidex commands manually.",
	}
	lines = append(lines, scrum.ContextLinesFromMetadata(job.Metadata)...)
	if executionAgent := metadataString(job.Metadata, "execution_agent"); executionAgent != "" {
		lines = append(lines, "Execution agent: "+executionAgent)
	}
	if feedback := strings.TrimSpace(contexts["user_feedback"]); feedback != "" {
		lines = append(lines, "Feedback:", feedback)
	}
	lines = append(lines, "", "Task:", strings.TrimSpace(job.Instruction), "", scrum.AgentStatusFooter)
	return strings.Join(lines, "\n")
}

func externalAgentJobMode(job model.Job) string {
	if scrum.IsScrumJob(job.Metadata) || strings.EqualFold(strings.TrimSpace(job.Pipeline), "scrum") {
		return "scrum_task"
	}
	return "cli_agent_task"
}

func buildGenericExternalAgentPrompt(job model.Job, contexts map[string]string) string {
	lines := []string{
		"You are executing a bounded CLI agent task inside an Omnidex-managed workspace.",
		"Use the job context below. Do not ask the user to run Omnidex commands manually.",
		"Treat Omnidex as the control plane: perform the implementation work, stream concrete progress, and leave validation to Omnidex when proof commands are configured.",
	}
	if executionAgent := metadataString(job.Metadata, "execution_agent"); executionAgent != "" {
		lines = append(lines, "Execution agent: "+executionAgent)
	}
	if workspace := codingWorkspaceForJob(job); workspace != "" {
		lines = append(lines, "Workspace: "+workspace)
	}
	if feedback := strings.TrimSpace(contexts["user_feedback"]); feedback != "" {
		lines = append(lines, "Feedback:", feedback)
	}
	if environment := strings.TrimSpace(contexts["environment"]); environment != "" {
		lines = append(lines, "Environment:", trimForBudget(environment, 1600))
	}
	if tooling := strings.TrimSpace(contexts["tooling"]); tooling != "" {
		lines = append(lines, "Tooling:", trimForBudget(tooling, 1600))
	}
	lines = append(lines,
		"",
		"Task:",
		strings.TrimSpace(job.Instruction),
		"",
		"Completion rule: report what changed and what verification you ran or could not run. Do not claim Omnidex accepted the work.",
	)
	return strings.Join(lines, "\n")
}

func metadataStringSlice(metadata json.RawMessage, key string) []string {
	raw := metadataString(metadata, key)
	if raw == "" {
		return nil
	}
	var items []string
	if err := json.Unmarshal([]byte(raw), &items); err == nil {
		return items
	}
	return strings.Split(raw, ",")
}

func externalAgentOptionalForGeneralChat(job model.Job, workspace string) bool {
	if !strings.EqualFold(strings.TrimSpace(job.Pipeline), model.PipelineChat) {
		return false
	}
	if strings.TrimSpace(workspace) != "" {
		return false
	}
	return metadataString(job.Metadata, "source") == "omni-web-chat" && metadataString(job.Metadata, "project_directory") == ""
}
