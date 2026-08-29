package api

import "fmt"

func summarizeChatDeterministicCognitionEvent(event parsedChatStepEvent) (chatProgressKind, string, bool, error) {
	switch event.Type {
	case "application_evidence_need_opened":
		fields, err := exactChatEventFields(event.Message, "need", "source", "stop")
		if err != nil {
			return "", "", true, err
		}
		need, err := requireChatEventToken(fields, "need", 256)
		if err != nil || fields["source"] != "repository" {
			return "", "", true, firstChatProgressError(err, fmt.Errorf("application evidence source must be repository"))
		}
		if _, err := requireChatEventText(fields, "stop", 512); err != nil {
			return "", "", true, err
		}
		return chatProgressRetrieval, "Acquiring repository evidence for " + need, true, nil
	case "application_evidence_need_resolved":
		fields, err := exactChatEventFields(event.Message, "need", "facts", "stop")
		if err != nil {
			return "", "", true, err
		}
		need, err := requireChatEventToken(fields, "need", 256)
		if err != nil {
			return "", "", true, err
		}
		facts, err := requireChatEventInteger(fields, "facts", true)
		if err != nil {
			return "", "", true, err
		}
		if _, err := requireChatEventText(fields, "stop", 512); err != nil {
			return "", "", true, err
		}
		return chatProgressRetrieval, fmt.Sprintf("Resolved %s with %d repository facts", need, facts), true, nil
	case "coding_target_tree_validation_failed":
		fields, err := exactChatEventFields(event.Message, "diagnostic")
		diagnostic, fieldErr := requireChatEventText(fields, "diagnostic", maxChatProgressRawBytes)
		return chatProgressDiagnostic, "Target tree validation failed: " + boundedChatProgressText(diagnostic), true,
			firstChatProgressError(err, fieldErr)
	case "coding_compiler_repair_applied":
		fields, err := exactChatEventFields(event.Message, "block", "mechanism")
		if err != nil {
			return "", "", true, err
		}
		block, err := requireChatEventToken(fields, "block", 256)
		if err != nil || fields["mechanism"] != "deterministic_primitive_nullish_narrowing" {
			return "", "", true, firstChatProgressError(err, fmt.Errorf("compiler repair mechanism is not registered"))
		}
		return chatProgressDiagnostic, "Applied deterministic compiler repair to " + block, true, nil
	default:
		return "", "", false, nil
	}
}
