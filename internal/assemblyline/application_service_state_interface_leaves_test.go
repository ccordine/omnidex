package assemblyline

import (
	"strings"
	"testing"
)

func TestApplicationServiceStatePurposeInventoriesAreBoundedUntrustedRawLines(t *testing.T) {
	t.Parallel()
	authority := serviceStateInterfaceFixture()
	rootInput := ApplicationStateFieldPurposeInventoryInput{Authority: authority}
	rootJob, err := NewApplicationStateFieldPurposeInventoryJob(rootInput)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(rootJob)
	if err != nil {
		t.Fatal(err)
	}
	for _, visible := range []string{
		authority.ProductContext,
		authority.Needs[0].RequirementQuote,
		"minimal durable root-value purposes",
		"omit customary capabilities not required by the supplied authority",
	} {
		if !strings.Contains(prompt, visible) {
			t.Fatalf("root inventory prompt omitted %q: %s", visible, prompt)
		}
	}
	for _, forbidden := range []string{
		"Code screens", "sieve", "queue", "workflow", "completion", "downstream",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("root inventory prompt exposed orchestration language %q: %s", forbidden, prompt)
		}
	}
	raw := "The stored shipment entries.\nThe stored shipment entries.\nThe last retrieval cursor."
	inventory, err := DecodeApplicationStateFieldPurposeInventory(rootInput, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Purposes) != 3 || inventory.Purposes[0] != inventory.Purposes[1] {
		t.Fatalf("inventory=%+v", inventory)
	}
	if err := inventory.ValidateForStateFields(rootInput); err != nil {
		t.Fatal(err)
	}

	recordInput := ApplicationRecordFieldPurposeInventoryInput{
		Authority: authority, ParentPurpose: "The stored shipment entries.",
	}
	recordJob, err := NewApplicationRecordFieldPurposeInventoryJob(recordInput)
	if err != nil {
		t.Fatal(err)
	}
	recordPrompt, err := RenderPortableJob(recordJob)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(recordPrompt, recordInput.ParentPurpose) ||
		strings.Contains(recordPrompt, "accepted_fields") {
		t.Fatalf("record inventory prompt has wrong authority: %s", recordPrompt)
	}
	if _, err := DecodeApplicationRecordFieldPurposeInventory(
		recordInput, "The shipment identifier.\nThe recorded amount.",
	); err != nil {
		t.Fatal(err)
	}
}

func TestApplicationServiceStatePurposeInventoryRejectsInvalidFramingAndBounds(t *testing.T) {
	t.Parallel()
	input := ApplicationStateFieldPurposeInventoryInput{Authority: serviceStateInterfaceFixture()}
	tooMany := strings.Repeat("A necessary value.\n", MaxApplicationServiceStateInterfaceFields) +
		"One extra value."
	for _, raw := range []string{
		"",
		"The stored value.\r\nThe other value.",
		"The stored value.\n\nThe other value.",
		`{"purposes":["The stored value."]}`,
		tooMany,
	} {
		if _, err := DecodeApplicationStateFieldPurposeInventory(input, raw); err == nil {
			t.Fatalf("invalid inventory %q was accepted", raw)
		}
	}
}

func TestApplicationServiceStatePurposeNecessityUsesOnlyDirectAuthorityAndCandidate(t *testing.T) {
	t.Parallel()
	input := ApplicationServiceStatePurposeNecessityInput{
		Scope:            ApplicationServiceStateRootPurposeScope,
		Authority:        serviceStateInterfaceFixture(),
		CandidatePurpose: "The stored shipment entries.",
	}
	job, err := NewApplicationServiceStatePurposeNecessityJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, visible := range []string{
		input.Authority.ProductContext,
		input.Authority.Needs[0].RequirementQuote,
		input.CandidatePurpose,
		ApplicationServiceStatePurposeNecessary,
		ApplicationServiceStatePurposeNotNecessary,
	} {
		if !strings.Contains(prompt, visible) {
			t.Fatalf("necessity prompt omitted %q: %s", visible, prompt)
		}
	}
	result, err := DecodeApplicationServiceStatePurposeNecessityResult(
		input, ApplicationServiceStatePurposeNecessary,
	)
	if err != nil || result.Relation != ApplicationServiceStatePurposeNecessary {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if _, err := DecodeApplicationServiceStatePurposeNecessityResult(input, "ACCEPT"); err == nil {
		t.Fatal("workflow control label was accepted as necessity relation")
	}
}

func TestApplicationServiceStatePurposeRelationIsPairwiseAndByteDifferent(t *testing.T) {
	t.Parallel()
	input := ApplicationServiceStatePurposeRelationInput{
		Scope:            ApplicationServiceStateRecordPurposeScope,
		CandidatePurpose: "The shipment identifier.",
		AcceptedPurpose:  "The identifier of each shipment.",
	}
	job, err := NewApplicationServiceStatePurposeRelationJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, visible := range []string{input.CandidatePurpose, input.AcceptedPurpose} {
		if !strings.Contains(prompt, visible) {
			t.Fatalf("relation prompt omitted %q: %s", visible, prompt)
		}
	}
	for _, hidden := range []string{"product_context", "requirement_quote", "accepted_fields"} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("relation prompt exposed unrelated authority %q: %s", hidden, prompt)
		}
	}
	result, err := DecodeApplicationServiceStatePurposeRelationResult(
		input, ApplicationServiceStateSamePurpose,
	)
	if err != nil || result.Relation != ApplicationServiceStateSamePurpose {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	input.AcceptedPurpose = input.CandidatePurpose
	if _, err := NewApplicationServiceStatePurposeRelationJob(input); err == nil {
		t.Fatal("byte-identical purposes opened a semantic relation")
	}
}

func TestApplicationServiceStateKindLeavesReceiveOnlyFocusedSemanticValue(t *testing.T) {
	t.Parallel()
	input := ApplicationStateFieldKindInput{
		Authority:      serviceStateInterfaceFixture(),
		FocusedPurpose: "The stored shipment entries.",
	}
	job, err := NewApplicationStateFieldKindJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, input.FocusedPurpose) || strings.Contains(prompt, "accepted_fields") {
		t.Fatalf("kind prompt has wrong semantic authority: %s", prompt)
	}
	if kind, err := DecodeApplicationStateFieldKindLeaf(
		input, string(ApplicationServiceStateRecordList),
	); err != nil || kind != ApplicationServiceStateRecordList {
		t.Fatalf("kind=%q err=%v", kind, err)
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
}
