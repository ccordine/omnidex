package odn

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

func ExecuteDeterministicPipeline(ctx context.Context, session *Session, input string, mode PermissionMode, in io.Reader, out io.Writer, registry Registry, client *OllamaClient, nextEventID func() string, runLogger *RunLogger) (string, []Event, error) {
	events := make([]Event, 0, 12)
	outcomes := make([]ToolOutcome, 0, 8)
	summaries := make([]string, 0, 8)

	routeStarted := time.Now().UTC()
	route := RouteTools(ctx, client, registry, input)
	routeDuration := time.Since(routeStarted)

	events = append(events, Event{
		ID:      nextEventID(),
		Type:    "routing_completed",
		Summary: "router_llm phase completed",
		Details: map[string]string{
			"source":         route.Source,
			"raw_output":     route.RawOutput,
			"selected_tools": strings.Join(route.SelectedTools, ","),
			"duration_ms":    fmt.Sprintf("%d", routeDuration.Milliseconds()),
		},
		CreatedAt: nowUTC(),
	})
	if strings.TrimSpace(route.ParseError) != "" {
		events = append(events, Event{
			ID:      nextEventID(),
			Type:    "routing_parse_warning",
			Summary: "Router output required deterministic fallback",
			Details: map[string]string{
				"error": route.ParseError,
			},
			CreatedAt: nowUTC(),
		})
	}

	logFields := map[string]interface{}{
		"source":         route.Source,
		"raw_output":     route.RawOutput,
		"selected_tools": strings.Join(route.SelectedTools, ","),
		"parse_error":    route.ParseError,
		"duration_ms":    routeDuration.Milliseconds(),
	}
	if route.LLMResponse != nil {
		logFields["llm_request"] = route.LLMResponse.RequestJSON
		logFields["llm_response"] = route.LLMResponse.ResponseJSON
		logFields["llm_total_duration_ns"] = route.LLMResponse.TotalDuration
		logFields["llm_prompt_eval_count"] = route.LLMResponse.PromptEvalCount
		logFields["llm_eval_count"] = route.LLMResponse.EvalCount
	}
	_ = runLogger.Log("router", "routing_completed", logFields)

	if len(route.SelectedTools) == 0 {
		return "Execution mode detected, but router selected no tools for this request.", events, nil
	}

	for _, toolID := range route.SelectedTools {
		tool, ok := registry.GetTool(toolID)
		if !ok {
			events = append(events, Event{
				ID:        nextEventID(),
				Type:      "tool_failed",
				Summary:   "Unknown tool selected by router",
				Details:   map[string]string{"tool_id": toolID},
				CreatedAt: nowUTC(),
			})
			outcomes = append(outcomes, ToolOutcome{ToolID: toolID, Status: "failed"})
			continue
		}

		events = append(events, Event{
			ID:      nextEventID(),
			Type:    "tool_dispatched",
			Summary: "Dispatching selected tool",
			Details: map[string]string{
				"tool_id":     tool.ID,
				"role_id":     tool.RoleID,
				"risk_tier":   fmt.Sprintf("%d", tool.RiskTier),
				"implemented": fmt.Sprintf("%t", tool.Implemented),
			},
			CreatedAt: nowUTC(),
		})

		if !tool.Implemented {
			events = append(events, Event{
				ID:        nextEventID(),
				Type:      "tool_unimplemented",
				Summary:   "Tool is registered but not implemented yet",
				Details:   map[string]string{"tool_id": tool.ID},
				CreatedAt: nowUTC(),
			})
			summaries = append(summaries, fmt.Sprintf("%s: not implemented yet", tool.ID))
			outcomes = append(outcomes, ToolOutcome{ToolID: tool.ID, Status: "blocked"})
			continue
		}

		switch tool.ID {
		case "scaffold_go_html_project":
			plan, ok := BuildExecutionPlan(input, session.WorkspacePath)
			if !ok {
				plan = BuildGoHTMLScaffoldPlan(session.WorkspacePath)
			}

			planEvents, err := ExecutePlan(plan, mode, in, out, session.WorkspacePath, nextEventID)
			events = append(events, planEvents...)
			if err != nil {
				summaries = append(summaries, fmt.Sprintf("%s: failed (%v)", tool.ID, err))
				outcomes = append(outcomes, ToolOutcome{ToolID: tool.ID, Status: "failed"})
				_ = runLogger.Log("tool", "tool_failed", map[string]interface{}{"tool_id": tool.ID, "error": err.Error()})
				continue
			}

			denied := false
			for _, evt := range planEvents {
				if evt.Type == "permission_denied" {
					denied = true
					break
				}
			}
			if denied {
				summaries = append(summaries, fmt.Sprintf("%s: blocked by permission", tool.ID))
				outcomes = append(outcomes, ToolOutcome{ToolID: tool.ID, Status: "blocked"})
				_ = runLogger.Log("tool", "tool_blocked", map[string]interface{}{"tool_id": tool.ID, "reason": "permission_denied"})
				continue
			}

			summaries = append(summaries, fmt.Sprintf("%s: project scaffolded", tool.ID))
			outcomes = append(outcomes, ToolOutcome{ToolID: tool.ID, Status: "success"})
			_ = runLogger.Log("tool", "tool_completed", map[string]interface{}{"tool_id": tool.ID})

		case "linux_command":
			toolCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
			linuxResult, err := ExecuteLinuxCommandTool(toolCtx, client, input, mode, in, out, session.WorkspacePath, nextEventID, runLogger)
			cancel()
			events = append(events, linuxResult.Events...)
			if err != nil {
				events = append(events, Event{
					ID:        nextEventID(),
					Type:      "tool_failed",
					Summary:   "linux_command failed",
					Details:   map[string]string{"error": err.Error()},
					CreatedAt: nowUTC(),
				})
				summaries = append(summaries, fmt.Sprintf("%s: failed (%v)", tool.ID, err))
				outcomes = append(outcomes, ToolOutcome{ToolID: tool.ID, Status: "failed"})
				_ = runLogger.Log("tool", "tool_failed", map[string]interface{}{"tool_id": tool.ID, "error": err.Error()})
				continue
			}
			summaries = append(summaries, linuxResult.Summary)
			status := "success"
			if linuxResult.FailedCount > 0 {
				status = "failed"
			} else if linuxResult.ExecutedCount == 0 {
				status = "blocked"
			}
			outcomes = append(outcomes, ToolOutcome{ToolID: tool.ID, Status: status})

		case "verification_gate":
			verifySummary, verifyEvents, passed := ExecuteVerificationTool(outcomes, nextEventID)
			events = append(events, verifyEvents...)
			summaries = append(summaries, verifySummary)
			if passed {
				outcomes = append(outcomes, ToolOutcome{ToolID: tool.ID, Status: "success"})
				_ = runLogger.Log("tool", "verification_passed", map[string]interface{}{"summary": verifySummary})
			} else {
				outcomes = append(outcomes, ToolOutcome{ToolID: tool.ID, Status: "failed"})
				_ = runLogger.Log("tool", "verification_failed", map[string]interface{}{"summary": verifySummary})
			}

		default:
			events = append(events, Event{
				ID:        nextEventID(),
				Type:      "tool_unimplemented",
				Summary:   "Tool dispatch branch missing",
				Details:   map[string]string{"tool_id": tool.ID},
				CreatedAt: nowUTC(),
			})
			summaries = append(summaries, fmt.Sprintf("%s: dispatch branch missing", tool.ID))
			outcomes = append(outcomes, ToolOutcome{ToolID: tool.ID, Status: "blocked"})
		}
	}

	finalMessage := "Execution completed."
	if len(summaries) > 0 {
		finalMessage = "Execution completed:\n- " + strings.Join(summaries, "\n- ")
	}
	return finalMessage, events, nil
}
