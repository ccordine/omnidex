package assemblyline

import "fmt"

const (
	ApplicationWorkloadDraftSchemaV1  = "omnidex.application-workload-draft.v1"
	ApplicationWorkloadFrozenSchemaV1 = "omnidex.application-workload.v1"
	maxApplicationRequiredBehaviors   = 4
	maxApplicationAcceptanceCriteria  = 4
	maxApplicationObjectiveRunes      = 512
	maxApplicationBehaviorRunes       = 512
	maxApplicationCriterionRunes      = 512
	maxApplicationDependencyIDBytes   = 64
)

type ApplicationJobSpecificationInput struct {
	Surface              ApplicationSurface `json:"surface"`
	ProductQuote         string             `json:"product_quote"`
	AcceptedRequirements []Requirement      `json:"accepted_requirements"`
	FocusedRequirement   Requirement        `json:"focused_requirement"`
}

type ApplicationJobSpecification struct {
	Objective          string   `json:"objective"`
	RequiredBehaviors  []string `json:"required_behaviors"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

type ApplicationJobSpecificationField string

const (
	ApplicationJobSpecificationObjectiveField          ApplicationJobSpecificationField = "objective"
	ApplicationJobSpecificationRequiredBehaviorsField  ApplicationJobSpecificationField = "required_behaviors"
	ApplicationJobSpecificationAcceptanceCriteriaField ApplicationJobSpecificationField = "acceptance_criteria"
)

type ApplicationJobSpecificationDefect struct {
	Field  ApplicationJobSpecificationField `json:"field"`
	Detail string                           `json:"detail"`
}

func (defect *ApplicationJobSpecificationDefect) Error() string {
	if defect == nil {
		return ""
	}
	return fmt.Sprintf("%s: %s", defect.Field, defect.Detail)
}

type ApplicationWorkloadDraftInput struct {
	Surface      ApplicationSurface `json:"surface"`
	ProductQuote string             `json:"product_quote"`
	Requirements []Requirement      `json:"requirements"`
}

type ApplicationWorkloadTaskDraft struct {
	RequirementID      string   `json:"requirement_id"`
	Objective          string   `json:"objective"`
	RequiredBehaviors  []string `json:"required_behaviors"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	DependsOn          []string `json:"depends_on"`
}

type ApplicationWorkloadDraft struct {
	Schema string                         `json:"schema"`
	Tasks  []ApplicationWorkloadTaskDraft `json:"tasks"`
}

type ApplicationWorkloadTaskField string

const (
	ApplicationWorkloadObjectiveField          ApplicationWorkloadTaskField = "objective"
	ApplicationWorkloadRequiredBehaviorsField  ApplicationWorkloadTaskField = "required_behaviors"
	ApplicationWorkloadAcceptanceCriteriaField ApplicationWorkloadTaskField = "acceptance_criteria"
	ApplicationWorkloadDependsOnField          ApplicationWorkloadTaskField = "depends_on"
)

type ApplicationWorkloadDefect struct {
	TaskID string                       `json:"task_id,omitempty"`
	Field  ApplicationWorkloadTaskField `json:"field,omitempty"`
	Detail string                       `json:"detail"`
}

func (defect *ApplicationWorkloadDefect) Error() string {
	if defect == nil {
		return ""
	}
	if defect.TaskID == "" {
		return defect.Detail
	}
	if defect.Field == "" {
		return fmt.Sprintf("%s: %s", defect.TaskID, defect.Detail)
	}
	return fmt.Sprintf("%s %s: %s", defect.TaskID, defect.Field, defect.Detail)
}

func (defect *ApplicationWorkloadDefect) Repairable() bool {
	return false
}

type FrozenApplicationTask struct {
	ID                 string   `json:"task_id"`
	RequirementID      string   `json:"requirement_id"`
	RequirementQuote   string   `json:"requirement_quote"`
	Objective          string   `json:"objective"`
	RequiredBehaviors  []string `json:"required_behaviors"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
	DependsOn          []string `json:"depends_on"`
}

type FrozenApplicationWorkload struct {
	Schema       string                  `json:"schema"`
	SHA256       string                  `json:"sha256"`
	Surface      ApplicationSurface      `json:"surface"`
	ProductQuote string                  `json:"product_quote"`
	Tasks        []FrozenApplicationTask `json:"tasks"`
}

type ApplicationTaskContext struct {
	WorkloadSHA256 string                             `json:"workload_sha256"`
	Surface        ApplicationSurface                 `json:"surface"`
	ProductQuote   string                             `json:"product_quote"`
	Task           ApplicationTaskContextTask         `json:"task"`
	Dependencies   []ApplicationTaskDependencyContext `json:"dependencies"`
}

type ApplicationTaskContextTask struct {
	TaskID             string   `json:"task_id"`
	RequirementID      string   `json:"requirement_id"`
	RequirementQuote   string   `json:"requirement_quote"`
	Objective          string   `json:"objective"`
	RequiredBehaviors  []string `json:"required_behaviors"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

type ApplicationTaskDependencyContext struct {
	TaskID           string `json:"task_id"`
	RequirementID    string `json:"requirement_id"`
	RequirementQuote string `json:"requirement_quote"`
}
