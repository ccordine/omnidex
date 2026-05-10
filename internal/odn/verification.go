package odn

import "fmt"

type ToolOutcome struct {
	ToolID string
	Status string
}

func ExecuteVerificationTool(outcomes []ToolOutcome, nextEventID func() string) (string, []Event, bool) {
	failed := 0
	blocked := 0
	succeeded := 0

	for _, outcome := range outcomes {
		switch outcome.Status {
		case "success":
			succeeded++
		case "blocked":
			blocked++
		default:
			failed++
		}
	}

	passed := failed == 0
	summary := fmt.Sprintf("verification: success=%d blocked=%d failed=%d", succeeded, blocked, failed)
	events := []Event{{
		ID:      nextEventID(),
		Type:    "verification_completed",
		Summary: "verification_gate completed",
		Details: map[string]string{
			"success_count": fmt.Sprintf("%d", succeeded),
			"blocked_count": fmt.Sprintf("%d", blocked),
			"failed_count":  fmt.Sprintf("%d", failed),
			"passed":        fmt.Sprintf("%t", passed),
		},
		CreatedAt: nowUTC(),
	}}

	if !passed {
		events = append(events, Event{
			ID:        nextEventID(),
			Type:      "verification_failed",
			Summary:   "verification_gate found failed tool outcomes",
			CreatedAt: nowUTC(),
		})
	}

	return summary, events, passed
}
