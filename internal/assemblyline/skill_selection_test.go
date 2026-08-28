package assemblyline

import (
	"strings"
	"testing"
)

func TestSkillSelectionSeesOnlyBoundedOpaqueSummaries(t *testing.T) {
	t.Parallel()

	input := SkillSelectionInput{
		LocalContext: "interactive browser tool",
		Need:         "support pressure-sensitive pointer interaction",
		Candidates: []SkillCandidateSummary{
			{Token: "SKILL_1", Purpose: "Handle pointer pressure in an interactive browser view."},
			{Token: "SKILL_2", Purpose: "Filter a bounded collection from text input."},
		},
	}
	job, err := NewSkillSelectionJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{input.Need, "SKILL_1", "SKILL_2", SkillSelectionNone} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("selection prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{"learned_", "workspace", "filename", "project tree", "procedure"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("selection prompt leaked %q:\n%s", forbidden, prompt)
		}
	}
	decision, err := DecodeSkillSelectionDecision(input, "SKILL_1")
	if err != nil || decision.Selected != "SKILL_1" || decision.Schema != SkillSelectionSchemaV1 {
		t.Fatalf("decision=%+v err=%v", decision, err)
	}
}

func TestSkillSelectionRejectsTokenOutsideCandidateSet(t *testing.T) {
	t.Parallel()

	input := SkillSelectionInput{
		LocalContext: "interactive browser tool",
		Need:         "filter a bounded collection",
		Candidates:   []SkillCandidateSummary{{Token: "SKILL_1", Purpose: "Filter a collection."}},
	}
	if err := (SkillSelectionDecision{
		Schema: SkillSelectionSchemaV1, Selected: "SKILL_9",
	}).ValidateFor(input); err == nil {
		t.Fatal("selection accepted a token outside the code-owned candidate set")
	}
	if err := (SkillSelectionDecision{
		Schema: SkillSelectionSchemaV1, Selected: SkillSelectionNone,
	}).ValidateFor(input); err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeSkillSelectionDecision(input, `{"selected":"SKILL_1"}`); err == nil {
		t.Fatal("accepted JSON wrapper")
	}
}
