package main

import (
	"strings"

	"github.com/gryph/omnidex/internal/client"
)

type chatWorkerActivity struct {
	action, subject, model, status string
}

func workerActivityForEvent(event client.RealtimeEvent) (chatWorkerActivity, bool) {
	if event.EventName != client.RealtimeJobRuntimeEvent {
		return chatWorkerActivity{}, false
	}
	var activity chatWorkerActivity
	switch event.RuntimeEvent {
	case "coding_worker_started", "objective_worker_started":
		activity.status = "started"
	case "coding_worker_completed", "objective_worker_completed":
		activity.status = "completed"
	case "coding_worker_rejected", "objective_worker_rejected":
		activity.status = "rejected"
	case "coding_worker_failed", "objective_worker_failed":
		activity.status = "failed"
	case "coding_worker_stopped", "objective_worker_stopped":
		activity.status = "stopped"
	default:
		return chatWorkerActivity{}, false
	}
	for _, field := range strings.Fields(event.Detail) {
		name, value, _ := strings.Cut(field, "=")
		switch name {
		case "subject":
			activity.subject = value
			activity.action = workerSubjectAction(value)
		case "model":
			activity.model = value
		}
	}
	return activity, true
}

// The subject is an exact code-owned work identity. Unknown subjects retain
// that identity with mechanical spacing, without guessing their meaning.
func workerSubjectAction(subject string) string {
	switch subject {
	case "conversation_objective_kind":
		return "Classify request"
	case "application_requirement_inventory":
		return "Collect requirements"
	case "application_requirement_candidate_authorization":
		return "Check requirement authorization"
	case "application_requirement_candidate_runtime_content_presence":
		return "Classify runtime content"
	case "application_requirement_candidate_non_runtime_content_presence":
		return "Classify non-runtime content"
	case "application_requirement_candidate_partition":
		return "Partition requirement outcomes"
	case "application_requirement_candidate_cardinality":
		return "Check outcome count"
	case "application_requirement_candidate_derived_value_presence":
		return "Check derived values"
	case "application_requirement_candidate_outcome_relation":
		return "Check outcome relations"
	default:
		return strings.ReplaceAll(subject, "_", " ")
	}
}

func (renderer chatRenderer) workerActivity(activity chatWorkerActivity) error {
	if renderer.verbosity == chatQuiet && activity.status != "failed" && activity.status != "rejected" {
		return nil
	}
	label := activity.action
	if label == "" {
		label = "Work"
	}
	label += " · " + activity.status
	if activity.model != "" {
		label += " · " + activity.model
	}
	return renderer.system("%s", label)
}
