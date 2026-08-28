package assemblyline

import (
	"reflect"
	"testing"
)

func TestApplicationJobSpecificationInputRejectsIncompleteOrDriftedAuthority(t *testing.T) {
	t.Parallel()
	valid := applicationJobSpecificationTestInput(1)
	tests := map[string]func(*ApplicationJobSpecificationInput){
		"unsupported surface": func(value *ApplicationJobSpecificationInput) {
			value.Surface = ApplicationSurfaceUnsupported
		},
		"missing accepted requirements": func(value *ApplicationJobSpecificationInput) {
			value.AcceptedRequirements = nil
		},
		"focused requirement absent": func(value *ApplicationJobSpecificationInput) {
			value.FocusedRequirement = Requirement{ID: "requirement_999", SourceQuote: "invented feature"}
		},
		"focused quote drift": func(value *ApplicationJobSpecificationInput) {
			value.FocusedRequirement.SourceQuote = "changed quote"
		},
		"accepted identity drift": func(value *ApplicationJobSpecificationInput) {
			value.AcceptedRequirements[1].ID = "model_owned"
		},
	}
	for name, mutate := range tests {
		name, mutate := name, mutate
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			candidate := cloneApplicationJobSpecificationInput(valid)
			mutate(&candidate)
			if _, err := NewApplicationJobObjectiveJob(candidate); err == nil {
				t.Fatalf("accepted incomplete or drifted authority: %+v", candidate)
			}
		})
	}
}

func TestApplicationJobSpecificationValidationRetainsCandidateAndFailsLoudly(t *testing.T) {
	t.Parallel()
	specification := ApplicationJobSpecification{
		Objective: "Implement filtering.",
		RequiredBehaviors: []string{
			"Apply the selected filter.", "Apply the selected filter.",
		},
		AcceptanceCriteria: []string{" Visible records match the filter. "},
	}
	defect := FirstApplicationJobSpecificationDefect(specification)
	if defect == nil || defect.Field != ApplicationJobSpecificationRequiredBehaviorsField {
		t.Fatalf("first code-owned defect=%+v", defect)
	}
	if err := ValidateApplicationJobSpecification(specification); err == nil {
		t.Fatal("accepted duplicate required behaviors")
	}

	valid := applicationJobSpecificationTestValue()
	invalid := []ApplicationJobSpecification{
		{Objective: valid.Objective, RequiredBehaviors: nil, AcceptanceCriteria: valid.AcceptanceCriteria},
		{Objective: valid.Objective, RequiredBehaviors: valid.RequiredBehaviors, AcceptanceCriteria: make([]string, 5)},
		{Objective: "Contains\nnewline.", RequiredBehaviors: valid.RequiredBehaviors, AcceptanceCriteria: valid.AcceptanceCriteria},
		{Objective: valid.Objective, RequiredBehaviors: []string{"Contains\ttab."}, AcceptanceCriteria: valid.AcceptanceCriteria},
	}
	for _, candidate := range invalid {
		if err := ValidateApplicationJobSpecification(candidate); err == nil {
			t.Fatalf("accepted invalid job specification %+v", candidate)
		}
	}
}

func TestMaterializeApplicationWorkloadDraftUsesExecutableSpecifications(t *testing.T) {
	t.Parallel()
	authority := applicationWorkloadTestInput()
	specifications := []ApplicationJobSpecification{
		{Objective: "Implement status grouping.", RequiredBehaviors: []string{"Place records in their selected group."}, AcceptanceCriteria: []string{"Changing status moves the record to its selected group."}},
		{Objective: "Implement record filtering.", RequiredBehaviors: []string{"Apply a selected filter to visible records."}, AcceptanceCriteria: []string{"Visible records match the selected filter."}},
		{Objective: "Implement printable export.", RequiredBehaviors: []string{"Create a printable summary from visible records."}, AcceptanceCriteria: []string{"A user can open a printable summary."}},
	}
	draft, err := MaterializeApplicationWorkloadDraft(authority, specifications)
	if err != nil {
		t.Fatal(err)
	}
	for index, task := range draft.Tasks {
		if task.RequirementID != authority.Requirements[index].ID || len(task.DependsOn) != 0 {
			t.Fatalf("code-owned materialization drifted at task %d: %+v", index, task)
		}
		if !reflect.DeepEqual(task.RequiredBehaviors, specifications[index].RequiredBehaviors) ||
			!reflect.DeepEqual(task.AcceptanceCriteria, specifications[index].AcceptanceCriteria) {
			t.Fatalf("task %d lost executable specification: %+v", index, task)
		}
	}
}

func applicationJobSpecificationTestInput(focused int) ApplicationJobSpecificationInput {
	input := ApplicationJobSpecificationInput{
		Surface: ApplicationSurfaceBrowser, ProductQuote: "browser music studio",
		AcceptedRequirements: []Requirement{
			{ID: "requirement_001", SourceQuote: "mixer channels"},
			{ID: "requirement_002", SourceQuote: "drum pads"},
			{ID: "requirement_003", SourceQuote: "keyboard"},
		},
	}
	input.FocusedRequirement = input.AcceptedRequirements[focused]
	return input
}

func cloneApplicationJobSpecificationInput(value ApplicationJobSpecificationInput) ApplicationJobSpecificationInput {
	copy := value
	copy.AcceptedRequirements = append([]Requirement(nil), value.AcceptedRequirements...)
	return copy
}

func applicationJobSpecificationTestValue() ApplicationJobSpecification {
	return ApplicationJobSpecification{
		Objective:          "Implement interactive mixer channels for the browser music studio.",
		RequiredBehaviors:  []string{"Users can add and remove mixer channels.", "Channel controls update channel audio state."},
		AcceptanceCriteria: []string{"Adding a channel displays an independently controllable mixer channel."},
	}
}
