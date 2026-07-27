package worker

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/gryph/omnidex/internal/artifacts"
)

func subtaskToolCorrectionDirective(records []subtaskToolRecord) string {
	if len(records) == 0 {
		return ""
	}
	latest := records[len(records)-1]
	if latest.Result.Tool == subtaskDecisionContractTool {
		count, _ := latest.Result.Output["tool_call_count"].(int)
		toolNames := rejectedDecisionToolNames(latest)
		lines := []string{
			"DECISION REJECTED",
			fmt.Sprintf("Your last response returned %d tool calls; this stateful loop accepts exactly one action before it refreshes workspace state.", count),
			"Reason: " + trimForBudget(latest.Result.Error, 1200),
			"No tool ran and the workspace was not changed by that response.",
		}
		if len(toolNames) > 0 {
			lines = append(lines, "Next action: Return exactly one "+toolNames[0]+" call now.")
			if len(toolNames) > 1 {
				lines = append(lines, "Do not include "+strings.Join(toolNames[1:], ", ")+" in the same response; it has not run.")
			}
		} else {
			lines = append(lines, "Next action: Return one concrete action in a valid JSON envelope now.")
		}
		lines = append(lines, "After fresh server feedback, choose the next single action from the observed result. Do not restart or discard current files.")
		if repeatedDecisionContractRejection(records) {
			lines = append([]string{"LOOP DETECTED: the same response-shape error happened again. Follow the one-action instruction exactly."}, lines...)
		}
		return strings.Join(lines, "\n")
	}
	if latest.Result.Tool == subtaskCompletionGateTool && strings.TrimSpace(latest.Result.Error) != "" {
		return strings.Join([]string{
			"COMPLETION NOT VERIFIED",
			"The server rejected your success claim.",
			"Unmet evidence: " + trimForBudget(latest.Result.Error, 1800),
			"Do not restart or discard working files.",
			"Next action: Address each listed gap with one concrete patch or verification command, then reassess the full acceptance criteria.",
		}, "\n")
	}
	if !latest.Result.Accepted && strings.TrimSpace(latest.Result.Error) != "" {
		reason := strings.TrimSpace(latest.Result.Error)
		lines := []string{
			"CORRECTION REQUIRED",
			fmt.Sprintf("Your last %s call was rejected.", safeLine(latest.Call.Tool, "tool")),
			"Reason: " + trimForBudget(reason, 1800),
			"No workspace mutation or successful command resulted from it.",
			"Do not repeat that call.",
			"Next action: " + rejectedToolNextAction(latest),
			"Required afterward: complete the assigned objective and obtain successful verification evidence before returning success.",
		}
		if repeatedRejectedToolCall(records) {
			lines = append([]string{"LOOP DETECTED: You repeated the same rejected call. Stop repeating it and follow the specific next action below."}, lines...)
		}
		return strings.Join(lines, "\n")
	}
	if latest.Result.Tool == "command.run" && toolResultFailed(latest.Result) {
		command := subtaskCommandText(latest.Call)
		lines := []string{
			"CORRECTION REQUIRED",
			"Your last verification command failed: " + command,
		}
		if exitCode, ok := latest.Result.Output["exit_code"]; ok {
			lines = append(lines, fmt.Sprintf("Observed exit code: %v", exitCode))
		}
		if stderr := toolResultText(latest.Result.Output, "stderr"); stderr != "" {
			lines = append(lines, "Observed stderr:\n"+trimForBudget(stderr, 1800))
		}
		if stdout := toolResultText(latest.Result.Output, "stdout"); stdout != "" {
			lines = append(lines, "Observed stdout:\n"+trimForBudget(stdout, 1800))
		}
		lines = append(lines,
			"Do not claim success or rerun the unchanged implementation.",
			"Next action: Write the corrected complete file that fixes the observed failure, then run "+command+" again.",
		)
		return strings.Join(lines, "\n")
	}
	return ""
}

func repeatedDecisionContractRejection(records []subtaskToolRecord) bool {
	if len(records) < 2 {
		return false
	}
	return records[len(records)-1].Result.Tool == subtaskDecisionContractTool &&
		records[len(records)-2].Result.Tool == subtaskDecisionContractTool
}

