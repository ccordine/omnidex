package worker

import (
	"fmt"
	"strings"
)

func renderDirectCodingWorkerEvent(event typedWorkerEvent) string {
	parts := []string{
		"kind=" + safeLine(string(event.Kind), "unknown"),
		"subject=" + safeEventToken(event.Subject, "unknown"),
		"model=" + safeEventToken(event.Model, "unknown"),
	}
	if event.MaxAttempts > 0 {
		parts = append(parts, fmt.Sprintf("attempt=%d/%d", event.Attempt, event.MaxAttempts))
	} else if event.Attempt > 0 {
		parts = append(parts, fmt.Sprintf("attempt=%d", event.Attempt))
	}
	if event.PromptBytes > 0 {
		parts = append(parts, fmt.Sprintf(
			"context=prompt:%dB,capabilities:%dB,current:%dB,correction:%dB",
			event.PromptBytes, event.CapabilityBytes, event.CurrentBytes, event.CorrectionBytes,
		))
	}
	if detail := strings.TrimSpace(event.Detail); detail != "" {
		parts = append(parts, "error="+safeLine(detail, "unknown"))
	}
	if warning := strings.TrimSpace(event.Warning); warning != "" {
		parts = append(parts, "warning="+safeLine(warning, "unknown"))
	}
	return strings.Join(parts, " ")
}

func safeEventToken(value, fallback string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	return strings.NewReplacer(" ", "_", "\t", "_", "\n", "_").Replace(value)
}
