package assemblyline

import (
	"strings"
	"testing"
)

func TestDatabaseSchemaRelationChoiceKeepsRelationIDsCodeOwned(t *testing.T) {
	input := DatabaseSchemaRelationChoiceInput{
		ExactNeed: "Find the account name for each invoice.",
		Context:   ObjectiveContext{},
		Candidates: []DatabaseSchemaCandidate{
			{
				RelationID: "private-account-id",
				Descriptor: "Account records contain account names.",
			},
			{
				RelationID: "private-invoice-id",
				Descriptor: "Invoice records identify their associated account.",
			},
		},
	}
	prompt, err := BuildDatabaseSchemaRelationChoicePrompt(input)
	if err != nil {
		t.Fatalf("build relation choice prompt: %v", err)
	}
	job, err := NewDatabaseSchemaRelationChoiceJob(input)
	if err != nil {
		t.Fatalf("build relation choice job: %v", err)
	}
	registeredPrompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatalf("render registered relation choice job: %v", err)
	}
	if registeredPrompt != prompt {
		t.Fatalf("registered relation choice renderer differs from its prompt builder")
	}
	framing, err := PortableResponseFramingForJob(job)
	if err != nil || framing != PortableResponseFramingSingleLine {
		t.Fatalf("relation choice response framing = %q, %v", framing, err)
	}
	maximum, err := PortableResponseMaximumBytesForJob(job)
	if err != nil || maximum != 1 {
		t.Fatalf("relation choice response maximum = %d, %v; want one letter", maximum, err)
	}
	if _, err := SemanticUncertaintyContractForWorkKind(job.Kind); err != nil {
		t.Fatalf("relation choice semantic uncertainty contract: %v", err)
	}
	for _, forbidden := range []string{"private-account-id", "private-invoice-id"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt exposed code-owned relation ID %q:\n%s", forbidden, prompt)
		}
	}
	for _, required := range []string{
		"Account records contain account names.",
		"Invoice records identify their associated account.",
		"No additional remaining relation is necessary to answer the database objective",
		"A.", "B.", "C.",
	} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("prompt omitted %q:\n%s", required, prompt)
		}
	}

	selected, err := DecodeDatabaseSchemaRelationChoiceResult(input, "B")
	if err != nil {
		t.Fatalf("decode relation choice: %v", err)
	}
	if selected.NoAdditional || selected.RelationID != "private-invoice-id" {
		t.Fatalf("decoded relation choice = %+v", selected)
	}
	stopped, err := DecodeDatabaseSchemaRelationChoiceResult(input, "C")
	if err != nil {
		t.Fatalf("decode no-additional choice: %v", err)
	}
	if !stopped.NoAdditional || stopped.RelationID != "" {
		t.Fatalf("decoded no-additional choice = %+v", stopped)
	}
}