func subtaskToolTurnDirective(records []subtaskToolRecord) string {
	if correction := subtaskToolCorrectionDirective(records); correction != "" {
		return correction
	}
	if len(records) == 0 {
		return "Inspect the current workspace snapshot and take one concrete implementation action now. Create complete working code, not advice, placeholders, or a description of future work."
	}
	latest := records[len(records)-1]
	switch latest.Result.Tool {
	case "workspace.write":
		return "The last complete-file write succeeded. Build on the files in current_workspace. If the implementation is incomplete, write the next complete file; if it is complete, run the required verification command now."
	case "command.run":
		command := subtaskCommandText(latest.Call)
		if strings.Contains(command, " mod init ") {
			return "Initialization succeeded. Build on the exact generated files shown in current_workspace. Leave the generated manifest unchanged unless the objective requires a manifest change, and implement complete working application files now."
		}
		if succeeded, _ := latest.Result.Output["succeeded"].(bool); succeeded && verificationReportsNoTests(toolResultText(latest.Result.Output, "stdout")+"\n"+toolResultText(latest.Result.Output, "stderr")) {
			return "The command exited successfully but reported no tests. That does not satisfy a tested application objective. Add focused tests for success and failure behavior, then run the test command again."
		}
		if succeeded, _ := latest.Result.Output["succeeded"].(bool); succeeded {
			return "The command succeeded. Check every acceptance criterion against current_workspace. If anything is missing, patch it and verify again; return success only when the full objective is implemented and verified."
		}
	}
	return "Use current_workspace and the latest observed result to take the next concrete action toward the unmet objective. Do not restart or discard working files."
}

func rejectedToolNextAction(record subtaskToolRecord) string {
	tool := strings.TrimSpace(record.Call.Tool)
	reason := strings.ToLower(strings.TrimSpace(record.Result.Error))
	command := subtaskCommandText(record.Call)
	switch {
	case tool == "workspace.write" && strings.Contains(reason, "create target already exists"):
		return "The file already exists. Submit one workspace.write call with operation replace and the complete intended file content."
	case tool == "workspace.write" && strings.Contains(reason, "replace target is unavailable"):
		return "The file does not exist. Submit one workspace.write call with operation create and the complete intended file content."
	case tool == "workspace.write" && strings.Contains(reason, "does not change"):
		return "Do not rewrite the unchanged file. Write the next missing or corrected complete file, or run verification if implementation is complete."
	case tool == "workspace.write":
		return "Submit one corrected workspace.write call that addresses the rejection reason and contains one complete file, not a diff."
	case tool == "command.run" && strings.Contains(command, "go mod tidy"):
		return "Do not use go mod tidy as verification. Implement the requested files, then run go test ./... ."
	case tool == "command.run":
		return "Choose an exact allowlisted command form from Available Tools that advances or verifies the objective."
	default:
		return "Submit a materially different valid tool call that directly addresses the rejection reason."
	}
}

func repeatedRejectedToolCall(records []subtaskToolRecord) bool {
	if len(records) < 2 {
		return false
	}
	previous := records[len(records)-2]
	latest := records[len(records)-1]
	if previous.Result.Accepted || latest.Result.Accepted || strings.TrimSpace(previous.Result.Error) == "" || strings.TrimSpace(latest.Result.Error) == "" {
		return false
	}
	return toolCallFingerprint(previous.Call) == toolCallFingerprint(latest.Call)
}

func toolCallFingerprint(call artifacts.ToolCallArtifact) string {
	raw, err := json.Marshal(struct {
		Tool  string         `json:"tool"`
		Input map[string]any `json:"input"`
	}{Tool: strings.TrimSpace(call.Tool), Input: call.Input})
	if err != nil {
		return strings.TrimSpace(call.Tool) + ":unencodable"
	}
	return string(raw)
}

func toolResultFailed(result artifacts.ToolResultArtifact) bool {
	succeeded, ok := result.Output["succeeded"].(bool)
	return ok && !succeeded
}

func toolResultText(output map[string]any, key string) string {
	value, _ := output[key].(string)
	return strings.TrimSpace(value)
}

func subtaskCommandText(call artifacts.ToolCallArtifact) string {
	program, _ := call.Input["program"].(string)
	args, _ := strictV3StringArray(call.Input["args"], "args")
	command := strings.TrimSpace(strings.Join(append([]string{program}, args...), " "))
	return safeLine(command, "the verification command")
}
