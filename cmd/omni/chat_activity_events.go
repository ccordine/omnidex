package main

import "github.com/gryph/omnidex/internal/client"

// Subjects are exact code-owned worker metadata, not user-language routing.
func chatEventActivity(event client.RealtimeEvent) (string, bool) {
	activity, known := workerActivityForEvent(event)
	if !known || activity.status != "started" {
		return "", false
	}
	return activity.action, true
}
