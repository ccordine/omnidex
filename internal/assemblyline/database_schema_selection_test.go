package assemblyline

import (
	"strings"
	"testing"
)

func TestDatabaseSchemaSelectionReturnsOnlyProjectedOpaqueIDs(t *testing.T) {
	input := databaseSchemaSelectionFixture()
	job, err := NewDatabaseSchemaSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	if job.Kind != WorkDatabaseSchemaSelection {
		t.Fatalf("kind=%q", job.Kind)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"password", "dsn", "SELECT ", "execute"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("prompt contains forbidden authority %q: %s", forbidden, prompt)
		}
	}
	items := schema["properties"].(map[string]any)["relation_ids"].(map[string]any)["items"].(map[string]any)
	if got := items["enum"].([]string); len(got) != 2 || got[0] != "rel_a" || got[1] != "rel_b" {
		t.Fatalf("relation enum=%v", got)
	}
	decision, err := DecodeDatabaseSchemaSelectionDecision(input,
		`{"schema":"omnidex.database-schema-selection.v1","evidence_need_id":"need-1","relation_ids":["rel_b"]}`)
	if err != nil || len(decision.RelationIDs) != 1 || decision.RelationIDs[0] != "rel_b" {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func TestDatabaseSchemaSelectionRejectsInventedDuplicateAndImplicitState(t *testing.T) {
	input := databaseSchemaSelectionFixture()
	for name, raw := range map[string]string{
		"invented":  `{"schema":"omnidex.database-schema-selection.v1","evidence_need_id":"need-1","relation_ids":["rel_missing"]}`,
		"duplicate": `{"schema":"omnidex.database-schema-selection.v1","evidence_need_id":"need-1","relation_ids":["rel_a","rel_a"]}`,
		"implicit":  `{"schema":"omnidex.database-schema-selection.v1","evidence_need_id":"need-1","relation_ids":null}`,
		"extra":     `{"schema":"omnidex.database-schema-selection.v1","evidence_need_id":"need-1","relation_ids":[],"sql":"SELECT 1"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeDatabaseSchemaSelectionDecision(input, raw); err == nil {
				t.Fatalf("accepted %s", raw)
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
