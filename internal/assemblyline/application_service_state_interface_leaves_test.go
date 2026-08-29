package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationServiceStateLeavesKeepIdentifiersCodeOwned(t *testing.T) {
	t.Parallel()
	authority := serviceStateInterfaceFixture()
	accepted := []ApplicationServiceStateField{{
		Name: "state_001", Purpose: "The accepted shipment count.",
		Kind:         ApplicationServiceStateInteger,
		RecordFields: []ApplicationServiceStateRecordField{},
	}}
	leafInput := ApplicationStateFieldLeafInput{
		Authority: authority, AcceptedFields: accepted,
	}
	for name, construct := range map[string]func() (PortableJob, error){
		"coverage": func() (PortableJob, error) {
			return NewApplicationStateFieldCoverageJob(leafInput)
		},
		"purpose": func() (PortableJob, error) {
			return NewApplicationStateFieldPurposeJob(leafInput)
		},
	} {
		t.Run(name, func(t *testing.T) {
			job, err := construct()
			if err != nil {
				t.Fatal(err)
			}
			prompt, err := RenderPortableJob(job)
			if err != nil {
				t.Fatal(err)
			}
			for _, visible := range []string{
				authority.ProductContext,
				authority.Needs[0].RequirementQuote,
				accepted[0].Purpose,
			} {
				if !strings.Contains(prompt, visible) {
					t.Fatalf("%s prompt omitted semantic authority %q: %s", name, visible, prompt)
				}
			}
			for _, hidden := range []string{accepted[0].Name, `"name"`} {
				if strings.Contains(prompt, hidden) {
					t.Fatalf("%s prompt exposed code-owned identifier %q: %s", name, hidden, prompt)
				}
			}
			if !strings.Contains(string(job.Payload), accepted[0].Name) {
				t.Fatalf("%s payload lost code-owned purpose binding: %s", name, job.Payload)
			}
		})
	}
}

func TestApplicationServiceStateKindLeavesReceiveOnlyFocusedSemanticValue(t *testing.T) {
	t.Parallel()
	authority := serviceStateInterfaceFixture()
	acceptedPurpose := "ACCEPTED PURPOSE HIDDEN FROM KIND SENTINEL"
	focusedPurpose := "FOCUSED PURPOSE VISIBLE SENTINEL"
	input := ApplicationStateFieldKindInput{
		Authority: authority,
		AcceptedFields: []ApplicationServiceStateField{{
			Name: "state_001", Purpose: acceptedPurpose,
			Kind:         ApplicationServiceStateInteger,
			RecordFields: []ApplicationServiceStateRecordField{},
		}},
		FocusedPurpose: focusedPurpose,
	}
	job, err := NewApplicationStateFieldKindJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, focusedPurpose) || !strings.Contains(prompt, authority.ProductContext) {
		t.Fatalf("kind prompt omitted focused semantic authority: %s", prompt)
	}
	for _, hidden := range []string{acceptedPurpose, "state_001", `"accepted_fields"`} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("kind prompt exposed unrelated accepted state %q: %s", hidden, prompt)
		}
		if !strings.Contains(string(job.Payload), hidden) && hidden != `"accepted_fields"` {
			t.Fatalf("kind payload lost code-owned retained state %q: %s", hidden, job.Payload)
		}
	}
	if kind, err := DecodeApplicationStateFieldKindLeaf(
		input, string(ApplicationServiceStateInteger),
	); err != nil || kind != ApplicationServiceStateInteger {
		t.Fatalf("kind=%q err=%v", kind, err)
	}
}

