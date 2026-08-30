package assemblyline

import (
	"strings"
	"testing"
)

func TestDatabaseSchemaRelationInventoryIsBoundedUntrustedSemanticData(t *testing.T) {
	selection := databaseSchemaSelectionFixture()
	input, err := ProjectDatabaseSchemaRelationInventoryInput(selection)
	if err != nil {
		t.Fatal(err)
	}
	job, err := NewDatabaseSchemaRelationInventoryJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, visible := range []string{
		selection.ExactNeed,
		selection.Candidates[0].Descriptor,
		"candidate semantic relation responsibilities",
		"Include no customary, optional, speculative, or merely useful responsibility",
	} {
		if !strings.Contains(prompt, visible) {
			t.Fatalf("inventory prompt omitted %q: %s", visible, prompt)
		}
	}
	for _, forbidden := range []string{
		"Code independently", "authorize", "discard", "workflow", "completion", "queue", "sieve",
	} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("inventory prompt exposed orchestration language %q: %s", forbidden, prompt)
		}
	}
	for _, hidden := range []string{selection.Candidates[0].RelationID, selection.Candidates[1].RelationID} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("inventory prompt exposed relation ID %q: %s", hidden, prompt)
		}
		if !strings.Contains(string(job.Payload), hidden) {
			t.Fatalf("inventory payload lost code-owned relation ID %q: %s", hidden, job.Payload)
		}
	}
	raw := "The appointment records containing missed outcomes.\nThe appointment records containing missed outcomes."
	inventory, err := DecodeDatabaseSchemaRelationInventory(input, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(inventory.Candidates) != 2 || inventory.Candidates[0] != inventory.Candidates[1] {
		t.Fatalf("inventory=%+v", inventory)
	}
	if err := inventory.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
}

func TestDatabaseSchemaRelationInventoryRejectsInvalidFramingAndBounds(t *testing.T) {
	input, err := ProjectDatabaseSchemaRelationInventoryInput(databaseSchemaSelectionFixture())
	if err != nil {
		t.Fatal(err)
	}
	tooMany := strings.Repeat(
		"A candidate relation responsibility.\n",
		MaxDatabaseSchemaRelationInventoryCandidates,
	) + "One extra relation responsibility."
	for _, raw := range []string{
		"",
		"The appointment records.\r\nThe clinic records.",
		"The appointment records.\n\nThe clinic records.",
		tooMany,
		`{"candidates":["The appointment records."]}`,
	} {
		if _, err := DecodeDatabaseSchemaRelationInventory(input, raw); err == nil {
			t.Fatalf("invalid inventory %q was accepted", raw)
		}
	}
}

func TestDatabaseSchemaRelationNecessityReceivesOnlyObjectiveAndOneCandidate(t *testing.T) {
	selection := databaseSchemaSelectionFixture()
	input := DatabaseSchemaRelationNecessityInput{
		ExactNeed: selection.ExactNeed, Context: selection.Context,
		Candidate: "The appointment records containing missed outcomes.",
	}
	job, err := NewDatabaseSchemaRelationNecessityJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, visible := range []string{selection.ExactNeed, input.Candidate} {
		if !strings.Contains(prompt, visible) {
			t.Fatalf("necessity prompt omitted %q: %s", visible, prompt)
		}
	}
	for _, hidden := range []string{
		selection.Candidates[0].RelationID,
		selection.Candidates[0].Descriptor,
		"selected_relation_ids",
	} {
		if strings.Contains(prompt, hidden) {
			t.Fatalf("necessity prompt exposed unrelated selection authority %q: %s", hidden, prompt)
		}
	}
	result, err := DecodeDatabaseSchemaRelationNecessityResult(
		input, DatabaseSchemaRelationNecessary,
	)
	if err != nil || result.Relation != DatabaseSchemaRelationNecessary {
		t.Fatalf("necessity=%+v err=%v", result, err)
	}
}

func TestDatabaseSchemaRelationResolutionMapsOneCandidateToRegisteredID(t *testing.T) {
	selection := databaseSchemaSelectionFixture()
	input := DatabaseSchemaRelationResolutionInput{
		Candidate:  "The appointment records containing missed outcomes.",
		Candidates: selection.Candidates,
	}
	job, err := NewDatabaseSchemaRelationResolutionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, visible := range []string{
		input.Candidate,
		selection.Candidates[0].RelationID,
		selection.Candidates[1].Descriptor,
	} {
		if !strings.Contains(prompt, visible) {
			t.Fatalf("resolution prompt omitted %q: %s", visible, prompt)
		}
	}
	if strings.Contains(prompt, selection.ExactNeed) {
		t.Fatalf("resolution prompt exposed the already-resolved objective: %s", prompt)
	}
	result, err := DecodeDatabaseSchemaRelationResolutionResult(input, "rel_b")
	if err != nil || result.RelationID != "rel_b" {
		t.Fatalf("resolution=%+v err=%v", result, err)
	}
	for _, raw := range []string{"rel_missing", `["rel_b"]`} {
		if _, err := DecodeDatabaseSchemaRelationResolutionResult(input, raw); err == nil {
			t.Fatalf("invalid resolution %q was accepted", raw)
		}
	}
}

func TestDatabaseSchemaSelectionCodeAssemblesAndValidatesRetainedSet(t *testing.T) {
	input := databaseSchemaSelectionFixture()
	decision, err := AssembleDatabaseSchemaSelectionDecision(input, []string{"rel_b"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Schema != DatabaseSchemaSelectionV1 ||
		decision.EvidenceNeedID != input.EvidenceNeedID ||
		len(decision.RelationIDs) != 1 || decision.RelationIDs[0] != "rel_b" {
		t.Fatalf("decision=%+v", decision)
	}
	for name, relationIDs := range map[string][]string{
		"invented":  {"rel_missing"},
		"duplicate": {"rel_a", "rel_a"},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := AssembleDatabaseSchemaSelectionDecision(input, relationIDs); err == nil {
				t.Fatalf("accepted relation IDs %#v", relationIDs)
			}
		})
	}
}

func databaseSchemaSelectionFixture() DatabaseSchemaSelectionInput {
	return DatabaseSchemaSelectionInput{
		EvidenceNeedID: "need-1",
		ExactNeed:      "Which clinics have the most missed appointments?",
		Candidates: []DatabaseSchemaCandidate{
			{RelationID: "rel_a", Descriptor: "public.clinics columns: col_a name:text"},
			{RelationID: "rel_b", Descriptor: "public.appointments columns: col_b status:text, col_c clinic_id:uuid"},
		},
		MaxSelections: 2,
	}
}
