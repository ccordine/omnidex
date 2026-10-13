package main

import "github.com/gryph/omnidex/internal/client"

func (renderer *chatRenderer) eventActivity(event client.RealtimeEvent) (string, bool) {
	if event.PlanningActivity != nil {
		activity := *event.PlanningActivity
		if activity.Validate() != nil {
			return "", false
		}
		detail, err := planningActivityNarration(activity)
		if err != nil {
			return "", false
		}
		if activity.Kind == "semantic_started" || activity.Kind == "semantic_completed" {
			detail = activity.Question
			if activity.Subject != "" {
				detail = activity.Subject + " · " + detail
			}
		}
		if renderer.verbosity != chatVerbose && renderer.verbosity != chatDebug {
			detail = shortAgentExcerpt(detail)
		}
		return detail, true
	}
	if renderer.planningWorkerObserved(event) {
		return "", false
	}
	return chatEventActivity(event)
}

// Subjects are exact code-owned worker metadata, not user-language routing.
func chatEventActivity(event client.RealtimeEvent) (string, bool) {
	activity, known := workerActivityForEvent(event)
	if !known || activity.status != "started" {
		return "", false
	}
	return activity.action, true
}