func TestApplicationServiceRecordLeavesProjectPurposesWithoutTechnicalKeys(t *testing.T) {
	t.Parallel()
	authority := serviceStateInterfaceFixture()
	input := ApplicationRecordFieldLeafInput{
		Authority: authority, ParentPurpose: "The stored shipment measurements.",
		AcceptedRecordFields: []ApplicationServiceStateRecordField{{
			Name: "member_001", Purpose: "The measurement label.",
			Kind: ApplicationServiceStateString,
		}},
	}
	job, err := NewApplicationRecordFieldPurposeJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, visible := range []string{input.ParentPurpose, input.AcceptedRecordFields[0].Purpose} {
		if !strings.Contains(prompt, visible) {
			t.Fatalf("record purpose prompt omitted %q: %s", visible, prompt)
		}
	}
	for _, hidden := range []string{input.AcceptedRecordFields[0].Name, `"name"`} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("record purpose prompt exposed code-owned key %q: %s", hidden, prompt)
		}
	}
	if purpose, err := DecodeApplicationRecordFieldPurposeLeaf(
		input, "The measurement rank.",
	); err != nil || purpose != "The measurement rank." {
		t.Fatalf("purpose=%q err=%v", purpose, err)
	}
}

func TestApplicationServiceStatePurposeDecodersRejectStructuredOrRepeatedValues(t *testing.T) {
	t.Parallel()
	input := ApplicationStateFieldLeafInput{
		Authority: serviceStateInterfaceFixture(),
		AcceptedFields: []ApplicationServiceStateField{{
			Name: "state_001", Purpose: "The shipment count.",
			Kind:         ApplicationServiceStateInteger,
			RecordFields: []ApplicationServiceStateRecordField{},
		}},
	}
	for _, candidate := range []string{
		`{"purpose":"The next value."}`,
		`"The next value."`,
		"THE SHIPMENT COUNT.",
		"two\nlines",
		ApplicationStateFieldRemains,
		ApplicationNoUncoveredStateField,
		ApplicationRecordFieldRemains,
		ApplicationNoUncoveredRecordField,
	} {
		if _, err := DecodeApplicationStateFieldPurposeLeaf(input, candidate); err == nil {
			t.Fatalf("invalid purpose candidate %q was accepted", candidate)
		}
	}
	recordInput := ApplicationRecordFieldLeafInput{
		Authority:            input.Authority,
		ParentPurpose:        "The stored shipment measurements.",
		AcceptedRecordFields: []ApplicationServiceStateRecordField{},
	}
	for _, candidate := range []string{
		ApplicationStateFieldRemains,
		ApplicationNoUncoveredStateField,
		ApplicationRecordFieldRemains,
		ApplicationNoUncoveredRecordField,
	} {
		if _, err := DecodeApplicationRecordFieldPurposeLeaf(recordInput, candidate); err == nil {
			t.Fatalf("invalid record purpose candidate %q was accepted", candidate)
		}
	}
	if got, err := DecodeApplicationStateFieldCoverageLeaf(
		input, ApplicationNoUncoveredStateField,
	); err != nil || got != ApplicationNoUncoveredStateField {
		t.Fatalf("coverage=%q err=%v", got, err)
	}
}

func TestApplicationServiceStateTechnicalNamesAreExactCodeOwnedOrdinals(t *testing.T) {
	t.Parallel()
	for index, want := range []string{"state_001", "state_002", "state_003"} {
		got, err := CodeOwnedApplicationServiceStateFieldName(index + 1)
		if err != nil || got != want {
			t.Fatalf("root %d=%q want=%q err=%v", index+1, got, want, err)
		}
	}
	for index, want := range []string{"member_001", "member_002", "member_003"} {
		got, err := CodeOwnedApplicationServiceRecordFieldName(index + 1)
		if err != nil || got != want {
			t.Fatalf("member %d=%q want=%q err=%v", index+1, got, want, err)
		}
	}
	if _, err := CodeOwnedApplicationServiceStateFieldName(0); err == nil {
		t.Fatal("zero root field ordinal was accepted")
	}
	if _, err := CodeOwnedApplicationServiceRecordFieldName(
		MaxApplicationServiceStateInterfaceFields + 1,
	); err == nil {
		t.Fatal("out-of-bound record field ordinal was accepted")
	}
}
