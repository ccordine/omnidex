package assemblyline

import (
	"strings"
	"testing"
)

func TestDatabaseSchemaSelectionUsesOneRawLeafPerQuestion(t *testing.T) {
	input := DatabaseSchemaSelectionLeafInput{
		Authority: databaseSchemaSelectionFixture(), SelectedRelationIDs: []string{},
	}
	coverageJob, err := NewDatabaseSchemaSelectionCoverageJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(coverageJob)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(prompt, DatabaseSchemaRelationRemains) {
		t.Fatalf("coverage prompt omitted registered result tokens: %s", prompt)
	}
	if got, err := DecodeDatabaseSchemaSelectionCoverageLeaf(input, DatabaseSchemaRelationRemains); err != nil || got != DatabaseSchemaRelationRemains {
		t.Fatalf("coverage=%q err=%v", got, err)
	}
	if _, err := DecodeDatabaseSchemaSelectionCoverageLeaf(input, `{"coverage":"RELATION_REMAINS"}`); err == nil {
		t.Fatal("database schema coverage accepted JSON")
	}

	relationJob, err := NewDatabaseSchemaRelationSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if relationJob.Kind != WorkDatabaseSchemaRelationSelection {
		t.Fatalf("kind=%q", relationJob.Kind)
	}
	selected, err := DecodeDatabaseSchemaRelationSelectionLeaf(input, "rel_b")
	if err != nil || selected != "rel_b" {
		t.Fatalf("selected=%q err=%v", selected, err)
	}
	if _, err := DecodeDatabaseSchemaRelationSelectionLeaf(input, `["rel_b"]`); err == nil {
		t.Fatal("database relation selection accepted JSON")
	}
}

func TestDatabaseSchemaSelectionCodeAssemblesAndValidatesRetainedSet(t *testing.T) {
	input := databaseSchemaSelectionFixture()
	decision, err := AssembleDatabaseSchemaSelectionDecision(input, []string{"rel_b"})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Schema != DatabaseSchemaSelectionV1 || decision.EvidenceNeedID != input.EvidenceNeedID || len(decision.RelationIDs) != 1 || decision.RelationIDs[0] != "rel_b" {
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
