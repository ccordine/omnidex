package objectiveworkload

type Requirement struct {
	ID          RequirementID
	SourceQuote string
	Start       int
	End         int
	SHA256      string
}

type ObjectiveKind string

const (
	ObjectiveRoot        ObjectiveKind = "root"
	ObjectiveRequirement ObjectiveKind = "requirement"
	ObjectiveMaterialize ObjectiveKind = "materialize"
	ObjectiveVerify      ObjectiveKind = "verify"
)

type ObjectiveStatus string

const (
	ObjectivePending  ObjectiveStatus = "pending"
	ObjectiveComplete ObjectiveStatus = "complete"
)

type AcceptancePredicate string

const (
	AcceptanceRequirementsComplete AcceptancePredicate = "requirements_complete"
	AcceptanceRequirementVerified  AcceptancePredicate = "requirement_verified"
	AcceptanceArtifactProduced     AcceptancePredicate = "artifact_produced"
	AcceptanceArtifactVerified     AcceptancePredicate = "artifact_verified"
)

type Objective struct {
	ID            ObjectiveID
	Kind          ObjectiveKind
	Parent        ObjectiveID
	DependsOn     []ObjectiveID
	RequirementID RequirementID
	Acceptance    []AcceptancePredicate
	Status        ObjectiveStatus
}

type Workload struct {
	ID              WorkloadID
	Authority       Authority
	RootObjectiveID ObjectiveID
	Requirements    []Requirement
	Objectives      []Objective
}

func cloneRequirement(value Requirement) Requirement { return value }

func cloneObjective(value Objective) Objective {
	value.DependsOn = append([]ObjectiveID{}, value.DependsOn...)
	value.Acceptance = append([]AcceptancePredicate{}, value.Acceptance...)
	return value
}

func cloneWorkload(value Workload) Workload {
	value.Requirements = append([]Requirement{}, value.Requirements...)
	originalObjectives := value.Objectives
	value.Objectives = make([]Objective, len(originalObjectives))
	for index, objective := range originalObjectives {
		value.Objectives[index] = cloneObjective(objective)
	}
	return value
}
