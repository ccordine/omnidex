package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
	"github.com/gryph/omnidex/internal/scrum"
)

func (s *Service) runExternalAgentStep(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string) error {
	cfg := agentconfig.FromJobMetadata(claim.Job.Metadata)
	agent, agentName, unavailable := selectExternalAgent(cfg)
	if agent == nil {
		msg := unavailable
		if msg == "" {
			msg = cfg.System() + " agent is not configured"
		}
		if cfg.IsStrict() || scrum.IsStrictScrumExternal(claim.Job.Metadata) {
			return fmt.Errorf("strict external agent required: %s", msg)
		}
		s.emitStepEvent(claim.Step.ID, "external_agent_unavailable", msg)
		return s.runNativeV3Step(ctx, claim, contexts, "v3_intent_parse")
	}

	workspace := codingWorkspaceForJob(claim.Job)
	prompt := buildExternalAgentPrompt(claim.Job, contexts)
	packet := omni.CursorImplementationPacket{
		Task:       strings.TrimSpace(claim.Job.Instruction),
		Mode:       "scrum_task",
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
	result, err := agent.RunArchitectTask(ctx, input)
	if err != nil {
		if cfg.IsStrict() || scrum.IsStrictScrumExternal(claim.Job.Metadata) {
			return fmt.Errorf("%s failed: %w", agentName, err)
		}
		s.emitStepEvent(claim.Step.ID, "external_agent_failed", err.Error())
		return s.runNativeV3Step(ctx, claim, contexts, "v3_intent_parse")
	}

	output := strings.TrimSpace(firstNonEmptyString(result.Summary, result.Output, "external agent completed"))
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
	return completeStep(ctx, claim.Step.ID, output, "external_agent_execute", string(summary))
}

func selectExternalAgent(cfg agentconfig.Config) (omni.CursorArchitectAgent, string, string) {
	switch cfg.System() {
	case agentconfig.SystemCursor:
		agent := omni.NewCursorSDKArchitectAgent()
		if agent == nil {
			return nil, "cursor_sdk", "Cursor SDK agent is not enabled (set OMNI_ENABLE_CURSOR_ARCHITECT=true and CURSOR_API_KEY)"
		}
		return agent, "cursor_sdk", ""
	case agentconfig.SystemCodex:
		agent := omni.NewCodexSDKArchitectAgent()
		if agent == nil {
			return nil, "codex_sdk", "Codex SDK agent is not enabled (set OMNI_ENABLE_CODEX_ARCHITECT=true and CODEX_API_KEY/OPENAI_API_KEY)"
		}
		return agent, "codex_sdk", ""
	default:
		return nil, "", "not an external agent"
	}
}

func buildExternalAgentPrompt(job model.Job, contexts map[string]string) string {
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
