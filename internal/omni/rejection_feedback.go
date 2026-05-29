package omni

import (
	"encoding/json"
	"strings"
)

type StructuredRepairContext struct {
	Feedback         string `json:"feedback,omitempty"`
	RejectedCommand  string `json:"rejected_command,omitempty"`
	RejectedResponse string `json:"rejected_response,omitempty"`
	RejectedContent  string `json:"rejected_content,omitempty"`
	Guidance         string `json:"guidance,omitempty"`
}

func latestStructuredRepairContext(observations []StructuredCommandObservation) StructuredRepairContext {
	for i := len(observations) - 1; i >= 0; i-- {
		if ctx, ok := structuredRepairContextFromObservation(observations[i]); ok {
			return ctx
		}
	}
	return StructuredRepairContext{}
}

func latestStructuredRepairFeedback(observations []StructuredCommandObservation) string {
	return latestStructuredRepairContext(observations).Feedback
}

func structuredRepairContextFromObservation(obs StructuredCommandObservation) (StructuredRepairContext, bool) {
	feedback := strings.TrimSpace(obs.Stderr)
	if feedback == "" {
		feedback = strings.TrimSpace(obs.EvaluationFeedback)
	}
	rejectedCommand := strings.TrimSpace(obs.RejectedCommand)
	rejectedResponse := strings.TrimSpace(obs.RejectedResponse)
	if feedback == "" && rejectedCommand == "" && rejectedResponse == "" {
		return StructuredRepairContext{}, false
	}
	if !isStructuredRejectionObservation(obs) {
		return StructuredRepairContext{}, false
	}
	ctx := StructuredRepairContext{
		Feedback:         truncateStructuredObservation(feedback),
		RejectedCommand:  truncateStructuredObservation(rejectedCommand),
		RejectedResponse: truncateStructuredObservation(rejectedResponse),
		Guidance:         structuredRepairGuidanceFromFeedback(feedback),
	}
	if ctx.Feedback == "" && ctx.RejectedCommand != "" {
		ctx.Feedback = "rejected command: " + ctx.RejectedCommand
	}
	if ctx.Feedback == "" && ctx.RejectedResponse != "" {
		ctx.Feedback = "rejected planner response"
	}
	return ctx, ctx.Feedback != "" || ctx.RejectedCommand != "" || ctx.RejectedResponse != ""
}

func isStructuredRejectionObservation(obs StructuredCommandObservation) bool {
	if strings.TrimSpace(obs.RejectedCommand) != "" || strings.TrimSpace(obs.RejectedResponse) != "" {
		return true
	}
	feedback := strings.ToLower(strings.TrimSpace(obs.Stderr + "\n" + obs.EvaluationFeedback))
	if feedback == "" {
		return false
	}
	rejectionNeedles := []string{
		" rejected",
		"rejected:",
		"invalid:",
		"anti_loop:",
		"self-evaluation rejected",
		"content kind rejected",
		"placeholder-only",
		"done rejected",
		"ask rejected",
		"command rejected",
		"proof_plan invalid",
		"tool delegation rejected",
		"completion repair command rejected",
		"repeated rejected content",
	}
	for _, needle := range rejectionNeedles {
		if strings.Contains(feedback, needle) {
			return true
		}
	}
	return false
}

func structuredRepairGuidanceFromFeedback(feedback string) string {
	lower := strings.ToLower(strings.TrimSpace(feedback))
	switch {
	case strings.Contains(lower, "content kind rejected"):
		return "rewrite the output to match the required file or payload kind exactly; remove every forbidden content class"
	case strings.Contains(lower, "repeated rejected content"), strings.Contains(lower, "repeated command"):
		return "do not repeat the rejected output; choose a materially different action that satisfies the validator feedback"
	case strings.Contains(lower, "placeholder-only"):
		return "replace placeholder-only output with substantive source, build, or test content"
	case strings.Contains(lower, "done rejected"):
		return "do not return done=true yet; gather missing evidence or perform the next required action"
	case strings.Contains(lower, "ask rejected"):
		return "do not ask the user to run agent commands manually; inspect, delegate, or return a concrete command"
	case strings.Contains(lower, "self-evaluation rejected"):
		return "repair the rejected response directly using evaluator feedback as authoritative guidance"
	default:
		return "repair the rejected output using validator feedback as authoritative guidance"
	}
}

