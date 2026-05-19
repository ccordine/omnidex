package odn

import "strings"

func ClassifyIntent(message string) IntentResult {
	if strings.TrimSpace(message) == "" {
		return IntentResult{
			Classification: IntentAmbiguous,
			Confidence:     0,
			ReasonCodes:    []string{"empty_message"},
		}
	}
	return IntentResult{
		Classification: IntentExecution,
		Confidence:     1.0,
		ReasonCodes:    []string{"default_execution_mode"},
	}
}
