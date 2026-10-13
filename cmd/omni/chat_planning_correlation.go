package main

import "github.com/gryph/omnidex/internal/client"

const maxPlanningRenderCalls = 32

type planningCallKey struct {
	job, step, attempt int64
	station, model     string
}

// This bounded display state suppresses duplicate headers only after the same
// observed call boundary has already been presented with its exact facts.
type chatPlanningCalls struct {
	phases map[planningCallKey]string
	order  []planningCallKey
}

func (renderer *chatRenderer) rememberPlanningCall(event client.RealtimeEvent) {
	activity := event.PlanningActivity
	if activity.Kind != "semantic_started" && activity.Kind != "semantic_completed" {
		return
	}
	if renderer.planningCalls == nil {
		renderer.planningCalls = &chatPlanningCalls{phases: make(map[planningCallKey]string)}
	}
	calls := renderer.planningCalls
	key := planningCallKey{event.JobID, event.StepID, event.Attempt, activity.Station, activity.Model}
	if _, present := calls.phases[key]; !present {
		if len(calls.order) == maxPlanningRenderCalls {
			calls.forget(calls.order[0])
		}
		calls.order = append(calls.order, key)
	}
	calls.phases[key] = activity.Kind
}

func (renderer chatRenderer) planningWorkerObserved(event client.RealtimeEvent) bool {
	activity, known := workerActivityForEvent(event)
	if !known || renderer.planningCalls == nil {
		return false
	}
	phase := renderer.planningCalls.phases[workerPlanningCallKey(event, activity)]
	return activity.status == "started" && phase == "semantic_started" ||
		activity.status == "completed" && phase == "semantic_completed"
}

func (renderer chatRenderer) retirePlanningWorker(event client.RealtimeEvent) {
	activity, known := workerActivityForEvent(event)
	if !known || renderer.planningCalls == nil || activity.status == "started" {
		return
	}
	renderer.planningCalls.forget(workerPlanningCallKey(event, activity))
}

func (renderer chatRenderer) retainPlanningScope(activity chatActivity) {
	if renderer.planningCalls == nil {
		return
	}
	for key := range renderer.planningCalls.phases {
		if !activity.Working || key.job != activity.JobID || key.step != activity.StepID {
			renderer.planningCalls.forget(key)
		}
	}
}

func workerPlanningCallKey(event client.RealtimeEvent, activity chatWorkerActivity) planningCallKey {
	return planningCallKey{event.JobID, event.StepID, event.Attempt, activity.subject, activity.model}
}

func (calls *chatPlanningCalls) forget(key planningCallKey) {
	delete(calls.phases, key)
	for index, recorded := range calls.order {
		if recorded == key {
			calls.order = append(calls.order[:index], calls.order[index+1:]...)
			return
		}
	}
}
