package assemblyline

import (
	"strings"
	"testing"
)

func TestSkillProcedurePromptContainsOnlyOneNeedAndTechnicalBoundary(t *testing.T) {
	t.Parallel()

	input := SkillProcedureInput{
		LocalContext: "interactive browser tool",
		Need:         "support pressure-sensitive pointer interaction",
		Boundary:     SkillBoundaryTypeScriptReactView,
	}
	job, err := NewSkillProcedureJob(input)
	if err != nil {
		t.Fatal(err)
	}
	prompt, schema, err := RenderPortableJob(job)
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{input.Need, string(input.Boundary), "one reusable procedure"} {
		if !strings.Contains(prompt, required) {
			t.Fatalf("skill prompt omitted %q:\n%s", required, prompt)
		}
	}
	for _, forbidden := range []string{"workspace", "filename", "project tree", "dependency graph", "benchmark"} {
		if strings.Contains(strings.ToLower(prompt), forbidden) {
			t.Fatalf("skill prompt leaked %q:\n%s", forbidden, prompt)
		}
	}
	if schema == nil {
		t.Fatal("skill procedure has no structured response schema")
	}
}

func TestSkillProcedureDecisionRejectsEmptyOrOversizedProcedure(t *testing.T) {
	t.Parallel()

	input := SkillProcedureInput{
		LocalContext: "interactive browser tool",
		Need:         "provide one bounded interaction",
		Boundary:     SkillBoundaryTypeScriptReactView,
	}
	for _, procedure := range []string{"", " too broad", strings.Repeat("x", maxSkillProcedureBytes+1)} {
		decision := SkillProcedureDecision{Schema: SkillProcedureSchemaV1, Procedure: procedure}
		if err := decision.ValidateFor(input); err == nil {
			t.Fatalf("accepted invalid procedure length=%d", len(procedure))
		}
	}
	decision := SkillProcedureDecision{
		Schema:    SkillProcedureSchemaV1,
		Procedure: "Keep state local, expose a labelled control, and update the visible result from its interaction handler.",
	}
	if err := decision.ValidateFor(input); err != nil {
		t.Fatal(err)
	}
}
