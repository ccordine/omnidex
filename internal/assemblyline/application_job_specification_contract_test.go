package assemblyline

import (
	"reflect"
	"strings"
	"testing"
)

func TestApplicationJobSpecificationPromptContainsOnlyFocusedAuthoritativeIntent(t *testing.T) {
	t.Parallel()
	input := applicationJobSpecificationTestInput(1)
	prompt, err := BuildApplicationJobSpecificationPrompt(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		string(input.Surface), input.ProductQuote, input.FocusedRequirement.SourceQuote,
		"required_behaviors", "acceptance_criteria",
		"concrete action and result", "cover every required behavior", `"user_authority"`,
		"minimum sufficient derived build decisions", "Observable does not require invented numeric precision",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("job specification prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{
		"workspace_snapshot", "file_path", "tool_catalog", "depends_on", "execution_order", "completion",
		input.AcceptedRequirements[0].SourceQuote, input.AcceptedRequirements[2].SourceQuote,
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("job specification prompt exposes forbidden authority %q", forbidden)
		}
	}
}

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
			if _, err := NewApplicationJobSpecificationJob(candidate); err == nil {
				t.Fatalf("accepted incomplete or drifted authority: %+v", candidate)
			}
		})
	}
}

func TestApplicationJobSpecificationResponseIsConcreteAndBounded(t *testing.T) {
	t.Parallel()
	input := applicationJobSpecificationTestInput(1)
	schema, err := ApplicationJobSpecificationResponseSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	properties := schema["properties"].(map[string]any)
	wantFields := []string{"acceptance_criteria", "objective", "required_behaviors"}
	if !reflect.DeepEqual(sortedJobSpecificationKeys(properties), wantFields) {
		t.Fatalf("job specification properties=%v want %v", sortedJobSpecificationKeys(properties), wantFields)
	}
	for _, field := range []string{"required_behaviors", "acceptance_criteria"} {
		definition := properties[field].(map[string]any)
		if definition["type"] != "array" || definition["minItems"] != 1 || definition["maxItems"] != 4 {
			t.Fatalf("%s is not a bounded 1..4 array: %#v", field, definition)
		}
	}

	raw := `{"objective":"Implement interactive mixer channels for the browser music studio.","required_behaviors":["Users can add and remove mixer channels.","Each channel exposes controls that change its audio state."],"acceptance_criteria":["Adding a channel displays another independently controllable mixer channel.","Changing one channel control updates that channel without changing another channel."]}`
	specification, err := DecodeApplicationJobSpecification(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(specification.RequiredBehaviors) != 2 || len(specification.AcceptanceCriteria) != 2 {
		t.Fatalf("decoded impoverished job specification: %+v", specification)
	}
	for _, injected := range []string{
		`"path":"src/app.tsx",`, `"tools":["shell"],`, `"depends_on":[],`,
		`"execution_order":1,`, `"complete":true,`, `"requirement_id":"requirement_002",`,
	} {
		candidate := strings.Replace(raw, `"objective":`, injected+`"objective":`, 1)
		if _, err := DecodeApplicationJobSpecification(input, candidate); err == nil {
			t.Fatalf("accepted forbidden model authority %s", injected)
		}
	}
}

func TestApplicationJobSpecificationValidationRetainsCandidateAndFailsLoudly(t *testing.T) {
	t.Parallel()
	input := applicationJobSpecificationTestInput(1)
	raw := `{"objective":"Implement filtering.","required_behaviors":["Apply the selected filter.","Apply the selected filter."],"acceptance_criteria":[" Visible records match the filter. "]}`
	specification, err := DecodeApplicationJobSpecification(input, raw)
	if err != nil {
		t.Fatalf("decode must retain a structurally complete candidate: %v", err)
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
