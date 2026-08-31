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
	case "coding_compiler_repair_applied":
		fields, err := exactChatEventFields(event.Message, "block", "mechanism")
		if err != nil {
			return "", "", true, err
		}
		block, err := requireChatEventToken(fields, "block", 256)
		if err != nil || !registeredChatCompilerRepairMechanism(fields["mechanism"]) {
			return "", "", true, firstChatProgressError(err, fmt.Errorf("compiler repair mechanism is not registered"))
		}
		return chatProgressDiagnostic, "Applied deterministic compiler repair to " + block, true, nil
	default:
		return "", "", false, nil
	}
}

func registeredChatCompilerRepairMechanism(mechanism string) bool {
	switch mechanism {
	case "deterministic_primitive_nullish_narrowing",
		"deterministic_primitive_reference_narrowing":
		return true
	default:
		return false
	}
}
