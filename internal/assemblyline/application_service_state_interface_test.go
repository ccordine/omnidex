package assemblyline

import (
	"testing"
)

func TestApplicationServiceStateInterfaceAuthorityContainsSemanticNeedsOnly(t *testing.T) {
	t.Parallel()
	_, frozen := applicationTaskAuthorityProjectionFixture(t)
	authority, err := ProjectApplicationTaskRuntimeAuthority(frozen, "task_001")
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
	if err := input.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(input.Needs) != 1 || input.Needs[0].RequirementQuote != authority.RequirementQuote {
		t.Fatalf("state interface authority=%+v", input)
	}
}

func TestApplicationServiceStateInterfaceCodeStrictlyBoundsAssembledShape(t *testing.T) {
	t.Parallel()
	input := serviceStateInterfaceFixture()
	result := ApplicationServiceStateInterfaceResult{
		Schema: ApplicationServiceStateInterfaceSchemaV2,
		Fields: []ApplicationServiceStateField{{
			Name: "state_001", Purpose: "The stored shipment measurements.",
			Kind: ApplicationServiceStateRecordList,
			RecordFields: []ApplicationServiceStateRecordField{
				{Name: "member_001", Purpose: "The measurement label.", Kind: ApplicationServiceStateString},
				{Name: "member_002", Purpose: "The measurement rank.", Kind: ApplicationServiceStateInteger},
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
			Schema: ApplicationServiceStateInterfaceSchemaV2,
			Fields: []ApplicationServiceStateField{{
				Name: "state_001", Purpose: "The stored shipment measurements.", Kind: "object",
			}},
		},
		"nested record list": {
			Schema: ApplicationServiceStateInterfaceSchemaV2,
			Fields: []ApplicationServiceStateField{{
				Name: "state_001", Purpose: "The stored shipment measurements.",
				Kind: ApplicationServiceStateRecordList,
				RecordFields: []ApplicationServiceStateRecordField{{
					Name: "member_001", Purpose: "Nested measurements.",
					Kind: ApplicationServiceStateRecordList,
				}},
			}},
		},
		"record metadata on scalar": {
			Schema: ApplicationServiceStateInterfaceSchemaV2,
			Fields: []ApplicationServiceStateField{{
				Name: "state_001", Purpose: "The shipment count.", Kind: ApplicationServiceStateInteger,
				RecordFields: []ApplicationServiceStateRecordField{{
					Name: "member_001", Purpose: "The count value.", Kind: ApplicationServiceStateInteger,
				}},
			}},
		},
		"duplicate root purpose": {
			Schema: ApplicationServiceStateInterfaceSchemaV2,
			Fields: []ApplicationServiceStateField{
				{Name: "state_001", Purpose: "The shipment count.", Kind: ApplicationServiceStateInteger},
				{Name: "state_002", Purpose: "THE SHIPMENT COUNT.", Kind: ApplicationServiceStateInteger},
			},
		},
		"model authored root name": {
			Schema: ApplicationServiceStateInterfaceSchemaV2,
			Fields: []ApplicationServiceStateField{{
				Name: "shipment_count", Purpose: "The shipment count.", Kind: ApplicationServiceStateInteger,
			}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := candidate.ValidateFor(input); err == nil {
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
			},
		},
	}
}