func buildStructuredPlannerRepairFollowUpMessages(repair StructuredRepairContext) []OllamaMessage {
	if repair.Feedback == "" && repair.RejectedCommand == "" && repair.RejectedResponse == "" {
		return nil
	}
	rejectedPayload := strings.TrimSpace(repair.RejectedResponse)
	if rejectedPayload == "" && repair.RejectedCommand != "" {
		rejectedPayload, _ = marshalJSONStringMap(map[string]string{
			"command": repair.RejectedCommand,
			"done":    "false",
			"answer":  "",
		})
	}
	if rejectedPayload == "" {
		return nil
	}
	repairPayload := struct {
		RepairFeedback   string   `json:"repair_feedback"`
		RejectedCommand  string   `json:"rejected_command,omitempty"`
		RejectedResponse string   `json:"rejected_response_preview,omitempty"`
		RepairRules      []string `json:"repair_rules"`
	}{
		RepairFeedback:   repair.Feedback,
		RejectedCommand:  repair.RejectedCommand,
		RejectedResponse: repair.RejectedResponse,
		RepairRules: []string{
			"Return JSON only with the same structured command schema.",
			"The repair_feedback is authoritative for this retry.",
			"Repair the rejected payload directly; do not restate or argue with the feedback.",
			"Choose a command, tool delegation, or patch that visibly corrects the rejected pattern.",
			"Do not return the same rejected response.",
		},
	}
	if strings.TrimSpace(repair.Guidance) != "" {
		repairPayload.RepairRules = append(repairPayload.RepairRules, repair.Guidance)
	}
	repairBlob, err := json.Marshal(repairPayload)
	if err != nil {
		repairBlob = []byte(`{"repair_feedback":"repair rejected planner payload"}`)
	}
	return []OllamaMessage{
		{Role: "assistant", Content: rejectedPayload},
		{Role: "user", Content: string(repairBlob)},
	}
}

func buildSpecialistRepairFollowUpMessages(repairFeedback, rejectedPreview string, repairRules []string) []OllamaMessage {
	if strings.TrimSpace(repairFeedback) == "" || strings.TrimSpace(rejectedPreview) == "" {
		return nil
	}
	rejectedPayload, err := json.Marshal(map[string]string{
		"content":   truncateStructuredTimelineValue(rejectedPreview),
		"rationale": "previous attempt rejected by validator",
	})
	if err != nil {
		rejectedPayload = []byte(`{"content":"","rationale":"previous attempt rejected by validator"}`)
	}
	if len(repairRules) == 0 {
		repairRules = []string{
			"The validator feedback is authoritative for this repair attempt.",
			"Repair the rejected output directly; do not return the same rejected content.",
		}
	}
	repairBlob, err := json.Marshal(struct {
		RepairFeedback         string   `json:"repair_feedback"`
		RejectedContentPreview string   `json:"rejected_content_preview"`
		RepairRules            []string `json:"repair_rules"`
	}{
		RepairFeedback:         repairFeedback,
		RejectedContentPreview: truncateStructuredTimelineValue(rejectedPreview),
		RepairRules:            repairRules,
	})
	if err != nil {
		repairBlob = []byte(`{"repair_feedback":"repair rejected specialist output"}`)
	}
	return []OllamaMessage{
		{Role: "assistant", Content: string(rejectedPayload)},
		{Role: "user", Content: string(repairBlob)},
	}
}

func buildShellRepairFollowUpMessages(repairFeedback, rejectedCommand string) []OllamaMessage {
	if strings.TrimSpace(repairFeedback) == "" || strings.TrimSpace(rejectedCommand) == "" {
		return nil
	}
	rejectedPayload, err := json.Marshal(map[string]string{
		"command":   truncateStructuredTimelineValue(rejectedCommand),
		"rationale": "previous attempt rejected by validator",
	})
	if err != nil {
		rejectedPayload = []byte(`{"command":"","rationale":"previous attempt rejected by validator"}`)
	}
	repairBlob, err := json.Marshal(struct {
		RepairFeedback  string   `json:"repair_feedback"`
		RejectedCommand string   `json:"rejected_command"`
		RepairRules     []string `json:"repair_rules"`
	}{
		RepairFeedback:  repairFeedback,
		RejectedCommand: truncateStructuredTimelineValue(rejectedCommand),
		RepairRules: []string{
			"Return JSON only with schema {\"command\":\"...\",\"rationale\":\"...\"}.",
			"The validator feedback is authoritative for this repair attempt.",
			"Repair the rejected command directly; do not return the same rejected command.",
			"Choose a materially different command that satisfies tool_task and corrects the rejection reason.",
		},
	})
	if err != nil {
		repairBlob = []byte(`{"repair_feedback":"repair rejected shell command"}`)
	}
	return []OllamaMessage{
		{Role: "assistant", Content: string(rejectedPayload)},
		{Role: "user", Content: string(repairBlob)},
	}
}

func marshalJSONStringMap(values map[string]string) (string, error) {
	blob, err := json.Marshal(values)
	if err != nil {
		return "", err
	}
	return string(blob), nil
}
