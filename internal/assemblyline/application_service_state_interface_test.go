package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationServiceStateInterfaceUsesOneRawLeafPerCall(t *testing.T) {
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
	leafInput := ApplicationStateFieldLeafInput{
		Authority: input, AcceptedFields: []ApplicationServiceStateField{},
	}
	constructors := []func() (PortableJob, error){
		func() (PortableJob, error) { return NewApplicationStateFieldCoverageJob(leafInput) },
		func() (PortableJob, error) { return NewApplicationStateFieldNameJob(leafInput) },
		func() (PortableJob, error) {
			return NewApplicationStateFieldKindJob(ApplicationStateFieldKindInput{
				Authority: input, AcceptedFields: []ApplicationServiceStateField{},
				FocusedName: "measurements",
			})
		},
	}
	prompts := ""
	for _, construct := range constructors {
		job, err := construct()
		if err != nil {
			t.Fatal(err)
		}
		prompt, err := RenderPortableJob(job)
		if err != nil {
			t.Fatal(err)
		}
		prompts += prompt
	}
	for _, required := range []string{
		input.ProductContext, input.Needs[0].RequirementQuote,
		string(ApplicationServiceStateRecordList),
	} {
		if !strings.Contains(prompts, required) {
			t.Fatalf("service state interface envelope omitted %q", required)
		}
	}
	for _, forbidden := range []string{
		"task_id", "workspace", "filename", "filesystem", " path", "tool", "command",
		"route", "orchestrat", "workflow", "completion", `"action"`, `"decision"`,
		`"status"`, `"accept"`, `"reject"`, `"apply"`, `"execute"`,
		strings.ToLower(frozen.Tasks[0].AcceptanceCriteria[0]), `"acceptance_criteria"`,
	} {
		if strings.Contains(strings.ToLower(prompts), forbidden) {
			t.Fatalf("service state interface envelope exposed %q: %s", forbidden, prompts)
		}
	}
}

func TestApplicationServiceStateInterfaceCodeStrictlyBoundsAssembledShape(t *testing.T) {
	t.Parallel()
	input := serviceStateInterfaceFixture()
	result := ApplicationServiceStateInterfaceResult{
		Schema: ApplicationServiceStateInterfaceSchemaV1,
		Fields: []ApplicationServiceStateField{{
			Name: "entries", Kind: ApplicationServiceStateRecordList,
			RecordFields: []ApplicationServiceStateRecordField{
				{Name: "label", Kind: ApplicationServiceStateString},
				{Name: "rank", Kind: ApplicationServiceStateInteger},
			},
		}},
	}
	if err := result.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	if len(result.Fields) != 1 || len(result.Fields[0].RecordFields) != 2 {
		t.Fatalf("assembled interface=%+v", result)
	}
	for name, candidate := range map[string]ApplicationServiceStateInterfaceResult{
		"unknown root kind": {
			Schema: ApplicationServiceStateInterfaceSchemaV1,
			Fields: []ApplicationServiceStateField{{Name: "entries", Kind: "object"}},
		},
		"nested record list": {
			Schema: ApplicationServiceStateInterfaceSchemaV1,
			Fields: []ApplicationServiceStateField{{
				Name: "entries", Kind: ApplicationServiceStateRecordList,
				RecordFields: []ApplicationServiceStateRecordField{{
					Name: "children", Kind: ApplicationServiceStateRecordList,
				}},
			}},
		},
		"record metadata on scalar": {
			Schema: ApplicationServiceStateInterfaceSchemaV1,
			Fields: []ApplicationServiceStateField{{
				Name: "count", Kind: ApplicationServiceStateInteger,
				RecordFields: []ApplicationServiceStateRecordField{{
					Name: "value", Kind: ApplicationServiceStateInteger,
				}},
			}},
		},
		"duplicate root": {
			Schema: ApplicationServiceStateInterfaceSchemaV1,
			Fields: []ApplicationServiceStateField{
				{Name: "count", Kind: ApplicationServiceStateInteger},
				{Name: "count", Kind: ApplicationServiceStateInteger},
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := candidate.ValidateFor(input); err == nil {
				t.Fatal("accepted invalid state interface")
			}
		})
	}
}

func TestApplicationServiceStateLeafDecodersRejectStructuredOutput(t *testing.T) {
	t.Parallel()
	input := ApplicationStateFieldLeafInput{
		Authority:      serviceStateInterfaceFixture(),
		AcceptedFields: []ApplicationServiceStateField{},
	}
	for _, candidate := range []string{
		`{"name":"entries"}`,
		`"entries"`,
		`["entries"]`,
	} {
		if _, err := DecodeApplicationStateFieldNameLeaf(input, candidate); err == nil {
			t.Fatalf("structured field name candidate %q was accepted", candidate)
		}
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
