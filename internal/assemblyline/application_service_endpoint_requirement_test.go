package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationServiceEndpointRequirementIsOneTaskLocalBlindEnum(t *testing.T) {
	t.Parallel()
	specification, frozen := applicationTaskAuthorityProjectionFixture(t)
	task := frozen.Tasks[0]
	authority, err := ProjectApplicationTaskRuntimeAuthority(frozen, task.ID)
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
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	envelope := prompt
	for _, required := range []string{
		specification.ProductQuote, task.RequirementQuote,
		"PRODUCT CONTEXT:", "EXACT ENDPOINT REQUIREMENT:",
		string(ApplicationServiceEndpointRequired), string(ApplicationServiceSupportOnly),
	} {
		if !strings.Contains(envelope, required) {
			t.Fatalf("endpoint requirement envelope omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		task.ID, task.RequirementID, `"task_id"`, `"requirement_id"`,
		`"objective"`, `"required_behaviors"`, `"acceptance_criteria"`,
		`"path"`, `"file"`, `"command"`, `"tool"`, `"workflow"`,
		`"route_template"`, `"method"`, `"handler"`, `"source"`,
		"accepted local", "this task", "TASK", "AUTHORITY_JSON",
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
		got, err := DecodeApplicationServiceEndpointRequirementResult(input, string(requirement))
		if err != nil {
			t.Fatalf("decode %q: %v", requirement, err)
		}
		if got != want {
			t.Fatalf("decoded result=%+v want=%+v", got, want)
		}
	}
}

func TestApplicationServiceEndpointRequirementRejectsInvalidAuthorityAndWire(t *testing.T) {
	t.Parallel()
	validInput := ApplicationServiceEndpointRequirementInput{
		ProductContext:   "inventory service",
		RequirementQuote: "Records are normalized before storage.",
	}
	for name, mutate := range map[string]func(*ApplicationServiceEndpointRequirementInput){
		"blank product": func(input *ApplicationServiceEndpointRequirementInput) {
			input.ProductContext = ""
		},
		"path-bearing requirement": func(input *ApplicationServiceEndpointRequirementInput) {
			input.RequirementQuote = "Normalize records in src/records.go."
		},
		"missing requirement": func(input *ApplicationServiceEndpointRequirementInput) {
			input.RequirementQuote = ""
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
		"unregistered enum": "endpoint_optional",
		"JSON wrapper":      `{"endpoint_requirement":"support_only"}`,
		"quoted":            `"endpoint_required"`,
		"label":             "endpoint_requirement: support_only",
		"trailing value":    "support_only endpoint_required",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeApplicationServiceEndpointRequirementResult(validInput, raw); err == nil {
				t.Fatal("invalid endpoint requirement result was accepted")
			}
		})
	}
}
