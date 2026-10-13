package main

import (
	"strings"

	"github.com/gryph/omnidex/internal/client"
)

// Subjects are exact code-owned worker metadata, not user-language routing.
func chatEventActivity(event client.RealtimeEvent) (string, bool) {
	if event.EventName != client.RealtimeJobRuntimeEvent {
		return "", false
	}
	if event.RuntimeEvent != "coding_worker_started" && event.RuntimeEvent != "objective_worker_started" {
		return "", false
	}
	for _, field := range strings.Fields(event.Detail) {
		subject, ok := strings.CutPrefix(field, "subject=")
		if !ok {
			continue
		}
		switch subject {
		case "conversation_objective_kind":
			return "Reading your request", true
		case "application_requirement_inventory":
			return "Collecting requirements", true
		case "application_requirement_candidate_authorization",
			"application_requirement_candidate_runtime_content_presence",
			"application_requirement_candidate_non_runtime_content_presence",
			"application_requirement_candidate_partition",
			"application_requirement_candidate_cardinality",
			"application_requirement_candidate_derived_value_presence",
			"application_requirement_candidate_outcome_relation":
			return "Checking requirements", true
		}
	}
	return "", true
}
