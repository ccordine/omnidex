package assemblyline

const ApplicationWorkloadFrozenSchemaV2 = "omnidex.application-workload.v2"

// FrozenApplicationTask is the code-owned one-to-one projection of an accepted
// requirement. It deliberately contains no model-authored objective, behavior,
// acceptance contract, dependency, or scheduling decision.
type FrozenApplicationTask struct {
	ID               string `json:"task_id"`
	RequirementID    string `json:"requirement_id"`
	RequirementQuote string `json:"requirement_quote"`
}

type FrozenApplicationWorkload struct {
	Schema       string                  `json:"schema"`
	Surface      ApplicationSurface      `json:"surface"`
	ProductQuote string                  `json:"product_quote"`
	Tasks        []FrozenApplicationTask `json:"tasks"`
}

type ApplicationTaskContext struct {
	Surface      ApplicationSurface         `json:"surface"`
	ProductQuote string                     `json:"product_quote"`
	Task         ApplicationTaskContextTask `json:"task"`
}

type ApplicationTaskContextTask struct {
	TaskID           string `json:"task_id"`
	RequirementID    string `json:"requirement_id"`
	RequirementQuote string `json:"requirement_quote"`
}
