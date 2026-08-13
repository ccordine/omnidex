package api

import (
	"context"
	"fmt"

	"github.com/gryph/omnidex/internal/agentconfig"
	"github.com/gryph/omnidex/internal/model"
	"github.com/gryph/omnidex/internal/omni"
)

func (s *Server) validateScrumPlayAgent(ctx context.Context, project model.Project, card ScrumCard, instance agentconfig.Config) error {
	resolved, _, err := s.resolveAgentConfig(ctx, project, card, instance)
	if err != nil {
		return err
	}
	if !resolved.IsExternal() {
		return nil
	}
	switch resolved.System() {
	case agentconfig.SystemCursor:
		agent := omni.NewCursorSDKAgent()
		if agent == nil {
			reason := omni.CursorSDKUnavailableReason()
			if reason == "" {
				reason = "Cursor SDK is not available"
			}
			return fmt.Errorf("%s\nAdd the Cursor API key under Admin → API secrets (DB) or set CURSOR_API_KEY in env.", reason)
		}
		agent.ApplyConfig(resolved)
		if ok, reason := agent.Available(); !ok {
			return fmt.Errorf("%s", reason)
		}
	case agentconfig.SystemCodex:
		agent := omni.NewCodexSDKAgent()
		if agent == nil {
			reason := omni.CodexSDKUnavailableReason()
			if reason == "" {
				reason = "Codex SDK is not available"
			}
			return fmt.Errorf("%s\nAdd the Codex API key under Admin → API secrets (DB) or set CODEX_API_KEY in env.", reason)
		}
		agent.ApplyConfig(resolved)
		if ok, reason := agent.Available(); !ok {
			return fmt.Errorf("%s", reason)
		}
	default:
		return nil
	}
	return nil
}
