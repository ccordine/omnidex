package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
)

func (s *Service) runExternalAgentStep(ctx context.Context, claim *model.ClaimedStep, contexts map[string]string) error {
	cfg := agentconfig.FromJobMetadata(claim.Job.Metadata)
	agent, agentName, unavailable := selectExternalAgent(cfg)
	if agent == nil {
		msg := unavailable
		if msg == "" {
			msg = cfg.System() + " agent is not configured"
		}
		if cfg.IsStrict() {
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
		if cfg.IsStrict() {
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
		"You are executing a bounded task inside an Omnidex-managed project workspace.",
		"Use the project context below. Do not ask the user to run Omnidex commands manually.",
	}
	if title := metadataString(job.Metadata, "scrum_card_title"); title != "" {
		lines = append(lines, "Scrum card: "+title)
	}
	if cardID := metadataString(job.Metadata, "scrum_card_id"); cardID != "" {
		lines = append(lines, "Card ID: "+cardID)
	}
	if dir := metadataString(job.Metadata, "project_directory"); dir != "" {
		lines = append(lines, "Project directory: "+dir)
	}
	if refs := metadataStringSlice(job.Metadata, "ref_files"); len(refs) > 0 {
		lines = append(lines, "Reference files:", strings.Join(refs, "\n"))
	}
	if feedback := strings.TrimSpace(contexts["user_feedback"]); feedback != "" {
		lines = append(lines, "Feedback:", feedback)
	}
	lines = append(lines, "", "Task:", strings.TrimSpace(job.Instruction))
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
