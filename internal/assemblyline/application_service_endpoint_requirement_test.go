package assemblyline

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestApplicationServiceEndpointRequirementIsOneTaskLocalBlindEnum(t *testing.T) {
	t.Parallel()
	workloadInput, frozen := applicationTaskAuthorityProjectionFixture(t)
	task := frozen.Tasks[0]
	authority, err := ProjectApplicationTaskRuntimeAuthority(
		workloadInput, frozen, task.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	input, err := ProjectApplicationServiceEndpointRequirementInput(authority)
	if err != nil {
		t.Fatal(err)
	}
	job, err := NewApplicationServiceEndpointRequirementJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkApplicationServiceEndpointRequirement {
		t.Fatalf("work kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		t.Fatal(err)
	}
	envelope := prompt + string(schemaJSON)
	for _, required := range []string{
		workloadInput.ProductQuote, task.RequirementQuote, task.Objective,
		string(ApplicationServiceEndpointRequired), string(ApplicationServiceSupportOnly),
	} {
		if !strings.Contains(envelope, required) {
			t.Fatalf("endpoint requirement envelope omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		task.ID, task.RequirementID, `"task_id"`, `"requirement_id"`,
		task.AcceptanceCriteria[0], `"acceptance_criteria"`,
		`"path"`, `"file"`, `"command"`, `"tool"`, `"workflow"`,
		`"route_template"`, `"method"`, `"handler"`, `"source"`,
	} {
		if strings.Contains(envelope, forbidden) {
			t.Fatalf("endpoint requirement envelope exposed forbidden authority %q", forbidden)
		}
	}

	for _, requirement := range []ApplicationServiceEndpointRequirement{
		ApplicationServiceEndpointRequired, ApplicationServiceSupportOnly,
	} {
		want := ApplicationServiceEndpointRequirementResult{
			Schema:              ApplicationServiceEndpointRequirementSchemaV1,
			EndpointRequirement: requirement,
		}
		raw, err := json.Marshal(want)
		if err != nil {
			t.Fatal(err)
		}
		got, err := DecodeApplicationServiceEndpointRequirementResult(input, string(raw))
		if err != nil {
			t.Fatalf("decode %q: %v", requirement, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("decoded result=%+v want=%+v", got, want)
		}
	}
}

func TestApplicationServiceEndpointRequirementRejectsInvalidAuthorityAndWire(t *testing.T) {
	t.Parallel()
	validInput := ApplicationServiceEndpointRequirementInput{
		ProductContext:    "inventory service",
		RequirementQuote:  "Records are normalized before storage.",
		Objective:         "Normalize accepted record values.",
		RequiredBehaviors: []string{"Normalize each accepted record value."},
	}
	for name, mutate := range map[string]func(*ApplicationServiceEndpointRequirementInput){
		"blank product": func(input *ApplicationServiceEndpointRequirementInput) {
			input.ProductContext = ""
		},
		"multiline objective": func(input *ApplicationServiceEndpointRequirementInput) {
			input.Objective = "Normalize\nrecords."
		},
		"missing behavior": func(input *ApplicationServiceEndpointRequirementInput) {
			input.RequiredBehaviors = nil
		},
	} {
		t.Run(name, func(t *testing.T) {
			input := validInput
			mutate(&input)
			if _, err := NewApplicationServiceEndpointRequirementJob(input); err == nil {
				t.Fatal("invalid endpoint requirement authority was accepted")
			}
		})
	}

	for name, raw := range map[string]string{
		"bad schema":        `{"schema":"v2","endpoint_requirement":"support_only"}`,
		"unregistered enum": `{"schema":"omnidex.application-service-endpoint-requirement.v1","endpoint_requirement":"endpoint_optional"}`,
		"control field":     `{"schema":"omnidex.application-service-endpoint-requirement.v1","endpoint_requirement":"support_only","execute":true}`,
		"missing enum":      `{"schema":"omnidex.application-service-endpoint-requirement.v1"}`,
		"trailing object":   `{"schema":"omnidex.application-service-endpoint-requirement.v1","endpoint_requirement":"support_only"} {}`,
		"non-object":        `"endpoint_required"`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeApplicationServiceEndpointRequirementResult(validInput, raw); err == nil {
				t.Fatal("invalid endpoint requirement result was accepted")
			}
		})
	}
}
