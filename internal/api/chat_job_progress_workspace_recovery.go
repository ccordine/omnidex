package api

import "fmt"

func summarizeChatWorkspaceRecovery(event parsedChatStepEvent) (chatProgressKind, string, error) {
	switch event.Type {
	case "workspace_mutation_recovery_started":
		fields, err := exactChatEventFields(event.Message, "stage", "source")
		if err != nil {
			return "", "", err
		}
		if _, err := requireChatEventText(fields, "stage", 256); err != nil {
			return "", "", err
		}
		if _, err := requireChatEventText(fields, "source", 256); err != nil {
			return "", "", err
		}
		return chatProgressReview, "Reconciling a durable workspace mutation", nil
	case "workspace_mutation_recovered":
		fields, err := exactChatEventFields(event.Message, "operation", "expected")
		if err != nil {
			return "", "", err
		}
		if _, err := requireChatEventText(fields, "operation", 256); err != nil {
			return "", "", err
		}
		if _, err := requireChatEventText(fields, "expected", 256); err != nil {
			return "", "", err
		}
		return chatProgressFile, "Recovered and reconciled the durable workspace mutation", nil
	default:
		return "", "", fmt.Errorf("event type %q is not a workspace mutation recovery event", event.Type)
	}
}
