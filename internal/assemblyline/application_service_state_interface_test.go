package assemblyline

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestApplicationServiceStateInterfaceIsOneMechanismBlindSemanticValue(t *testing.T) {
	t.Parallel()
	workloadInput, frozen := applicationTaskAuthorityProjectionFixture(t)
	authority, err := ProjectApplicationTaskRuntimeAuthority(workloadInput, frozen, "task_001")
	if err != nil {
		t.Fatal(err)
	}
	need, err := ProjectApplicationServiceStateInterfaceNeed(authority)
	if err != nil {
		t.Fatal(err)
	}
	input := ApplicationServiceStateInterfaceInput{
		ProductContext: authority.ProductQuote, Needs: []ApplicationServiceStateInterfaceNeed{need},
	}
	job, err := NewApplicationServiceStateInterfaceJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkApplicationServiceStateInterface {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, responseSchema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	encodedSchema, err := json.Marshal(responseSchema)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{
		input.ProductContext, input.Needs[0].RequirementQuote,
		ApplicationServiceStateInterfaceSchemaV1,
		string(ApplicationServiceStateRecordList),
	} {
		if !strings.Contains(prompt, required) && !strings.Contains(string(encodedSchema), required) {
			t.Fatalf("service state interface envelope omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"task_id", "workspace", "filename", "filesystem", " path", "tool", "command",
		"route", "orchestrat", "workflow", "completion", `"action"`, `"decision"`,
		`"status"`, `"accept"`, `"reject"`, `"apply"`, `"execute"`,
		strings.ToLower(frozen.Tasks[0].AcceptanceCriteria[0]), `"acceptance_criteria"`,
	} {
		if strings.Contains(strings.ToLower(prompt+string(encodedSchema)), forbidden) {
			t.Fatalf("service state interface envelope exposed %q: %s", forbidden, prompt)
		}
	}
}

func TestApplicationServiceStateInterfaceStrictlyBoundsOneSharedShape(t *testing.T) {
	t.Parallel()
	input := serviceStateInterfaceFixture()
	valid := `{"schema":"` + ApplicationServiceStateInterfaceSchemaV1 +
		`","fields":[{"name":"entries","kind":"record_list","record_fields":[` +
		`{"name":"label","kind":"string"},{"name":"rank","kind":"integer"}]}]}`
	result, err := DecodeApplicationServiceStateInterfaceResult(input, valid)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Fields) != 1 || len(result.Fields[0].RecordFields) != 2 {
		t.Fatalf("decoded interface=%+v", result)
	}
	for name, raw := range map[string]string{
		"unknown root kind": `{"schema":"` + ApplicationServiceStateInterfaceSchemaV1 +
			`","fields":[{"name":"entries","kind":"object","record_fields":[]}]}`,
		"nested record list": `{"schema":"` + ApplicationServiceStateInterfaceSchemaV1 +
			`","fields":[{"name":"entries","kind":"record_list","record_fields":[` +
			`{"name":"children","kind":"record_list"}]}]}`,
		"record metadata on scalar": `{"schema":"` + ApplicationServiceStateInterfaceSchemaV1 +
			`","fields":[{"name":"count","kind":"integer","record_fields":[` +
			`{"name":"value","kind":"integer"}]}]}`,
		"duplicate root": `{"schema":"` + ApplicationServiceStateInterfaceSchemaV1 +
			`","fields":[{"name":"count","kind":"integer","record_fields":[]},` +
			`{"name":"count","kind":"integer","record_fields":[]}]}`,
		"control field": `{"schema":"` + ApplicationServiceStateInterfaceSchemaV1 +
			`","fields":[{"name":"count","kind":"integer","record_fields":[],"action":"apply"}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeApplicationServiceStateInterfaceResult(input, raw); err == nil {
				t.Fatal("accepted invalid state interface")
			}
		})
	}
}

func serviceStateInterfaceFixture() ApplicationServiceStateInterfaceInput {
	return ApplicationServiceStateInterfaceInput{
		ProductContext: "shipment registry",
		Needs: []ApplicationServiceStateInterfaceNeed{
			{
				RequirementQuote: "Store a shipment measurement for later retrieval.",
				Objective:        "Preserve shipment measurements between requests.",
				RequiredBehaviors: []string{
					"Retain each measurement with its stable identifier.",
				},
			},
		},
	}
}
