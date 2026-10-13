package worker

import "github.com/gryph/omnidex/internal/model"

func observeDirectCodingPlanning(runtime typedWorkerRuntime, activity model.PlanningActivity) {
	if runtime.ObservePlanning != nil {
		runtime.ObservePlanning(activity)
	}
}

// The plan's disposition switch validates the resolution and consumes its state
// before this display projection is used. It creates no semantic authority.
func observeDirectCodingPlanningResolution(
	runtime typedWorkerRuntime,
	resolution directCodingApplicationRequirementCandidateResolution,
	count int,
) {
	outcomes := map[directCodingApplicationRequirementDisposition]string{
		directCodingApplicationRequirementRetained:    "retained",
		directCodingApplicationRequirementExcluded:    "excluded",
		directCodingApplicationRequirementUnrequested: "unrequested",
		directCodingApplicationRequirementDuplicate:   "duplicate",
		directCodingApplicationRequirementPartitioned: "partitioned",
		directCodingApplicationRequirementUnresolved:  "unresolved",
	}
	observeDirectCodingPlanning(runtime, model.PlanningActivity{
		Schema: 1, Kind: "candidate_resolved", Subject: resolution.Candidate,
		Outcome: outcomes[resolution.Disposition], Count: count,
	})
}
